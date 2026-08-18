package git

import (
	"fmt"
	"strings"
)

// StashEntry is one record from `git stash list`.
type StashEntry struct {
	Ref     string // e.g. stash@{0}
	Message string // e.g. "WIP on feature-x: 1a2b3c4 subject"
}

var stashFormat = strings.Join([]string{"%gd", "%s"}, fieldSepPretty)

const stashFieldCount = 2

// ListStashRefs returns the stash entries in the repository, newest first. A
// repository with no stash reports none rather than an error.
func ListStashRefs(repoPath string) ([]StashEntry, error) {
	out, err := run(repoPath, "stash", "list", "--format="+stashFormat)
	if err != nil {
		return nil, err
	}

	records := lines(out)
	entries := make([]StashEntry, 0, len(records))
	for _, record := range records {
		fields := strings.Split(record, fieldSep)
		if len(fields) != stashFieldCount {
			return nil, fmt.Errorf("parsing stash record %q: got %d fields, want %d",
				record, len(fields), stashFieldCount)
		}
		entries = append(entries, StashEntry{Ref: fields[0], Message: fields[1]})
	}
	return entries, nil
}

// BranchesWithStash reports which of the given branches appear in a stash
// message.
//
// Git does not record the owning branch of a stash in a way that is reliable to
// read back across versions, so this matches the branch name as a substring of
// the auto-generated message ("WIP on <branch>: ..."). It is a deliberately
// blunt heuristic: a branch named "api" is also matched by a stash taken on
// "api-v2". Over-protecting a branch costs the user one manual delete, while
// under-protecting one loses work, so the bias runs towards false positives
// (spec Section 5.1).
func BranchesWithStash(entries []StashEntry, branchNames []string) map[string]bool {
	stashed := make(map[string]bool, len(branchNames))
	for _, name := range branchNames {
		if name == "" {
			continue
		}
		for _, entry := range entries {
			if strings.Contains(entry.Message, name) {
				stashed[name] = true
				break
			}
		}
	}
	return stashed
}
