package classify

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	lastActivity = time.Date(2026, time.March, 15, 12, 0, 0, 0, time.UTC)
	mergedAt     = time.Date(2026, time.March, 10, 9, 30, 0, 0, time.UTC)
)

func sampleBranch(name string) BranchInfo {
	return BranchInfo{
		Name:           name,
		LastCommitSHA:  "abc1234",
		LastCommitDate: lastActivity,
		AheadCount:     3,
		BehindCount:    1,
	}
}

func TestClassifyDecisionTree(t *testing.T) {
	t.Parallel()

	defaultCfg := Config{ExcludePatterns: []string{"main", "master", "develop", "release/*", "hotfix/*"}}
	gone := RemoteStatus{Exists: false}
	live := RemoteStatus{Exists: true}
	noPR := PRStatus{}
	mergedPR := PRStatus{Found: true, Merged: true, Number: 42, MergedAt: mergedAt}

	tests := []struct {
		name       string
		branch     BranchInfo
		remote     RemoteStatus
		pr         PRStatus
		cfg        Config
		wantBucket Bucket
		wantConf   string
		wantReason string
	}{
		{
			name:       "current branch is protected",
			branch:     BranchInfo{Name: "feature", IsCurrent: true, IsAncestor: true, AheadCount: 0},
			remote:     gone,
			pr:         mergedPR,
			cfg:        defaultCfg,
			wantBucket: BucketProtected,
			wantConf:   ConfidenceHigh,
			wantReason: "currently checked out",
		},
		{
			name:       "worktree branch is protected",
			branch:     BranchInfo{Name: "feature", HasWorktree: true, IsAncestor: true, AheadCount: 0},
			remote:     gone,
			pr:         mergedPR,
			cfg:        defaultCfg,
			wantBucket: BucketProtected,
			wantConf:   ConfidenceHigh,
			wantReason: "checked out in another worktree",
		},
		{
			name:       "exact exclude pattern is protected",
			branch:     BranchInfo{Name: "develop", IsAncestor: true, AheadCount: 0},
			remote:     gone,
			cfg:        defaultCfg,
			wantBucket: BucketProtected,
			wantConf:   ConfidenceHigh,
			wantReason: "matches exclude pattern in config",
		},
		{
			name:       "glob exclude pattern is protected",
			branch:     BranchInfo{Name: "release/1.4", IsAncestor: true, AheadCount: 0},
			remote:     gone,
			cfg:        defaultCfg,
			wantBucket: BucketProtected,
			wantConf:   ConfidenceHigh,
			wantReason: "matches exclude pattern in config",
		},
		{
			name:       "stash-associated branch is protected",
			branch:     BranchInfo{Name: "feature", HasStash: true, IsAncestor: true, AheadCount: 0},
			remote:     gone,
			cfg:        defaultCfg,
			wantBucket: BucketProtected,
			wantConf:   ConfidenceHigh,
			wantReason: "has associated stash entry",
		},
		{
			name:       "remote still exists is active",
			branch:     sampleBranch("feature"),
			remote:     live,
			pr:         mergedPR,
			cfg:        defaultCfg,
			wantBucket: BucketActive,
			wantConf:   ConfidenceHigh,
			wantReason: "remote branch still exists",
		},
		{
			name: "remote gone and ancestor is safe_delete high",
			branch: BranchInfo{
				Name:        "feature",
				IsAncestor:  true,
				AheadCount:  0,
				BehindCount: 4,
			},
			remote:     gone,
			cfg:        defaultCfg,
			wantBucket: BucketSafeDelete,
			wantConf:   ConfidenceHigh,
			wantReason: "fully merged into main, remote deleted",
		},
		{
			name: "remote gone not ancestor matched merged PR is squash_merged",
			branch: BranchInfo{
				Name:           "feature",
				AheadCount:     5,
				BehindCount:    2,
				LastCommitDate: lastActivity,
			},
			remote:     gone,
			pr:         mergedPR,
			cfg:        defaultCfg,
			wantBucket: BucketSquashMerged,
			wantConf:   ConfidenceHigh,
			wantReason: "matched merged PR #42 (merged 2026-03-10), likely squash-merged",
		},
		{
			name: "remote gone not ancestor no PR zero ahead is safe_delete medium",
			branch: BranchInfo{
				Name:        "feature",
				AheadCount:  0,
				BehindCount: 3,
			},
			remote:     gone,
			pr:         noPR,
			cfg:        defaultCfg,
			wantBucket: BucketSafeDelete,
			wantConf:   ConfidenceMedium,
			wantReason: "no unique commits relative to main, remote deleted",
		},
		{
			name: "remote gone not ancestor no PR nonzero ahead is needs_review",
			branch: BranchInfo{
				Name:           "feature",
				AheadCount:     4,
				BehindCount:    1,
				LastCommitDate: lastActivity,
			},
			remote:     gone,
			pr:         noPR,
			cfg:        defaultCfg,
			wantBucket: BucketNeedsReview,
			wantConf:   ConfidenceNA,
			wantReason: "remote deleted but branch has 4 unmerged commit(s), last activity 2026-03-15",
		},
		{
			name: "priority: current wins over otherwise safe_delete",
			branch: BranchInfo{
				Name:        "feature",
				IsCurrent:   true,
				IsAncestor:  true,
				AheadCount:  0,
				BehindCount: 2,
			},
			remote:     gone,
			cfg:        defaultCfg,
			wantBucket: BucketProtected,
			wantConf:   ConfidenceHigh,
			wantReason: "currently checked out",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Classify(tc.branch, tc.remote, tc.pr, "main", tc.cfg)

			assert.Equal(t, tc.wantBucket, got.Bucket)
			assert.Equal(t, tc.wantConf, got.Confidence)
			assert.Equal(t, tc.wantReason, got.Reason)
			assert.Equal(t, tc.branch, got.Branch, "the input branch must be echoed in the result")
			assert.NotEmpty(t, got.Reason, "reason is always populated")
		})
	}
}

