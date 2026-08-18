package git

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListLocalBranchesReportsMetadata(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	featureSHA := r.branchWithCommit("feature", "feature work")
	r.addOrigin(defaultBranchName)
	r.git("push", "--set-upstream", remoteName, "feature")

	branches, err := ListLocalBranches(r.dir)
	require.NoError(t, err)
	require.Len(t, branches, 2)

	assert.Equal(t, []string{"feature", defaultBranchName},
		[]string{branches[0].Name, branches[1].Name}, "branches should be sorted by name")

	byName := indexByName(branches)

	base := byName[defaultBranchName]
	assert.True(t, base.IsCurrent, "the checked-out branch should be marked current")
	assert.False(t, base.HasUpstream)
	assert.Empty(t, base.UpstreamName)
	assert.Equal(t, r.commitTime(1).Unix(), base.LastCommitDate.Unix())

	feature := byName["feature"]
	assert.False(t, feature.IsCurrent)
	assert.True(t, feature.HasUpstream)
	assert.Equal(t, remoteName+"/feature", feature.UpstreamName)
	assert.Equal(t, featureSHA, feature.LastCommitSHA)
	assert.Equal(t, r.commitTime(2).Unix(), feature.LastCommitDate.Unix())
}

func TestListLocalBranchesOnRepoWithoutCommits(t *testing.T) {
	t.Parallel()

	branches, err := ListLocalBranches(newEmptyRepo(t).dir)

	require.NoError(t, err)
	assert.Empty(t, branches)
}

func TestListLocalBranchesHandlesSlashedNames(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feat/nested/deeply", "work")

	branches, err := ListLocalBranches(r.dir)

	require.NoError(t, err)
	assert.Contains(t, indexByName(branches), "feat/nested/deeply")
}

func TestCurrentBranch(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.git("checkout", "-b", "feature")

	got, err := CurrentBranch(r.dir)

	require.NoError(t, err)
	assert.Equal(t, "feature", got)
}

func TestCurrentBranchDetachedHead(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.git("checkout", "--detach", "HEAD")

	_, err := CurrentBranch(r.dir)

	assert.ErrorIs(t, err, ErrDetachedHead)
}

func TestParseBranchRecord(t *testing.T) {
	t.Parallel()

	record := func(fields ...string) string { return strings.Join(fields, fieldSep) }

	t.Run("marks the current branch", func(t *testing.T) {
		t.Parallel()

		got, err := parseBranchRecord(record("main", "abc123", "2026-03-01T13:00:00+00:00", "", "*"))

		require.NoError(t, err)
		assert.True(t, got.IsCurrent)
		assert.Equal(t, "main", got.Name)
		assert.Equal(t, "abc123", got.LastCommitSHA)
	})

	malformed := map[string]string{
		"too few fields":   record("main", "abc123"),
		"too many fields":  record("main", "abc123", "2026-03-01T13:00:00+00:00", "", " ", "extra"),
		"unparseable date": record("main", "abc123", "last tuesday", "", " "),
	}
	for name, input := range malformed {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := parseBranchRecord(input)

			assert.Error(t, err, "malformed output must not be silently accepted")
		})
	}
}
