package cli

import "github.com/spf13/cobra"

// scanOptions holds flags specific to `deadwood scan`.
type scanOptions struct {
	json bool
}

const scanLong = `Scan classifies every local branch into one of five buckets: safe to delete,
squash-merged, needs review, active, or protected.

scan is strictly read-only. It never prompts for deletion and never modifies the
repository.`

func newScanCommand(_ *globalOptions) *cobra.Command {
	opts := &scanOptions{}

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Report which local branches are dead on the remote (read-only)",
		Long:  scanLong,
		Args:  cobra.NoArgs,
		RunE:  notImplemented(4),
	}

	cmd.Flags().BoolVar(&opts.json, "json", false,
		"emit machine-readable JSON instead of the human-readable report")

	return cmd
}
