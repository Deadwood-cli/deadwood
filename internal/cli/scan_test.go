package cli

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func localScanFixture(t *testing.T) *testRepo {
	t.Helper()

	r := newTestRepo(t)
	r.branchWithCommit("merged", "merged work")
	r.mergeNoFF("merged")
	r.branchWithCommit("unmerged", "unmerged work")
	r.branchWithCommit("release/1.0", "release work")
	r.branchWithCommit("stashed", "work that will be stashed")
	r.stashOn("stashed")
	r.addOriginAndHead()
	return r
}

func TestBuildScanClassifiesLocalTopology(t *testing.T) {
	r := localScanFixture(t)

	report, err := buildScan(r.dir, classify.DefaultConfig())
	require.NoError(t, err)

	assert.Equal(t, "main", report.DefaultBranch)
	assert.True(t, report.LocalOnly)

	got := bucketsByName(report.Results)
	assert.Equal(t, classify.BucketProtected, got["main"], "current + excluded")
	assert.Equal(t, classify.BucketSafeDelete, got["merged"])
	assert.Equal(t, classify.BucketNeedsReview, got["unmerged"])
	assert.Equal(t, classify.BucketProtected, got["release/1.0"], "exclude glob")
	assert.Equal(t, classify.BucketProtected, got["stashed"])

	counts := map[classify.Bucket]int{}
	for _, bucket := range got {
		counts[bucket]++
	}
	assert.Zero(t, counts[classify.BucketActive], "local-only scan never sees a live remote")
	assert.Zero(t, counts[classify.BucketSquashMerged], "local-only scan never matches PRs")
}

func TestBuildScanOutsideRepository(t *testing.T) {
	_, err := buildScan(t.TempDir(), classify.DefaultConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestBuildScanWithoutOriginHead(t *testing.T) {
	r := newTestRepo(t)

	_, err := buildScan(r.dir, classify.DefaultConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine the default branch")
}

func TestScanCommandJSON(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, stderr, err := run(t, "scan", "--json")
	require.NoError(t, err)
	assert.Empty(t, stderr)

	var payload struct {
		DefaultBranch string         `json:"default_branch"`
		LocalOnly     bool           `json:"local_only"`
		BranchCount   int            `json:"branch_count"`
		Counts        map[string]int `json:"counts"`
		Branches      []struct {
			Name   string `json:"name"`
			Bucket string `json:"bucket"`
		} `json:"branches"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	assert.Equal(t, "main", payload.DefaultBranch)
	assert.True(t, payload.LocalOnly)
	assert.Equal(t, 5, payload.BranchCount)
	assert.Equal(t, 1, payload.Counts["safe_delete"])
	assert.Equal(t, 0, payload.Counts["squash_merged"])
	assert.Equal(t, 1, payload.Counts["needs_review"])
	assert.Equal(t, 0, payload.Counts["active"])
	assert.Equal(t, 3, payload.Counts["protected"])
}

func TestScanCommandHumanAndVerbose(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := run(t, "scan")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Deadwood scan — 5 local branches (local-only)")
	assert.Contains(t, stdout, "Safe to delete")
	assert.Contains(t, stdout, "Remote branches and pull requests were not checked.")
	assert.Contains(t, stdout, "Run `deadwood clean` to review and delete.")
	assert.NotContains(t, stdout, "unmerged work") // non-verbose has no per-branch reasons

	verbose, _, err := run(t, "scan", "--verbose")
	require.NoError(t, err)
	assert.Contains(t, verbose, "unmerged")
	assert.Contains(t, verbose, "currently checked out")
	assert.Contains(t, verbose, "fully merged into main, remote deleted")
}

func TestNoSubcommandRunsScan(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := run(t)
	require.NoError(t, err)
	assert.Contains(t, stdout, "Deadwood scan —")
}

func bucketsByName(results []classify.BranchResult) map[string]classify.Bucket {
	got := make(map[string]classify.Bucket, len(results))
	for _, result := range results {
		got[result.Branch.Name] = result.Bucket
	}
	return got
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(wd) })
}
