package cli

import "github.com/spf13/cobra"

const authLong = `Manage the GitHub credentials deadwood uses to check remote branches and match
squash-merged pull requests.

Tokens are stored in the OS keychain, never on disk in plaintext. In environments
without a keychain, set DEADWOOD_GITHUB_TOKEN instead.`

func newAuthCommand(_ *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage GitHub authentication",
		Long:  authLong,
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		&cobra.Command{
			Use:   "login",
			Short: "Authorise deadwood with GitHub via device flow",
			Args:  cobra.NoArgs,
			RunE:  notImplemented(5),
		},
		&cobra.Command{
			Use:   "logout",
			Short: "Remove the stored GitHub token from the keychain",
			Args:  cobra.NoArgs,
			RunE:  notImplemented(5),
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show whether a stored GitHub token is present and valid",
			Args:  cobra.NoArgs,
			RunE:  notImplemented(5),
		},
	)

	return cmd
}
