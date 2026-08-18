package cli

import "github.com/spf13/cobra"

// cleanOptions holds flags specific to `deadwood clean`.
type cleanOptions struct {
	dryRun             bool
	yes                bool
	includeNeedsReview bool
}

const cleanLong = `Clean runs a scan, then opens an interactive checklist of branches to delete.
Safe-to-delete and squash-merged branches are pre-checked; needs-review branches
are listed but must be checked in by hand.

Nothing is deleted unless you take an explicit action beyond running the command:
--dry-run defaults to true and must be set to false. Every deletion is preceded
by a backup ref under refs/deadwood-backup/, restorable with 'deadwood undo'.`

func newCleanCommand(_ *globalOptions) *cobra.Command {
	opts := &cleanOptions{}

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Review and delete dead local branches interactively",
		Long:  cleanLong,
		Args:  cobra.NoArgs,
		RunE:  notImplemented(8),
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
