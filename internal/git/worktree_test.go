package git

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWorktreesReportsBranchesCheckedOutElsewhere(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.branchWithCommit("in-worktree", "work")
	r.branchWithCommit("ordinary", "work")
	r.git("worktree", "add", filepath.Join(t.TempDir(), "linked"), "in-worktree")

	got, err := ListWorktrees(r.dir)

	require.NoError(t, err)
	// The branch checked out in the main worktree is reported as current
	// instead, so only the linked worktree's branch belongs here.
	assert.Equal(t, []string{"in-worktree"}, got)
}

func TestListWorktreesWithoutLinkedWorktrees(t *testing.T) {
	t.Parallel()

	got, err := ListWorktrees(newRepo(t).dir)

	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListWorktreesIgnoresDetachedWorktrees(t *testing.T) {
	t.Parallel()

	r := newRepo(t)
	r.git("worktree", "add", "--detach", filepath.Join(t.TempDir(), "detached"))

	got, err := ListWorktrees(r.dir)

	require.NoError(t, err)
	assert.Empty(t, got, "a detached worktree holds no branch to protect")
}
