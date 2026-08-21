package git

import (
	"errors"
	"fmt"
)

var (
	// ErrForceDeleteDisabled reports an attempt to force-delete a branch.
	// Deadwood never escalates to `git branch -D`: a branch git itself
	// considers unmerged is reported and skipped, because git's own refusal is
	// the last line of defence behind deadwood's classification.
	ErrForceDeleteDisabled = errors.New("force delete is not available in deadwood")

	// ErrNotFullyMerged reports that git declined a safe delete because the
	// branch holds commits reachable from nowhere else.
	ErrNotFullyMerged = errors.New("git considers the branch not fully merged")
)

// DeleteBranch deletes a local branch with `git branch -d`.
//
// The force parameter exists to match the signature in spec Section 6 and is
// always rejected: no code path in this repository can produce a `git branch -D`
// invocation, so the flag cannot be turned on by accident or by a future caller
// that has not read the safety rules.
//
// The branch is only deleted once a backup ref is confirmed to record its
// current tip. That check is repeated here rather than left to the caller so
// the sequencing rule holds structurally, not by convention.
func DeleteBranch(repoPath, branch string, force bool) error {
	if force {
		return fmt.Errorf("deleting branch %q: %w", branch, ErrForceDeleteDisabled)
	}
	if err := validateRefArgument(branch); err != nil {
		return err
	}
	if err := verifyBackupCoversTip(repoPath, branch); err != nil {
		return err
	}

	if _, err := runMutating(repoPath, "branch", "-d", branch); err != nil {
		if stderrContains(err, "not fully merged") {
			return fmt.Errorf("deleting branch %q: %w", branch, ErrNotFullyMerged)
		}
		return fmt.Errorf("deleting branch %q: %w", branch, err)
	}
	return nil
}
