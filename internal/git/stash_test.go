package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListStashRefs(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.git("checkout", "-b", "stashed-branch")
	r.writeFile("file-01.txt", "an uncommitted change")
	r.git("stash")

	entries, err := ListStashRefs(r.dir)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "stash@{0}", entries[0].Ref)
	assert.Contains(t, entries[0].Message, "stashed-branch",
		"git's auto-generated message is what the branch heuristic matches on")
}

func TestListStashRefsWithoutStashes(t *testing.T) {
	t.Parallel()

	entries, err := ListStashRefs(newRepo(t).dir)

	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestBranchesWithStash(t *testing.T) {
	t.Parallel()

	entries := []StashEntry{
		{Ref: "stash@{0}", Message: "WIP on api-v2: 1a2b3c4 tidy handlers"},
		{Ref: "stash@{1}", Message: "On release/1.4: hotfix in progress"},
	}

	tests := map[string]struct {
		branches []string
		want     map[string]bool
	}{
		"exact match on a stashed branch": {
			branches: []string{"api-v2"},
			want:     map[string]bool{"api-v2": true},
		},
		"custom stash message still matches": {
			branches: []string{"release/1.4"},
			want:     map[string]bool{"release/1.4": true},
		},
		"branch with no stash is absent": {
			branches: []string{"unrelated"},
			want:     map[string]bool{},
		},
		// Documented over-protection: "api" is a substring of "api-v2", so it is
		// flagged too. Costing the user a manual delete beats losing a stash.
		"substring of a stashed branch is over-protected": {
			branches: []string{"api"},
			want:     map[string]bool{"api": true},
		},
		"empty names are skipped": {
			branches: []string{""},
			want:     map[string]bool{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, BranchesWithStash(entries, tc.branches))
		})
	}
}