func TestClassifyAdditionalPriorityAndFallthrough(t *testing.T) {
	t.Parallel()

	cfg := Config{ExcludePatterns: []string{"main", "release/*"}}
	gone := RemoteStatus{}

	tests := []struct {
		name       string
		branch     BranchInfo
		remote     RemoteStatus
		pr         PRStatus
		cfg        Config
		wantBucket Bucket
		wantReason string
	}{
		{
			name: "worktree wins over exclude",
			branch: BranchInfo{
				Name:        "release/1.0",
				HasWorktree: true,
			},
			remote:     gone,
			cfg:        cfg,
			wantBucket: BucketProtected,
			wantReason: "checked out in another worktree",
		},
		{
			name: "exclude wins over stash",
			branch: BranchInfo{
				Name:     "main",
				HasStash: true,
			},
			remote:     gone,
			cfg:        cfg,
			wantBucket: BucketProtected,
			wantReason: "matches exclude pattern in config",
		},
		{
			name: "stash wins over a live remote",
			branch: BranchInfo{
				Name:     "feature",
				HasStash: true,
			},
			remote:     RemoteStatus{Exists: true},
			cfg:        cfg,
			wantBucket: BucketProtected,
			wantReason: "has associated stash entry",
		},
		{
			name: "ancestor wins over a merged PR",
			branch: BranchInfo{
				Name:       "feature",
				IsAncestor: true,
			},
			remote:     gone,
			pr:         PRStatus{Found: true, Merged: true, Number: 9, MergedAt: mergedAt},
			cfg:        cfg,
			wantBucket: BucketSafeDelete,
			wantReason: "fully merged into main, remote deleted",
		},
		{
			name: "unmerged PR does not count as squash-merged",
			branch: BranchInfo{
				Name:           "feature",
				AheadCount:     2,
				LastCommitDate: lastActivity,
			},
			remote:     gone,
			pr:         PRStatus{Found: true, Merged: false, Number: 9},
			cfg:        cfg,
			wantBucket: BucketNeedsReview,
			wantReason: "remote deleted but branch has 2 unmerged commit(s), last activity 2026-03-15",
		},
		{
			name: "zero ahead and zero behind is needs_review not medium safe_delete",
			branch: BranchInfo{
				Name:           "feature",
				AheadCount:     0,
				BehindCount:    0,
				LastCommitDate: lastActivity,
			},
			remote:     gone,
			cfg:        cfg,
			wantBucket: BucketNeedsReview,
			wantReason: "remote deleted but branch has 0 unmerged commit(s), last activity 2026-03-15",
		},
		{
			name: "ahead of default with no other evidence is needs_review",
			branch: BranchInfo{
				Name:           "feature",
				AheadCount:     1,
				BehindCount:    0,
				LastCommitDate: lastActivity,
			},
			remote:     gone,
			cfg:        cfg,
			wantBucket: BucketNeedsReview,
			wantReason: "remote deleted but branch has 1 unmerged commit(s), last activity 2026-03-15",
		},
		{
			name: "empty exclude list does not protect main by name",
			branch: BranchInfo{
				Name:       "main",
				IsAncestor: true,
			},
			remote:     gone,
			cfg:        Config{},
			wantBucket: BucketSafeDelete,
			wantReason: "fully merged into main, remote deleted",
		},
		{
			name: "hotfix glob matches one path segment",
			branch: BranchInfo{
				Name: "hotfix/urgent",
			},
			remote:     RemoteStatus{Exists: true},
			cfg:        Config{ExcludePatterns: []string{"hotfix/*"}},
			wantBucket: BucketProtected,
			wantReason: "matches exclude pattern in config",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Classify(tc.branch, tc.remote, tc.pr, "main", tc.cfg)

			assert.Equal(t, tc.wantBucket, got.Bucket)
			assert.Equal(t, tc.wantReason, got.Reason)
		})
	}
}

