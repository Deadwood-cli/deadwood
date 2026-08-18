package git

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepoRootFromSubdirectory(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	nested := filepath.Join(r.dir, "pkg", "deep")
	require.NoError(t, os.MkdirAll(nested, 0o750))

	root, err := RepoRoot(nested)

	require.NoError(t, err)
	// Compared by identity rather than as strings: macOS resolves the temp
	// directory through a symlink, so the two paths differ textually.
	assert.True(t, samePath(root, r.dir), "RepoRoot(%q) = %q, want the repo root %q", nested, root, r.dir)
}

func TestRepoRootOutsideRepository(t *testing.T) {
	t.Parallel()

	_, err := RepoRoot(t.TempDir())

	assert.Error(t, err)
}

func TestGetDefaultBranchFromOriginHead(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.addOrigin(defaultBranchName)
	r.setOriginHead(defaultBranchName)

	got, err := GetDefaultBranch(r.dir)

	require.NoError(t, err)
	assert.Equal(t, defaultBranchName, got)
}

// Guessing a default branch would put every branch in the repository at risk,
// so an unset origin/HEAD has to surface as a distinguishable error the caller
// can fall through on.
func TestGetDefaultBranchWithoutOriginHead(t *testing.T) {
	t.Parallel()

	_, err := GetDefaultBranch(newRepo(t).dir)

	assert.ErrorIs(t, err, ErrDefaultBranchUnknown)
}

func TestLocalBranchExists(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")

	tests := map[string]struct {
		branch string
		want   bool
	}{
		"existing branch":       {branch: "feature", want: true},
		"default branch":        {branch: defaultBranchName, want: true},
		"branch never created":  {branch: "nope", want: false},
		"remote-style ref name": {branch: "origin/feature", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := LocalBranchExists(r.dir, tc.branch)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestValidateRefArgument(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name    string
		wantErr bool
	}{
		"ordinary name":     {name: "feature"},
		"slashed name":      {name: "feat/thing"},
		"empty name":        {name: "", wantErr: true},
		"leading dash":      {name: "-D", wantErr: true},
		"long option shape": {name: "--force", wantErr: true},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()

			err := validateRefArgument(tc.name)

			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
