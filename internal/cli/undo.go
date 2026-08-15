package cli

import "github.com/spf13/cobra"

// undoOptions holds flags specific to `deadwood undo`.
type undoOptions struct {
	list bool
}

const undoLong = `Undo recreates a branch from the backup ref deadwood wrote before deleting it
(refs/deadwood-backup/<branch-name>).

If the name is not found, the available backups are listed instead.`

func newUndoCommand(_ *globalOptions) *cobra.Command {
	opts := &undoOptions{}

	cmd := &cobra.Command{
		Use:   "undo [branch-name]",
		Short: "Restore a branch deadwood deleted, from its backup ref",
		Long:  undoLong,
		// One name to restore, or none when --list is used; RunE enforces the
		// pairing once the command is implemented.
		Args: cobra.MaximumNArgs(1),
		RunE: notImplemented(9),
	}

	cmd.Flags().BoolVar(&opts.list, "list", false,
		"list available backups with their deletion context, without restoring")

	return cmd
}
