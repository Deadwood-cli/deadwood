package cli

import (
	"testing"

	"github.com/Deadwood-cli/deadwood/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUndoRestoresDeletedBranch(t *testing.T) {
	r := newTestRepo(t)
	r.branchWithCommit("feature", "work worth keeping")
	tip := r.git("rev-parse", "refs/heads/feature")
	r.mergeNoFF("feature")
	require.NoError(t, git.CreateBackupRef(r.dir, "feature"))
	require.NoError(t, git.DeleteBranch(r.dir, "feature", false))
	chdir(t, r.dir)

	stdout, _, err := runUndoCLI(t, "undo", "feature")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored feature from refs/deadwood-backup/feature")

	got := r.git("rev-parse", "refs/heads/feature")
	assert.Equal(t, tip, got)
}

func TestUndoListShowsBackupContext(t *testing.T) {
	r := newTestRepo(t)
	r.branchWithCommit("feature", "work worth keeping")
	require.NoError(t, git.CreateBackupRef(r.dir, "feature"))
	chdir(t, r.dir)

	stdout, _, err := runUndoCLI(t, "undo", "--list")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Available backups:")
	assert.Contains(t, stdout, "feature")
	assert.Contains(t, stdout, "work worth keeping")
}

func TestUndoWithoutNameListsBackups(t *testing.T) {
	r := newTestRepo(t)
	chdir(t, r.dir)

	stdout, _, err := runUndoCLI(t, "undo")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No backup refs found.")
}

func TestUndoMissingNameListsAvailable(t *testing.T) {
	r := newTestRepo(t)
	r.branchWithCommit("feature", "work worth keeping")
	require.NoError(t, git.CreateBackupRef(r.dir, "feature"))
	chdir(t, r.dir)

	stdout, _, err := runUndoCLI(t, "undo", "no-such-branch")
	require.Error(t, err)
	assert.ErrorIs(t, err, git.ErrBackupMissing)
	assert.Contains(t, stdout, `No backup found for "no-such-branch"`)
	assert.Contains(t, stdout, "feature")
}

func TestUndoRefusesToClobberExistingBranch(t *testing.T) {
	r := newTestRepo(t)
	r.branchWithCommit("feature", "work")
	require.NoError(t, git.CreateBackupRef(r.dir, "feature"))
	chdir(t, r.dir)

	_, _, err := runUndoCLI(t, "undo", "feature")
	require.Error(t, err)
	r.git("show-ref", "--verify", "refs/heads/feature")
}

func runUndoCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return runCleanCLI(t, nil, nil, "", args...)
}
