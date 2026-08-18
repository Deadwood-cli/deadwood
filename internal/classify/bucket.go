package classify

import "time"

// Bucket is the classification assigned to a local branch. The five values
// match spec Section 4; the decision tree in Section 5 is the only producer.
type Bucket string

const (
	BucketSafeDelete   Bucket = "safe_delete"   // remote gone, fully merged (ancestor) or no unique commits
	BucketSquashMerged Bucket = "squash_merged" // remote gone, matched to a merged pull request
	BucketNeedsReview  Bucket = "needs_review"  // remote gone, merge status could not be confirmed
	BucketActive       Bucket = "active"        // remote still exists
	BucketProtected    Bucket = "protected"     // current, worktree, stash, or excluded by config
)

// Confidence is surfaced in the UI. Protected and active outcomes are
// unambiguous, so they report high even though the spec only spells out the
// values for the remote-gone cases.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
	ConfidenceNA     = "n/a"
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
// internal/config (phase 7); this is only the fields the decision tree reads.
type Config struct {
	ExcludePatterns []string
}
