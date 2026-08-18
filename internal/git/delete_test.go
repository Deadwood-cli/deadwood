package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteBranchRemovesAMergedBranch(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	r.mergeNoFF("feature")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	require.NoError(t, DeleteBranch(r.dir, "feature", false))

	requireBranchPresence(t, r, "feature", false)
}

// The sequencing rule from spec Section 6 is enforced inside DeleteBranch, so
// no caller can delete first and back up afterwards.
func TestDeleteBranchRefusesWithoutABackup(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	r.mergeNoFF("feature")

	err := DeleteBranch(r.dir, "feature", false)

	assert.ErrorIs(t, err, ErrBackupMissing)
	requireBranchPresence(t, r, "feature", true)
}

// A backup taken before the branch moved on would restore the wrong commit, so
// it counts as no backup at all.
func TestDeleteBranchRefusesAStaleBackup(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	r.checkout("feature")
	r.commit("work taken after the backup")
	r.checkout(defaultBranchName)
	r.mergeNoFF("feature")

	err := DeleteBranch(r.dir, "feature", false)

	assert.ErrorIs(t, err, ErrBackupStale)
	requireBranchPresence(t, r, "feature", true)
}

// Safety invariant, spec Section 11 and AGENTS.md Section 1: no code path may
// produce a `git branch -D`. The parameter exists only to match the specified
// signature, and passing it is an error rather than an escalation.
func TestDeleteBranchRejectsForce(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "unmerged work")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	err := DeleteBranch(r.dir, "feature", true)

	assert.ErrorIs(t, err, ErrForceDeleteDisabled)
	requireBranchPresence(t, r, "feature", true)
}

// Git's own refusal is the last line of defence behind deadwood's
// classification. When it fires, the branch is reported and left alone.
func TestDeleteBranchReportsUnmergedRatherThanEscalating(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "unmerged work")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	err := DeleteBranch(r.dir, "feature", false)

	assert.ErrorIs(t, err, ErrNotFullyMerged)
	requireBranchPresence(t, r, "feature", true)
}

func TestDeleteBranchRejectsFlagLikeNames(t *testing.T) {
	t.Parallel()

	err := DeleteBranch(newRepo(t).dir, "--all", false)

	assert.Error(t, err)
}

func TestDeleteBranchOnUnknownBranch(t *testing.T) {
	t.Parallel()

	err := DeleteBranch(newRepo(t).dir, "no-such-branch", false)

	assert.Error(t, err)
}
