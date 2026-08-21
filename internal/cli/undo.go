package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/Deadwood-cli/deadwood/internal/git"
	"github.com/spf13/cobra"
)

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
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUndo(cmd, opts, args)
		},
	}

	cmd.Flags().BoolVar(&opts.list, "list", false,
		"list available backups with their deletion context, without restoring")

	return cmd
}

func runUndo(cmd *cobra.Command, opts *undoOptions, args []string) error {
	root, err := git.RepoRoot(".")
	if err != nil {
		return fmt.Errorf("not a git repository: %w", err)
	}

	if opts.list || len(args) == 0 {
		return listBackups(cmd.OutOrStdout(), root)
	}

	branch := args[0]
	if err := git.RestoreFromBackup(root, branch); err != nil {
		if errors.Is(err, git.ErrBackupMissing) {
			fmt.Fprintf(cmd.OutOrStdout(), "No backup found for %q.\n", branch)
			if listErr := listBackups(cmd.OutOrStdout(), root); listErr != nil {
				return listErr
			}
			return err
		}
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Restored %s from %s%s\n", branch, git.BackupRefPrefix, branch)
	return nil
}

func listBackups(w io.Writer, repoPath string) error {
	infos, err := git.ListBackupDetails(repoPath)
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(w, "No backup refs found.")
		return nil
	}

	fmt.Fprintln(w, "Available backups:")
	for _, info := range infos {
		date := "unknown"
		if !info.CommitDate.IsZero() {
			date = info.CommitDate.UTC().Format("2006-01-02")
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\n", info.Branch, date, info.Subject)
	}
	return nil
}