func TestMatchesExclude(t *testing.T) {
	t.Parallel()

	patterns := []string{"main", "release/*", "feat-?"}

	tests := map[string]struct {
		name string
		want bool
	}{
		"exact":                       {name: "main", want: true},
		"glob one segment":            {name: "release/1.0", want: true},
		"glob does not cross slashes": {name: "release/1.0/rc", want: false},
		"single-char glob":            {name: "feat-x", want: true},
		"no match":                    {name: "feature", want: false},
		"empty name":                  {name: "", want: false},
	}

	for label, tc := range tests {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, matchesExclude(tc.name, patterns))
		})
	}

	t.Run("invalid pattern is ignored not treated as a match", func(t *testing.T) {
		t.Parallel()
		assert.False(t, matchesExclude("feature", []string{"["}))
	})
}

func TestFormatTime(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "unknown", formatTime(time.Time{}))
	assert.Equal(t, "2026-03-10", formatTime(mergedAt))
}

// The classification engine must stay unit-testable against plain structs. An
// import of internal/git or internal/github would couple it to I/O and is a
// hard stop per AGENTS.md Section 1.
func TestClassifyHasNoGitOrGitHubImports(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		src, err := os.ReadFile(filepath.Join(".", entry.Name()))
		require.NoError(t, err)

		file, err := parser.ParseFile(fset, entry.Name(), src, parser.ImportsOnly)
		require.NoError(t, err)

		for _, spec := range file.Imports {
			path := strings.Trim(spec.Path.Value, `"`)
			assert.NotContains(t, path, "internal/git", "%s imports %s", entry.Name(), path)
			assert.NotContains(t, path, "internal/github", "%s imports %s", entry.Name(), path)
		}
	}
}
