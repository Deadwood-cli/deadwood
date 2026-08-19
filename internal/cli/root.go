// Package cli wires the deadwood cobra command tree together. It is the only
// layer permitted to combine internal/git, internal/github and internal/classify,
// which keeps the classification engine testable in isolation.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// BuildInfo carries version metadata injected at link time.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// globalOptions holds the flags every subcommand shares. The spec lists
// --config and --verbose under scan; they are registered as persistent flags so
// clean and undo can honour the same config file and verbosity.
type globalOptions struct {
	configPath string
	verbose    bool
}

const rootLong = `Deadwood finds local Git branches whose remote counterpart is gone and helps
you delete the ones that are provably merged.

It never deletes a branch containing work that is unreachable from the default
branch without explicit, informed confirmation. Running deadwood with no
subcommand performs a read-only scan.`

// NewRootCommand builds the complete command tree. It is exported so tests can
// exercise the tree without going through os.Args.
func NewRootCommand(info BuildInfo) *cobra.Command {
	return newRootCommand(info, defaultAuthRuntime(), defaultScanDeps(), nil)
}

func newRootCommand(info BuildInfo, ar *authRuntime, sd *scanDeps, cd *cleanDeps) *cobra.Command {
	if ar == nil {
		ar = defaultAuthRuntime()
	}
	if sd == nil {
		sd = defaultScanDeps()
	}
	global := &globalOptions{}

	root := &cobra.Command{
		Use:     "deadwood",
		Short:   "Safely clean up local Git branches that are dead on the remote",
		Long:    rootLong,
		Version: info.Version,
		Args:    cobra.NoArgs,
		// Runtime failures are the user's problem to fix, not a cue to reprint
		// the whole usage block.
		SilenceUsage: true,
	}

	root.SetVersionTemplate(fmt.Sprintf(
		"deadwood %s (commit %s, built %s)\n", info.Version, info.Commit, info.Date))

	persistent := root.PersistentFlags()
	persistent.StringVar(&global.configPath, "config", "",
		"path to .deadwood.yml (default: repository root)")
	persistent.BoolVar(&global.verbose, "verbose", false,
		"show the reason and confidence for every branch, including active and protected ones")

	scan := newScanCommand(global, sd)
	root.AddCommand(
		scan,
		newCleanCommand(global, sd, cd),
		newUndoCommand(global),
		newAuthCommand(global, ar),
	)

	// `deadwood --json` must work the same as `deadwood scan --json`, because
	// scan is the default command.
	root.Flags().AddFlagSet(scan.LocalFlags())

	// `deadwood` with no subcommand is equivalent to `deadwood scan`. Dispatch
	// through the scan command itself so its own flags resolve correctly.
	root.RunE = func(_ *cobra.Command, args []string) error {
		return scan.RunE(scan, args)
	}

	return root
}

// Execute runs the root command. Cobra prints the error; main only needs the
// exit status.
func Execute(info BuildInfo) error {
	return NewRootCommand(info).Execute()
}

// notImplemented is the placeholder body for a command whose build phase has
// not landed yet. Each is replaced by the real implementation in the phase that
// owns it (deadwood-spec.md Section 12).
func notImplemented(phase int) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		fmt.Fprintf(cmd.ErrOrStderr(),
			"%s: not implemented yet (build phase %d)\n", cmd.CommandPath(), phase)
		return nil
	}
}
