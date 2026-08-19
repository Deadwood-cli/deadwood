// Package classify is the decision engine that assigns every local branch to a
// bucket. It has no dependency on internal/git or internal/github: callers pass
// plain structs, and tests drive it with fixtures.
package classify

import "time"

// Bucket is the classification assigned to a local branch. The five values
// match spec Section 4; the decision tree in Section 5 is the only producer.
type Bucket string

const (
	// BucketSafeDelete is remote gone, fully merged (ancestor) or with no unique commits.
	BucketSafeDelete Bucket = "safe_delete"
	// BucketSquashMerged is remote gone, matched to a merged pull request.
	BucketSquashMerged Bucket = "squash_merged"
	// BucketNeedsReview is remote gone, merge status could not be confirmed.
	BucketNeedsReview Bucket = "needs_review"
	// BucketActive is a branch whose remote counterpart still exists.
	BucketActive Bucket = "active"
	// BucketProtected is current, in a worktree, stashed, or excluded by config.
	BucketProtected Bucket = "protected"
)

const (
	// ConfidenceHigh is an unambiguous classification.
	ConfidenceHigh = "high"
	// ConfidenceMedium is a likely-safe classification with weaker evidence
	// (no unique commits, but not a strict ancestor).
	ConfidenceMedium = "medium"
	// ConfidenceLow is reserved for a future weaker signal; unused in v0.1.
	ConfidenceLow = "low"
	// ConfidenceNA is reported when merge status could not be confirmed.
	ConfidenceNA = "n/a"
)

// BranchInfo is the per-branch input to Classify. It is a plain value type so
// the engine can be tested with fixtures; the cli layer is responsible for
// filling it from git and GitHub.
//
// HasStash and IsAncestor are not produced by a single git command — they are
// computed by the caller from ListStashRefs / IsAncestor and passed in, so this
// package never imports internal/git.
type BranchInfo struct {
	Name           string
	IsCurrent      bool
	HasWorktree    bool
	HasStash       bool
	IsAncestor     bool // tip is reachable from the default branch
	LastCommitSHA  string
	LastCommitDate time.Time
	AheadCount     int
	BehindCount    int
	HasUpstream    bool
	UpstreamName   string
}

// RemoteStatus reports whether origin still has this branch.
type RemoteStatus struct {
	Exists bool
}

// PRStatus is the result of looking up a closed pull request whose head ref
// matches the branch name. Found is independent of Merged so an unmerged PR
// does not get treated as squash-merged.
type PRStatus struct {
	Found    bool
	Merged   bool
	Number   int
	MergedAt time.Time
}

// BranchResult is Classify's output. Reason is always populated.
type BranchResult struct {
	Branch     BranchInfo
	Bucket     Bucket
	Reason     string
	Confidence string
}

// Config is the configuration Classify consults. YAML loading belongs to
// internal/config; this is only the fields the decision tree reads.
type Config struct {
	ExcludePatterns []string
}

// DefaultConfig returns the exclude patterns from spec Section 9. A missing
// .deadwood.yml must behave like this rather than protecting nothing.
func DefaultConfig() Config {
	return Config{
		ExcludePatterns: []string{
			"main",
			"master",
			"develop",
			"release/*",
			"hotfix/*",
		},
	}
}
