package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteHumanSummary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := Write(&buf, sampleReport(), Options{})
	require.NoError(t, err)

	got := buf.String()
	assert.Contains(t, got, "Deadwood scan — 3 local branches (local-only)")
	assert.Contains(t, got, "Safe to delete")
	assert.Contains(t, got, "Needs review")
	assert.Contains(t, got, "Remote branches and pull requests were not checked.")
	assert.Contains(t, got, "Run `deadwood clean` to review and delete.")
	assert.NotContains(t, got, "feature-a")
}

func TestWriteHumanPRLookupSkipped(t *testing.T) {
	t.Parallel()

	report := sampleReport()
	report.LocalOnly = false
	report.PRsChecked = false

	var buf bytes.Buffer
	require.NoError(t, Write(&buf, report, Options{}))

	got := buf.String()
	assert.Contains(t, got, "Deadwood scan — 3 local branches")
	assert.NotContains(t, got, "(local-only)")
	assert.Contains(t, got, "Squash-merged detection skipped; run `deadwood auth login`.")
}

func TestWriteHumanVerboseListsEveryBranch(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := Write(&buf, sampleReport(), Options{Verbose: true})
	require.NoError(t, err)

	got := buf.String()
	assert.Contains(t, got, "feature-a")
	assert.Contains(t, got, "feature-b")
	assert.Contains(t, got, "main")
	assert.Contains(t, got, "currently checked out")
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := Write(&buf, sampleReport(), Options{JSON: true})
	require.NoError(t, err)

	var payload jsonReport
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	assert.Equal(t, "main", payload.DefaultBranch)
	assert.True(t, payload.LocalOnly)
	assert.Equal(t, 3, payload.BranchCount)
	assert.Equal(t, 1, payload.Counts["safe_delete"])
	assert.Equal(t, 1, payload.Counts["needs_review"])
	assert.Equal(t, 1, payload.Counts["protected"])
	require.Len(t, payload.Branches, 3)
	assert.Equal(t, "abc123", payload.Branches[0].LastCommitSHA)
	assert.Equal(t, "2026-03-15T00:00:00Z", payload.Branches[0].LastCommitDate)
	assert.Equal(t, 2, payload.Branches[1].AheadCount)
}

func sampleReport() Report {
	now := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	return Report{
		DefaultBranch: "main",
		LocalOnly:     true,
		Results: []classify.BranchResult{
			{
				Branch: classify.BranchInfo{
					Name:           "feature-a",
					LastCommitSHA:  "abc123",
					LastCommitDate: now,
				},
				Bucket:     classify.BucketSafeDelete,
				Reason:     "fully merged into main, remote deleted",
				Confidence: classify.ConfidenceHigh,
			},
			{
				Branch:     classify.BranchInfo{Name: "feature-b", AheadCount: 2, LastCommitDate: now},
				Bucket:     classify.BucketNeedsReview,
				Reason:     "remote deleted but branch has 2 unmerged commit(s), last activity 2026-03-15",
				Confidence: classify.ConfidenceNA,
			},
			{
				Branch:     classify.BranchInfo{Name: "main", IsCurrent: true},
				Bucket:     classify.BucketProtected,
				Reason:     "currently checked out",
				Confidence: classify.ConfidenceHigh,
			},
		},
	}
}
