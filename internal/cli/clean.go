package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/Deadwood-cli/deadwood/internal/config"
	"github.com/Deadwood-cli/deadwood/internal/git"
	"github.com/Deadwood-cli/deadwood/internal/tui"
	"github.com/spf13/cobra"
)

// cleanOptions holds flags specific to `deadwood clean`.
type cleanOptions struct {
	dryRun             bool
	yes                bool
	includeNeedsReview bool
}

// cleanDeps lets tests stub the checklist and confirmation without a TTY.
type cleanDeps struct {
	runChecklist func(items []tui.Item, in io.Reader, out io.Writer) (selected []tui.Item, cancelled bool, err error)
	confirm      func(in io.Reader, out io.Writer, names []string) (bool, error)
}

func defaultCleanDeps() *cleanDeps {
	return &cleanDeps{
		runChecklist: tui.Run,
		confirm:      confirmDeletes,
	}
}

const cleanLong = `Clean runs a scan, then opens an interactive checklist of branches to delete.
Safe-to-delete and squash-merged branches are pre-checked; needs-review branches
are listed but must be checked in by hand.

Nothing is deleted unless you take an explicit action beyond running the command:
--dry-run defaults to true and must be set to false. Every deletion is preceded
by a backup ref under refs/deadwood-backup/, restorable with 'deadwood undo'.`

func newCleanCommand(global *globalOptions, sd *scanDeps, cd *cleanDeps) *cobra.Command {
	if cd == nil {
		cd = defaultCleanDeps()
	}
	opts := &cleanOptions{}

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Review and delete dead local branches interactively",
		Long:  cleanLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd, global, opts, sd, cd)
		},
	}

	flags := cmd.Flags()
	// Safety invariant (spec Section 11): this default is true and must not be
	// changed without an explicit instruction from the project owner.
	flags.BoolVar(&opts.dryRun, "dry-run", true,
		"report what would be deleted without deleting anything")
	flags.BoolVar(&opts.yes, "yes", false,
		"skip the typed confirmation prompt (still respects --dry-run)")
	flags.BoolVar(&opts.includeNeedsReview, "include-needs-review", false,
		"pre-check needs-review branches in the checklist")

	return cmd
}

func runClean(cmd *cobra.Command, global *globalOptions, opts *cleanOptions, sd *scanDeps, cd *cleanDeps) error {
	if sd == nil {
		sd = defaultScanDeps()
	}
	if cd == nil {
		cd = defaultCleanDeps()
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	root, err := git.RepoRoot(".")
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}
	cfg, err := config.Load(root, global.configPath)
	if err != nil {
		return err
	}
	report, err := buildScan(ctx, root, cfg, sd, cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	items := checklistItems(report.Results, opts.includeNeedsReview, cfg.StaleWarningDays)
	if len(items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No branches to clean.")
		return nil
	}

	selected, cancelled, err := cd.runChecklist(items, cmd.InOrStdin(), cmd.OutOrStdout())
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No branches were deleted.")
		return nil
	}
	if len(selected) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No branches selected. Nothing to delete.")
		return nil
	}

	names := make([]string, len(selected))
	for i, item := range selected {
		names[i] = item.Name
	}

	if !opts.yes {
		ok, err := cd.confirm(cmd.InOrStdin(), cmd.OutOrStdout(), names)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(cmd.OutOrStdout(), "Cancelled. No branches were deleted.")
			return nil
		}
	}

	if !opts.dryRun {
		return applyDeletes(cmd.OutOrStdout(), root, names)
	}

	return writeDryRunPlan(cmd.OutOrStdout(), names)
}

