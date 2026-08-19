package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/Deadwood-cli/deadwood/internal/auth"
	ghub "github.com/Deadwood-cli/deadwood/internal/github"
	"github.com/spf13/cobra"
)

const authLong = `Manage the GitHub credentials deadwood uses to check remote branches and match
squash-merged pull requests.

Tokens are stored in the OS keychain, never on disk in plaintext. In environments
without a keychain, set DEADWOOD_GITHUB_TOKEN instead.`

type authRuntime struct {
	store     *auth.Store
	flow      *ghub.DeviceFlow
	login     func(ctx context.Context, token string) (string, error)
	clientID  func() (string, error)
	lookupEnv func(string) (string, bool)
}

func defaultAuthRuntime() *authRuntime {
	return &authRuntime{
		store: auth.DefaultStore(),
		flow:  ghub.NewDeviceFlow(nil),
		login: func(ctx context.Context, token string) (string, error) {
			return ghub.AuthenticatedLogin(ctx, token, ghub.ClientOptions{})
		},
		clientID:  ghub.ResolveClientID,
		lookupEnv: os.LookupEnv,
	}
}

func newAuthCommand(_ *globalOptions, runtime *authRuntime) *cobra.Command {
	if runtime == nil {
		runtime = defaultAuthRuntime()
	}

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
			RunE: func(cmd *cobra.Command, _ []string) error {
				return runAuthLogin(cmd, runtime)
			},
		},
		&cobra.Command{
			Use:   "logout",
			Short: "Remove the stored GitHub token from the keychain",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return runAuthLogout(cmd, runtime)
			},
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show whether a stored GitHub token is present and valid",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return runAuthStatus(cmd, runtime)
			},
		},
	)

	return cmd
}

func runAuthLogin(cmd *cobra.Command, runtime *authRuntime) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	clientID, err := runtime.clientID()
	if err != nil {
		return err
	}

	code, err := runtime.flow.RequestCode(ctx, clientID)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Open %s and enter the code:\n\n  %s\n\nWaiting for authorization...\n",
		code.VerificationURI, code.UserCode)

	token, err := runtime.flow.WaitForToken(ctx, clientID, code)
	if err != nil {
		return err
	}

	if err := runtime.store.Set(token); err != nil {
		// The token is in memory only. Do not print it.
		return fmt.Errorf("authorization succeeded but the OS keychain refused to store the token: %w (set %s in this environment instead)", err, auth.EnvToken)
	}

	login, err := runtime.login(ctx, token)
	if err != nil {
		fmt.Fprintf(out, "token stored in the OS keychain, but could not confirm it: %v\n", err)
		return nil
	}
	fmt.Fprintf(out, "logged in as %s\n", login)
	return nil
}

func runAuthLogout(cmd *cobra.Command, runtime *authRuntime) error {
	if err := runtime.store.Delete(); err != nil {
		return err
	}
	if _, ok := runtime.lookupEnv(auth.EnvToken); ok {
		fmt.Fprintf(cmd.ErrOrStderr(), "keychain entry removed; %s is still set in this environment\n", auth.EnvToken)
		return nil
	}
	fmt.Fprintln(cmd.OutOrStdout(), "logged out")
	return nil
}

func runAuthStatus(cmd *cobra.Command, runtime *authRuntime) error {
	token, err := runtime.store.Get()
	if errors.Is(err, auth.ErrNotFound) {
		fmt.Fprintln(cmd.OutOrStdout(), "not logged in")
		return nil
	}
	if err != nil {
		return err
	}

	if token.FromEnv {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: using %s; this is less secure than the OS keychain\n", auth.EnvToken)
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	login, err := runtime.login(ctx, token.Value)
	if err != nil {
		return err
	}

	source := "OS keychain"
	if token.FromEnv {
		source = auth.EnvToken
	}
	fmt.Fprintf(cmd.OutOrStdout(), "logged in as %s (%s)\n", login, source)
	return nil
}
