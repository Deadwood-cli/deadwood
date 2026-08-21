package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testBuildInfo() BuildInfo {
	return BuildInfo{Version: "0.0.0-test", Commit: "deadbee", Date: "2026-01-01"}
}

// run executes the command tree against args and captures its output. The args
// slice must be non-nil even when empty, or cobra falls back to os.Args and
// picks up the test binary's own flags.
func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	root := NewRootCommand(testBuildInfo())
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{}, args...))

	err = root.Execute()
	return out.String(), errOut.String(), err
}

func findCommand(t *testing.T, path ...string) *cobra.Command {
	t.Helper()

	cmd, _, err := NewRootCommand(testBuildInfo()).Find(path)
	require.NoError(t, err)
	require.Equal(t, path[len(path)-1], cmd.Name())
	return cmd
}

// Safety invariant, spec Section 11: clean must default to doing nothing.
// Changing this default requires an explicit instruction from the project owner.
func TestCleanDryRunDefaultsToTrue(t *testing.T) {
	flag := findCommand(t, "clean").Flags().Lookup("dry-run")

	require.NotNil(t, flag, "clean must expose a --dry-run flag")
	assert.Equal(t, "true", flag.DefValue)
}

// Safety invariant, spec Section 11: force deletion is never offered as an
// option, so no code path can escalate a failed `git branch -d`.
func TestCleanExposesNoForceFlag(t *testing.T) {
	flags := findCommand(t, "clean").Flags()

	for _, name := range []string{"force", "hard", "no-backup"} {
		assert.Nil(t, flags.Lookup(name), "clean must not expose a --%s flag", name)
	}
	assert.Nil(t, flags.ShorthandLookup("D"), "clean must not expose a -D shorthand")
}

func TestRootRegistersSpecifiedCommands(t *testing.T) {
	tests := []struct {
		path  []string
		flags []string
	}{
		{path: []string{"scan"}, flags: []string{"json"}},
		{path: []string{"clean"}, flags: []string{"dry-run", "yes", "include-needs-review"}},
		{path: []string{"undo"}, flags: []string{"list"}},
		{path: []string{"auth", "login"}},
		{path: []string{"auth", "logout"}},
		{path: []string{"auth", "status"}},
	}

	for _, tc := range tests {
		name := tc.path[len(tc.path)-1]
		t.Run(name, func(t *testing.T) {
			cmd := findCommand(t, tc.path...)
			assert.NotEmpty(t, cmd.Short, "every command needs a short description")

			for _, flag := range tc.flags {
				assert.NotNil(t, cmd.Flags().Lookup(flag), "missing --%s", flag)
			}
		})
	}
}

func TestGlobalFlagsAreAcceptedBySubcommands(t *testing.T) {
	for _, name := range []string{"scan", "clean", "undo"} {
		t.Run(name, func(t *testing.T) {
			cmd := findCommand(t, name)
			assert.NotNil(t, cmd.InheritedFlags().Lookup("config"), "%s missing --config", name)
			assert.NotNil(t, cmd.InheritedFlags().Lookup("verbose"), "%s missing --verbose", name)
		})
	}
}

func TestVersionFlagReportsBuildInfo(t *testing.T) {
	stdout, _, err := run(t, "--version")

	require.NoError(t, err)
	assert.Equal(t, "deadwood 0.0.0-test (commit deadbee, built 2026-01-01)\n", stdout)
}

func TestUnknownCommandFails(t *testing.T) {
	_, _, err := run(t, "delete-everything")

	assert.Error(t, err)
}
