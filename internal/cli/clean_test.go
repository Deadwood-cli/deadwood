package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/Deadwood-cli/deadwood/internal/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanYesDryRunDoesNotDelete(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "", "clean", "--yes")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Dry-run: would delete 1 branch")
	assert.Contains(t, stdout, "merged")
	assert.Contains(t, stdout, "refs/deadwood-backup/merged")
	assert.NotContains(t, stdout, "unmerged")

	r.git("show-ref", "--verify", "refs/heads/merged")
}

func TestCleanDryRunFalseDeletesMerged(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)
	tip := r.git("rev-parse", "refs/heads/merged")

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "", "clean", "--yes", "--dry-run=false")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Deleted 1 branch")
	assert.Contains(t, stdout, "merged")
	assert.Contains(t, stdout, "refs/deadwood-backup/merged")
	assert.Contains(t, stdout, "deadwood undo")
	assert.NotContains(t, stdout, "Dry-run:")
	assert.False(t, r.hasRef("refs/heads/merged"))
	assert.True(t, r.hasRef("refs/deadwood-backup/merged"))

	stdout, _, err = runUndoCLI(t, "undo", "merged")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Restored merged")
	assert.True(t, r.hasRef("refs/heads/merged"))
	assert.Equal(t, tip, r.git("rev-parse", "refs/heads/merged"))
}

func TestCleanSkipsUnmergedRatherThanForceDeleting(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "", "clean", "--yes", "--dry-run=false", "--include-needs-review")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Deleted 1 branch")
	assert.Contains(t, stdout, "Skipped 1 branch")
	assert.Contains(t, stdout, "unmerged")
	assert.True(t, r.hasRef("refs/heads/unmerged"))
	assert.False(t, r.hasRef("refs/heads/merged"))
}

func TestCleanIncludeNeedsReviewPrechecks(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "", "clean", "--yes", "--include-needs-review")
	require.NoError(t, err)
	assert.Contains(t, stdout, "would delete 2 branches")
	assert.Contains(t, stdout, "merged")
	assert.Contains(t, stdout, "unmerged")
	r.git("show-ref", "--verify", "refs/heads/unmerged")
}

func TestCleanTypedYes(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "yes\n", "clean")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Type yes to continue:")
	assert.Contains(t, stdout, "Dry-run: would delete 1 branch")
	r.git("show-ref", "--verify", "refs/heads/merged")
}

func TestCleanRejectsNonYesConfirmation(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "YES\n", "clean")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Cancelled. No branches were deleted.")
	assert.NotContains(t, stdout, "Dry-run:")
	r.git("show-ref", "--verify", "refs/heads/merged")
}

func TestCleanChecklistCancel(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)
	cd := &cleanDeps{
		runChecklist: func([]tui.Item, io.Reader, io.Writer) ([]tui.Item, bool, error) {
			return nil, true, nil
		},
		confirm: confirmDeletes,
	}

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), cd, "", "clean", "--yes")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Cancelled. No branches were deleted.")
}

func TestCleanEmptySelection(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)
	cd := &cleanDeps{
		runChecklist: func([]tui.Item, io.Reader, io.Writer) ([]tui.Item, bool, error) {
			return nil, false, nil
		},
		confirm: confirmDeletes,
	}

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), cd, "", "clean", "--yes")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No branches selected.")
}

func TestCleanNothingToClean(t *testing.T) {
	r := newTestRepo(t)
	r.addOriginAndHead()
	chdir(t, r.dir)

	stdout, _, err := runCleanCLI(t, fixtureScanDeps(), nil, "", "clean", "--yes")
	require.NoError(t, err)
	assert.Contains(t, stdout, "No branches to clean.")
}

func TestChecklistItemsPrecheck(t *testing.T) {
	results := []classify.BranchResult{
		{Branch: classify.BranchInfo{Name: "safe"}, Bucket: classify.BucketSafeDelete, Reason: "merged"},
		{Branch: classify.BranchInfo{Name: "squash"}, Bucket: classify.BucketSquashMerged, Reason: "pr"},
		{Branch: classify.BranchInfo{Name: "review"}, Bucket: classify.BucketNeedsReview, Reason: "ahead"},
		{Branch: classify.BranchInfo{Name: "live"}, Bucket: classify.BucketActive, Reason: "remote"},
		{Branch: classify.BranchInfo{Name: "main"}, Bucket: classify.BucketProtected, Reason: "current"},
	}

	items := checklistItems(results, false, 90)
	require.Len(t, items, 3)
	assert.Equal(t, "safe", items[0].Name)
	assert.True(t, items[0].Checked)
	assert.True(t, items[1].Checked)
	assert.Equal(t, "review", items[2].Name)
	assert.False(t, items[2].Checked)

	included := checklistItems(results, true, 90)
	assert.True(t, included[2].Checked)
}

func TestItemReasonStale(t *testing.T) {
	result := classify.BranchResult{
		Bucket: classify.BucketNeedsReview,
		Reason: "unmerged",
		Branch: classify.BranchInfo{LastCommitDate: time.Now().UTC().Add(-100 * 24 * time.Hour)},
	}
	assert.Equal(t, "unmerged (stale)", itemReason(result, 90))
	assert.Equal(t, "unmerged", itemReason(result, 200))
}

func runCleanCLI(t *testing.T, sd *scanDeps, cd *cleanDeps, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCommand(testBuildInfo(), defaultAuthRuntime(), sd, cd)
	var out, errOut bytes.Buffer
	root.SetIn(strings.NewReader(stdin))
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{}, args...))
	err = root.Execute()
	return out.String(), errOut.String(), err
}
