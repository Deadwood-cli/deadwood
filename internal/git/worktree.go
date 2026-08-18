package git

import (
	"os"
	"strings"
)

// ListWorktrees returns the branches checked out in worktrees other than the
// one at repoPath. Git refuses to delete a branch that is checked out anywhere,
// so these are protected unconditionally.
func ListWorktrees(repoPath string) ([]string, error) {
	out, err := run(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	// Failing to resolve our own worktree is not fatal: reporting a branch as
	// checked out elsewhere only ever protects it, which is the safe direction.
	root, rootErr := RepoRoot(repoPath)

	var (
		branches []string
		path     string
	)
	for _, line := range lines(out) {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			if rootErr != nil || !samePath(path, root) {
				branches = append(branches, strings.TrimPrefix(ref, "refs/heads/"))
			}
		}
	}
	return branches, nil
}

// samePath reports whether two paths refer to the same directory. Comparing the
// strings is not enough: git prints forward slashes, and Windows temp paths can
// reach the same directory through different casing or 8.3 short names.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aInfo, err := os.Stat(a)
	if err != nil {
		return false
	}
	bInfo, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(aInfo, bInfo)
}
