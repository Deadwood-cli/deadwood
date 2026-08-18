package git

import (
	"fmt"
	"strings"
)

const remoteName = "origin"

// ListRemoteBranches returns the set of branch names present on origin.
//
// This is one network round trip for the whole repository. Callers deciding the
// fate of more than one branch must use this and test set membership locally;
// calling RemoteBranchExists per branch would make a scan of a few hundred
// branches a few hundred round trips (spec Section 6).
func ListRemoteBranches(repoPath string) (map[string]struct{}, error) {
	out, err := run(repoPath, "ls-remote", "--heads", remoteName)
	if err != nil {
		return nil, err
	}

	records := lines(out)
	branches := make(map[string]struct{}, len(records))
	for _, record := range records {
		name, err := parseLsRemoteRecord(record)
		if err != nil {
			return nil, err
		}
		branches[name] = struct{}{}
	}
	return branches, nil
}

// RemoteBranchExists reports whether a single branch is present on origin.
// Prefer ListRemoteBranches when checking more than one.
func RemoteBranchExists(repoPath, branch string) (bool, error) {
	if err := validateRefArgument(branch); err != nil {
		return false, err
	}

	// The refs/heads/ prefix keeps the pattern from also matching a branch
	// whose name merely ends with the one we asked about.
	out, err := run(repoPath, "ls-remote", "--heads", remoteName, "refs/heads/"+branch)
	if err != nil {
		return false, err
	}
	for _, record := range lines(out) {
		name, err := parseLsRemoteRecord(record)
		if err != nil {
			return false, err
		}
		if name == branch {
			return true, nil
		}
	}
	return false, nil
}

// parseLsRemoteRecord reads a "<sha>\trefs/heads/<name>" line.
func parseLsRemoteRecord(record string) (string, error) {
	_, ref, found := strings.Cut(record, "\t")
	if !found {
		return "", fmt.Errorf("parsing ls-remote record %q: no tab separator", record)
	}
	name := strings.TrimPrefix(ref, "refs/heads/")
	if name == ref {
		return "", fmt.Errorf("parsing ls-remote record %q: ref is not under refs/heads/", record)
	}
	return name, nil
}
