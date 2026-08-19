package git

import (
	"errors"
	"strings"
)

const originHeadRef = "refs/remotes/origin/HEAD"

// ErrDefaultBranchUnknown reports that origin/HEAD is not set locally, so the
// default branch cannot be determined from the clone alone. Callers fall back
// to the GitHub API or an explicit config value (spec Section 5.2) rather than
// guessing.
var ErrDefaultBranchUnknown = errors.New("origin/HEAD is not set locally")

// RepoRoot returns the absolute path to the top level of the working tree
// containing path.
func RepoRoot(path string) (string, error) {
	return run(path, "rev-parse", "--show-toplevel")
}

// GetDefaultBranch resolves the default branch from origin/HEAD, the first of
// the three sources listed in spec Section 5.2. It returns
// ErrDefaultBranchUnknown when the ref is absent, and never guesses a name.
func GetDefaultBranch(repoPath string) (string, error) {
	out, err := run(repoPath, "symbolic-ref", "--quiet", originHeadRef)
	if err != nil {
		// symbolic-ref --quiet exits 1 when the ref does not exist or is not
		// symbolic, which is the ordinary "not configured" case.
		if code, ok := exitCodeOf(err); ok && code == 1 {
			return "", ErrDefaultBranchUnknown
		}
		return "", err
	}

	name := strings.TrimPrefix(out, "refs/remotes/origin/")
	if name == "" || name == out {
		return "", ErrDefaultBranchUnknown
	}
	return name, nil
}

// OriginURL returns the URL configured for the origin remote.
func OriginURL(repoPath string) (string, error) {
	return run(repoPath, "remote", "get-url", remoteName)
}

// LocalBranchExists reports whether refs/heads/<branch> is present.
func LocalBranchExists(repoPath, branch string) (bool, error) {
	if err := validateRefArgument(branch); err != nil {
		return false, err
	}
	return refExists(repoPath, "refs/heads/"+branch)
}

// refExists reports whether a fully qualified ref resolves.
func refExists(repoPath, ref string) (bool, error) {
	_, err := run(repoPath, "rev-parse", "--verify", "--quiet", ref)
	if err == nil {
		return true, nil
	}
	if code, ok := exitCodeOf(err); ok && code == 1 {
		return false, nil
	}
	return false, err
}

// resolveRef returns the object ID a fully qualified ref points at.
func resolveRef(repoPath, ref string) (string, error) {
	return run(repoPath, "rev-parse", "--verify", ref)
}
