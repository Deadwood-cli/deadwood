package git

import (
	"fmt"
	"strconv"
	"strings"
)

// IsAncestor reports whether branch's tip is reachable from of, meaning every
// commit on branch is already contained in of. This is the strongest merge
// evidence deadwood has and drives the high-confidence safe_delete bucket.
func IsAncestor(repoPath, branch, of string) (bool, error) {
	for _, ref := range []string{branch, of} {
		if err := validateRefArgument(ref); err != nil {
			return false, err
		}
	}

	// merge-base --is-ancestor answers through its exit status: 0 yes, 1 no,
	// anything else is a real failure such as an unknown ref.
	_, err := run(repoPath, "merge-base", "--is-ancestor", branch, of)
	if err == nil {
		return true, nil
	}
	if code, ok := exitCodeOf(err); ok && code == 1 {
		return false, nil
	}
	return false, err
}

// AheadBehind counts the commits unique to branch and unique to base. A branch
// that is behind but not ahead has contributed nothing base does not already
// have.
func AheadBehind(repoPath, branch, base string) (ahead, behind int, err error) {
	for _, ref := range []string{branch, base} {
		if err := validateRefArgument(ref); err != nil {
			return 0, 0, err
		}
	}

	// With base...branch the left count is commits only in base (behind) and
	// the right count is commits only in branch (ahead).
	out, err := run(repoPath, "rev-list", "--left-right", "--count", base+"..."+branch)
	if err != nil {
		return 0, 0, err
	}

	fields := strings.Fields(out)
	if len(fields) != 2 {
		return 0, 0, fmt.Errorf("parsing rev-list count %q: got %d fields, want 2", out, len(fields))
	}

	if behind, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, fmt.Errorf("parsing behind count %q: %w", fields[0], err)
	}
	if ahead, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, fmt.Errorf("parsing ahead count %q: %w", fields[1], err)
	}
	return ahead, behind, nil
}
