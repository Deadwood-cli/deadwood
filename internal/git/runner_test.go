package git

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommandEnvStripsGitHubToken(t *testing.T) {
	t.Setenv("DEADWOOD_GITHUB_TOKEN", "gho_shouldnotleak1234")

	for _, mutating := range []bool{false, true} {
		env := commandEnv(mutating)
		for _, kv := range env {
			assert.NotContains(t, kv, "gho_shouldnotleak1234")
			key, _, _ := strings.Cut(kv, "=")
			assert.False(t, strings.EqualFold(key, "DEADWOOD_GITHUB_TOKEN"), kv)
		}
	}
}

func TestCommandEnvOptionalLocksOnlyOnReads(t *testing.T) {
	assert.Contains(t, commandEnv(false), "GIT_OPTIONAL_LOCKS=0")
	for _, kv := range commandEnv(true) {
		assert.NotEqual(t, "GIT_OPTIONAL_LOCKS=0", kv)
	}
}

func TestCommandEnvPreservesExistingCeiling(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", "/tmp/deadwood-ceiling")

	assert.Contains(t, commandEnv(false), "GIT_CEILING_DIRECTORIES=/tmp/deadwood-ceiling")
}

func TestRedactSecrets(t *testing.T) {
	got := redactSecrets("helper: gho_abcdefghijklmnopqrstuvwxyz stderr")
	assert.NotContains(t, got, "gho_abcdefghijklmnopqrstuvwxyz")
	assert.Contains(t, got, "[redacted]")
}

func TestCommandErrorRedactsStderr(t *testing.T) {
	err := &CommandError{
		Args:     []string{"branch", "-d", "x"},
		ExitCode: 1,
		Stderr:   redactSecrets("fatal: ghp_abcdefghijklmnopqrstuvwxyz"),
	}
	assert.NotContains(t, err.Error(), "ghp_")
	assert.Contains(t, err.Error(), "[redacted]")
}

func TestGitBinaryResolves(t *testing.T) {
	path := gitBinary()
	require.NotEmpty(t, path)
	assert.NotContains(t, path, "\x00")
}