func checklistItems(results []classify.BranchResult, includeNeedsReview bool, staleDays int) []tui.Item {
	order := []classify.Bucket{
		classify.BucketSafeDelete,
		classify.BucketSquashMerged,
		classify.BucketNeedsReview,
	}
	grouped := make(map[classify.Bucket][]classify.BranchResult, len(order))
	for _, result := range results {
		switch result.Bucket {
		case classify.BucketSafeDelete, classify.BucketSquashMerged, classify.BucketNeedsReview:
			grouped[result.Bucket] = append(grouped[result.Bucket], result)
		}
	}

	items := make([]tui.Item, 0)
	for _, bucket := range order {
		for _, result := range grouped[bucket] {
			checked := bucket == classify.BucketSafeDelete || bucket == classify.BucketSquashMerged
			if bucket == classify.BucketNeedsReview {
				checked = includeNeedsReview
			}
			items = append(items, tui.Item{
				Name:    result.Branch.Name,
				Bucket:  result.Bucket,
				Reason:  itemReason(result, staleDays),
				Checked: checked,
			})
		}
	}
	return items
}

func itemReason(result classify.BranchResult, staleDays int) string {
	reason := result.Reason
	if result.Bucket != classify.BucketNeedsReview || staleDays <= 0 || result.Branch.LastCommitDate.IsZero() {
		return reason
	}
	age := time.Since(result.Branch.LastCommitDate.UTC())
	if age >= time.Duration(staleDays)*24*time.Hour {
		return reason + " (stale)"
	}
	return reason
}

func confirmDeletes(in io.Reader, out io.Writer, names []string) (bool, error) {
	noun := "branch"
	if len(names) != 1 {
		noun = "branches"
	}
	fmt.Fprintf(out, "\nAbout to delete %d %s:\n", len(names), noun)
	for _, name := range names {
		fmt.Fprintf(out, "  %s\n", name)
	}
	fmt.Fprint(out, "\nType yes to continue: ")

	scanner := bufio.NewScanner(in)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return false, err
		}
		return false, nil
	}
	return strings.TrimSpace(scanner.Text()) == "yes", nil
}

// writeDryRunPlan reports what would be deleted without calling git.
func writeDryRunPlan(w io.Writer, names []string) error {
	noun := "branch"
	if len(names) != 1 {
		noun = "branches"
	}
	fmt.Fprintf(w, "Dry-run: would delete %d %s (nothing was changed).\n", len(names), noun)
	for _, name := range names {
		fmt.Fprintf(w, "  %s\n", name)
		fmt.Fprintf(w, "    backup would be %s%s\n", git.BackupRefPrefix, name)
	}
	return nil
}

// applyDeletes backs up then safe-deletes each branch. CreateBackupRef is
// required to succeed before DeleteBranch; a failed backup skips that branch.
// git branch -d failures are reported, never escalated to -D.
func applyDeletes(w io.Writer, repoPath string, names []string) error {
	var deleted []string
	type skippedBranch struct {
		name   string
		reason string
	}
	var skipped []skippedBranch

	for _, name := range names {
		if err := git.CreateBackupRef(repoPath, name); err != nil {
			skipped = append(skipped, skippedBranch{name, err.Error()})
			continue
		}
		if err := git.DeleteBranch(repoPath, name, false); err != nil {
			skipped = append(skipped, skippedBranch{name, err.Error()})
			continue
		}
		deleted = append(deleted, name)
	}

	if len(deleted) == 0 && len(skipped) == 0 {
		fmt.Fprintln(w, "Nothing was deleted.")
		return nil
	}

	if len(deleted) > 0 {
		noun := "branch"
		if len(deleted) != 1 {
			noun = "branches"
		}
		fmt.Fprintf(w, "Deleted %d %s.\n", len(deleted), noun)
		for _, name := range deleted {
			fmt.Fprintf(w, "  %s\n", name)
			fmt.Fprintf(w, "    backup: %s%s\n", git.BackupRefPrefix, name)
		}
	}

	if len(skipped) > 0 {
		noun := "branch"
		if len(skipped) != 1 {
			noun = "branches"
		}
		fmt.Fprintf(w, "Skipped %d %s:\n", len(skipped), noun)
		for _, item := range skipped {
			fmt.Fprintf(w, "  %s: %s\n", item.name, item.reason)
		}
	}

	if len(deleted) > 0 {
		fmt.Fprintln(w, "Restore with `deadwood undo <branch>`.")
	}
	return nil
}
