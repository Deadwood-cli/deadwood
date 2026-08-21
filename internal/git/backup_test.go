package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateBackupRefRecordsBranchTip(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	tip := r.branchWithCommit("feature", "work")

	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	assert.Equal(t, tip, r.git("rev-parse", backupRefName("feature")))
}

func TestCreateBackupRefFollowsALaterTip(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	r.checkout("feature")
	later := r.commit("more work")
	r.checkout(defaultBranchName)
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	assert.Equal(t, later, r.git("rev-parse", backupRefName("feature")))
}

func TestCreateBackupRefForUnknownBranch(t *testing.T) {
	t.Parallel()

	assert.Error(t, CreateBackupRef(newRepo(t).dir, "no-such-branch"))
}

func TestListBackupRefs(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feat/one", "one")
	r.branchWithCommit("two", "two")
	require.NoError(t, CreateBackupRef(r.dir, "feat/one"))
	require.NoError(t, CreateBackupRef(r.dir, "two"))

	got, err := ListBackupRefs(r.dir)

	require.NoError(t, err)
	assert.Equal(t, []string{"feat/one", "two"}, got)
}

func TestListBackupDetailsIncludesCommitSubject(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	tip := r.branchWithCommit("feature", "work worth keeping")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	got, err := ListBackupDetails(r.dir)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "feature", got[0].Branch)
	assert.Equal(t, tip, got[0].SHA)
	assert.Equal(t, "work worth keeping", got[0].Subject)
	assert.False(t, got[0].CommitDate.IsZero())
}

func TestListBackupRefsWhenNoneExist(t *testing.T) {
	t.Parallel()

	got, err := ListBackupRefs(newRepo(t).dir)

	require.NoError(t, err)
	assert.Empty(t, got)
}

// The delete-then-restore round trip is the guarantee the whole product rests
// on, so it is asserted end to end rather than in halves.
func TestBackupSurvivesDeleteAndRestoresTheSameCommit(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	tip := r.branchWithCommit("feature", "work worth keeping")
	r.mergeNoFF("feature")

	require.NoError(t, CreateBackupRef(r.dir, "feature"))
	require.NoError(t, DeleteBranch(r.dir, "feature", false))
	requireBranchPresence(t, r, "feature", false)

	require.NoError(t, RestoreFromBackup(r.dir, "feature"))

	requireBranchPresence(t, r, "feature", true)
	assert.Equal(t, tip, r.git("rev-parse", "refs/heads/feature"),
		"the restored branch must point at exactly the commit that was deleted")
}

func TestRestoreFromBackupWithoutABackup(t *testing.T) {
	t.Parallel()

	r := newRepo(t)

	err := RestoreFromBackup(r.dir, "never-backed-up")

	assert.ErrorIs(t, err, ErrBackupMissing)
}

func TestRestoreFromBackupRefusesToClobberAnExistingBranch(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	r.checkout("feature")
	newer := r.commit("work done since the backup")
	r.checkout(defaultBranchName)

	err := RestoreFromBackup(r.dir, "feature")

	require.Error(t, err)
	assert.Equal(t, newer, r.git("rev-parse", "refs/heads/feature"),
		"restoring must not rewind a branch that still exists")
}

func TestDeleteBackupRef(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("feature", "work")
	require.NoError(t, CreateBackupRef(r.dir, "feature"))

	require.NoError(t, DeleteBackupRef(r.dir, "feature"))

	got, err := ListBackupRefs(r.dir)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestVerifyBackupCoversTip(t *testing.T) {
	t.Parallel()

	t.Run("passes when the backup matches", func(t *testing.T) {
		t.Parallel()

		r := newRepo(t)
		r.branchWithCommit("feature", "work")
		require.NoError(t, CreateBackupRef(r.dir, "feature"))

		assert.NoError(t, verifyBackupCoversTip(r.dir, "feature"))
	})

	t.Run("reports a missing backup", func(t *testing.T) {
		t.Parallel()

		r := newRepo(t)
		r.branchWithCommit("feature", "work")

		assert.ErrorIs(t, verifyBackupCoversTip(r.dir, "feature"), ErrBackupMissing)
	})

	t.Run("reports a backup left behind by newer commits", func(t *testing.T) {
		t.Parallel()

		r := newRepo(t)
		r.branchWithCommit("feature", "work")
		require.NoError(t, CreateBackupRef(r.dir, "feature"))
		r.checkout("feature")
		r.commit("work the backup does not cover")
		r.checkout(defaultBranchName)

		assert.ErrorIs(t, verifyBackupCoversTip(r.dir, "feature"), ErrBackupStale)
	})
}
