package git

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrDetachedHead reports that the repository has no branch checked out.
var ErrDetachedHead = errors.New("HEAD is detached")

// BranchInfo is the per-branch metadata a single for-each-ref pass can supply.
// Worktree membership, stash association and ahead/behind counts each need
// their own git call and are joined onto this at the cli layer, which is the
// only place allowed to combine the git, github and classify packages.
type BranchInfo struct {
	Name           string
	IsCurrent      bool
	LastCommitSHA  string
	LastCommitDate time.Time
	HasUpstream    bool
	UpstreamName   string // e.g. origin/feature-x
}

// branchFormat pulls every field ListLocalBranches needs in one pass. Splitting
// this into per-branch calls would cost one process spawn per branch, and the
// tool is aimed at clones with hundreds of them.
var branchFormat = strings.Join([]string{
	"%(refname:short)",
	"%(objectname)",
	"%(committerdate:iso-strict)",
	"%(upstream:short)",
	"%(HEAD)",
}, fieldSepForEachRef)

const branchFieldCount = 5

// ListLocalBranches returns every branch under refs/heads, sorted by name. A
// repository with no commits yet reports no branches rather than an error.
func ListLocalBranches(repoPath string) ([]BranchInfo, error) {
	out, err := run(repoPath, "for-each-ref", "--sort=refname", "--format="+branchFormat, "refs/heads/")
	if err != nil {
		return nil, err
	}

	records := lines(out)
	branches := make([]BranchInfo, 0, len(records))
	for _, record := range records {
		branch, err := parseBranchRecord(record)
		if err != nil {
			return nil, err
		}
		branches = append(branches, branch)
	}
	return branches, nil
}

func parseBranchRecord(record string) (BranchInfo, error) {
	fields := strings.Split(record, fieldSep)
	if len(fields) != branchFieldCount {
		return BranchInfo{}, fmt.Errorf("parsing branch record %q: got %d fields, want %d",
			record, len(fields), branchFieldCount)
	}

	committed, err := time.Parse(time.RFC3339, fields[2])
	if err != nil {
		return BranchInfo{}, fmt.Errorf("parsing commit date for branch %q: %w", fields[0], err)
	}

	return BranchInfo{
		Name:           fields[0],
		LastCommitSHA:  fields[1],
		LastCommitDate: committed,
		HasUpstream:    fields[3] != "",
		UpstreamName:   fields[3],
		// for-each-ref marks the checked-out branch with an asterisk.
		IsCurrent: fields[4] == "*",
	}, nil
}

// CurrentBranch returns the checked-out branch name, or ErrDetachedHead when
// HEAD does not point at one.
func CurrentBranch(repoPath string) (string, error) {
	out, err := run(repoPath, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if code, ok := exitCodeOf(err); ok && code == 1 {
			return "", ErrDetachedHead
		}
		return "", err
	}
	return out, nil
}
