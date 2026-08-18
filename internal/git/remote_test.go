package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// originFixture builds a repo whose origin holds main and feature, plus a
// local-only branch that was never pushed.
func originFixture(t *testing.T) *repo {
	t.Helper()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	r.addOrigin(defaultBranchName, "feature")
	r.branchWithCommit("local-only", "work")
	return r
}

func TestListRemoteBranches(t *testing.T) {
	t.Parallel()

	got, err := ListRemoteBranches(originFixture(t).dir)

	require.NoError(t, err)
	assert.Equal(t, map[string]struct{}{defaultBranchName: {}, "feature": {}}, got)
}

func TestRemoteBranchExists(t *testing.T) {
	t.Parallel()

	r := originFixture(t)

	tests := map[string]struct {
		branch string
		want   bool
	}{
		"pushed branch":     {branch: "feature", want: true},
		"default branch":    {branch: defaultBranchName, want: true},
		"never pushed":      {branch: "local-only", want: false},
		"deleted on remote": {branch: "gone", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := RemoteBranchExists(r.dir, tc.branch)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseLsRemoteRecord(t *testing.T) {
	t.Parallel()

	t.Run("reads the branch name", func(t *testing.T) {
		t.Parallel()

		got, err := parseLsRemoteRecord("1a2b3c4\trefs/heads/feat/thing")

		require.NoError(t, err)
		assert.Equal(t, "feat/thing", got)
	})

	malformed := map[string]string{
		"no tab separator": "1a2b3c4 refs/heads/main",
		"not a branch ref": "1a2b3c4\trefs/tags/v1.0.0",
	}
	for name, record := range malformed {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := parseLsRemoteRecord(record)

			assert.Error(t, err)
		})
	}
}
