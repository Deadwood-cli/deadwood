package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsAncestor(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("merged", "merged work")
	r.branchWithCommit("unmerged", "unmerged work")
	r.mergeNoFF("merged")

	tests := map[string]struct {
		branch string
		want   bool
	}{
		"merged branch is contained in the default branch": {branch: "merged", want: true},
		"unmerged branch is not":                           {branch: "unmerged", want: false},
		"a branch is its own ancestor":                     {branch: defaultBranchName, want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := IsAncestor(r.dir, tc.branch, defaultBranchName)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// An unknown ref must be an error rather than a quiet false, which would read
// as "not merged" and could push a branch into the wrong bucket.
func TestIsAncestorUnknownRef(t *testing.T) {
	t.Parallel()

	_, err := IsAncestor(newRepo(t).dir, "no-such-branch", defaultBranchName)

	assert.Error(t, err)
}

func TestIsAncestorRejectsFlagLikeNames(t *testing.T) {
	t.Parallel()

	_, err := IsAncestor(newRepo(t).dir, "--all", defaultBranchName)

	assert.Error(t, err)
}

func TestAheadBehind(t *testing.T) {
	t.Parallel()

	// main    c1 --------- c3 ---------- M
	// feature   \- c2                  /
	// merged                \- c4 ----/
	//
	// So main holds c1, c3, c4 and the merge commit M; feature holds c1 and c2;
	// merged holds c1, c3 and c4.
	r := newRepo(t)
	r.branchWithCommit("feature", "feature work")
	r.commit("main moves on")
	r.branchWithCommit("merged", "merged work")
	r.mergeNoFF("merged")

	tests := map[string]struct {
		branch     string
		wantAhead  int
		wantBehind int
	}{
		"diverged branch counts both sides": {branch: "feature", wantAhead: 1, wantBehind: 3},
		"merged branch is only behind":      {branch: "merged", wantAhead: 0, wantBehind: 1},
		"default branch is level":           {branch: defaultBranchName},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ahead, behind, err := AheadBehind(r.dir, tc.branch, defaultBranchName)

			require.NoError(t, err)
			assert.Equal(t, tc.wantAhead, ahead, "ahead count")
			assert.Equal(t, tc.wantBehind, behind, "behind count")
		})
	}
}

func TestAheadBehindUnknownRef(t *testing.T) {
	t.Parallel()

	_, _, err := AheadBehind(newRepo(t).dir, "no-such-branch", defaultBranchName)

	assert.Error(t, err)
}
