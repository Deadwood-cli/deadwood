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
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: real deletion is not enabled yet; no branches were deleted")
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

// writeDryRunPlan is the phase 8 apply step. It must never call
// git.CreateBackupRef or git.DeleteBranch.
func writeDryRunPlan(w io.Writer, names []string) error {
	noun := "branch"
	if len(names) != 1 {
		noun = "branches"
	}
	fmt.Fprintf(w, "Dry-run: would delete %d %s (nothing was changed).\n", len(names), noun)
	for _, name := range names {
		fmt.Fprintf(w, "  %s\n", name)
		fmt.Fprintf(w, "    backup would be refs/deadwood-backup/%s\n", name)
	}
	return nil
}
