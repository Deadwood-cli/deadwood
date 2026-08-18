package classify

import (
	"fmt"
	"path"
	"time"
)

// Classify assigns a branch to a bucket per the Section 5 decision tree.
// Rules are evaluated top to bottom; the first match wins. Protected checks
// run before any merge evidence, so a current or stashed branch is never
// offered for deletion even when it would otherwise be a high-confidence
// safe_delete.
func Classify(branch BranchInfo, remote RemoteStatus, pr PRStatus, defaultBranch string, cfg Config) BranchResult {
	result := BranchResult{Branch: branch}

	switch {
	case branch.IsCurrent:
		result.Bucket = BucketProtected
		result.Confidence = ConfidenceHigh
		result.Reason = "currently checked out"

	case branch.HasWorktree:
		result.Bucket = BucketProtected
		result.Confidence = ConfidenceHigh
		result.Reason = "checked out in another worktree"

	case matchesExclude(branch.Name, cfg.ExcludePatterns):
		result.Bucket = BucketProtected
		result.Confidence = ConfidenceHigh
		result.Reason = "matches exclude pattern in config"

	case branch.HasStash:
		// HasStash is already the spec 5.1 heuristic applied by the caller
		// (substring match on git's auto-generated stash message). Classify
		// trusts that boolean rather than re-parsing stash text.
		result.Bucket = BucketProtected
		result.Confidence = ConfidenceHigh
		result.Reason = "has associated stash entry"

	case remote.Exists:
		result.Bucket = BucketActive
		result.Confidence = ConfidenceHigh
		result.Reason = "remote branch still exists"

	case branch.IsAncestor:
		result.Bucket = BucketSafeDelete
		result.Confidence = ConfidenceHigh
		result.Reason = fmt.Sprintf("fully merged into %s, remote deleted", defaultBranch)

	case pr.Found && pr.Merged:
		result.Bucket = BucketSquashMerged
		result.Confidence = ConfidenceHigh
		result.Reason = fmt.Sprintf(
			"matched merged PR #%d (merged %s), likely squash-merged",
			pr.Number, formatTime(pr.MergedAt),
		)

	case branch.AheadCount == 0 && branch.BehindCount > 0:
		result.Bucket = BucketSafeDelete
		result.Confidence = ConfidenceMedium
		result.Reason = fmt.Sprintf("no unique commits relative to %s, remote deleted", defaultBranch)

	default:
		result.Bucket = BucketNeedsReview
		result.Confidence = ConfidenceNA
		result.Reason = fmt.Sprintf(
			"remote deleted but branch has %d unmerged commit(s), last activity %s",
			branch.AheadCount, formatTime(branch.LastCommitDate),
		)
	}

	return result
}

// matchesExclude reports whether name matches any configured glob. Invalid
// patterns cannot match, so they never silently protect (or unprotect) a
// branch; phase 7 is where a malformed config file fails loudly.
func matchesExclude(name string, patterns []string) bool {
	for _, pattern := range patterns {
		ok, err := path.Match(pattern, name)
		if err == nil && ok {
			return true
		}
	}
	return false
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format("2006-01-02")
}
