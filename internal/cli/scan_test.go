package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Deadwood-cli/deadwood/internal/auth"
	"github.com/Deadwood-cli/deadwood/internal/classify"
	"github.com/Deadwood-cli/deadwood/internal/config"
	ghub "github.com/Deadwood-cli/deadwood/internal/github"
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

func fixtureScanDeps() *scanDeps {
	deps := defaultScanDeps()
	deps.originURL = func(string) (string, error) {
		return "https://github.com/Deadwood-cli/deadwood.git", nil
	}
	deps.store = auth.NewStore(auth.NewMemoryRing(), func(string) (string, bool) { return "", false })
	return deps
}

func TestBuildScanClassifiesWithRemotes(t *testing.T) {
	r := localScanFixture(t)
	var warnings bytes.Buffer

	report, err := buildScan(context.Background(), r.dir, config.Defaults(), fixtureScanDeps(), &warnings)
	require.NoError(t, err)

	assert.Equal(t, "main", report.DefaultBranch)
	assert.False(t, report.LocalOnly)
	assert.False(t, report.PRsChecked)
	assert.Contains(t, warnings.String(), "squash-merged detection skipped")

	got := bucketsByName(report.Results)
	assert.Equal(t, classify.BucketProtected, got["main"])
	assert.Equal(t, classify.BucketSafeDelete, got["merged"])
	assert.Equal(t, classify.BucketNeedsReview, got["unmerged"])
	assert.Equal(t, classify.BucketProtected, got["release/1.0"])
	assert.Equal(t, classify.BucketProtected, got["stashed"])
}

func TestBuildScanRemoteExistsIsActive(t *testing.T) {
	r := localScanFixture(t)
	r.git("push", "origin", "unmerged")

	report, err := buildScan(context.Background(), r.dir, config.Defaults(), fixtureScanDeps(), ioDiscard())
	require.NoError(t, err)

	got := bucketsByName(report.Results)
	assert.Equal(t, classify.BucketActive, got["unmerged"])
}

func TestBuildScanSquashMergedFromPR(t *testing.T) {
	r := localScanFixture(t)
	deps := fixtureScanDeps()
	require.NoError(t, deps.store.Set("test-token"))
	deps.mergedPRs = func(_ context.Context, token string, _ ghub.Repo, branches []string) (map[string]ghub.PRMatch, error) {
		assert.Equal(t, "test-token", token)
		out := make(map[string]ghub.PRMatch, len(branches))
		for _, branch := range branches {
			if branch == "unmerged" {
				out[branch] = ghub.PRMatch{
					Found:    true,
					Merged:   true,
					Number:   42,
					MergedAt: time.Date(2026, time.March, 10, 0, 0, 0, 0, time.UTC),
				}
			}
		}
		return out, nil
	}

	report, err := buildScan(context.Background(), r.dir, config.Defaults(), deps, ioDiscard())
	require.NoError(t, err)

	assert.True(t, report.PRsChecked)
	got := bucketsByName(report.Results)
	assert.Equal(t, classify.BucketSquashMerged, got["unmerged"])
	assert.Equal(t, classify.BucketSafeDelete, got["merged"], "ancestor still wins over a PR lookup")
}

func TestBuildScanRejectsNonGitHubOrigin(t *testing.T) {
	r := localScanFixture(t)
	deps := fixtureScanDeps()
	deps.originURL = func(string) (string, error) {
		return "https://gitlab.com/org/repo.git", nil
	}

	_, err := buildScan(context.Background(), r.dir, config.Defaults(), deps, ioDiscard())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "only supports GitHub")
}

func TestBuildScanOutsideRepository(t *testing.T) {
	_, err := buildScan(context.Background(), t.TempDir(), config.Defaults(), fixtureScanDeps(), ioDiscard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a git repository")
}

func TestBuildScanWithoutOriginHead(t *testing.T) {
	r := newTestRepo(t)

	_, err := buildScan(context.Background(), r.dir, config.Defaults(), fixtureScanDeps(), ioDiscard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine the default branch")
}

func TestBuildScanDefaultBranchFromGitHub(t *testing.T) {
	r := newTestRepo(t)
	r.git("remote", "add", "origin", "https://github.com/Deadwood-cli/deadwood.git")
	deps := fixtureScanDeps()
	require.NoError(t, deps.store.Set("test-token"))
	deps.listRemotes = func(string) (map[string]struct{}, error) {
		return map[string]struct{}{"main": {}}, nil
	}
	deps.ghDefault = func(_ context.Context, token string, repo ghub.Repo) (string, error) {
		assert.Equal(t, "test-token", token)
		assert.Equal(t, "Deadwood-cli", repo.Owner)
		return "main", nil
	}

	report, err := buildScan(context.Background(), r.dir, config.Defaults(), deps, ioDiscard())
	require.NoError(t, err)
	assert.Equal(t, "main", report.DefaultBranch)
}

func TestScanCommandJSON(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, stderr, err := runScanCLI(t, fixtureScanDeps(), "scan", "--json")
	require.NoError(t, err)
	assert.Contains(t, stderr, "squash-merged detection skipped")

	var payload struct {
		DefaultBranch string         `json:"default_branch"`
		LocalOnly     bool           `json:"local_only"`
		PRsChecked    bool           `json:"prs_checked"`
		BranchCount   int            `json:"branch_count"`
		Counts        map[string]int `json:"counts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))

	assert.Equal(t, "main", payload.DefaultBranch)
	assert.False(t, payload.LocalOnly)
	assert.False(t, payload.PRsChecked)
	assert.Equal(t, 5, payload.BranchCount)
	assert.Equal(t, 1, payload.Counts["safe_delete"])
	assert.Equal(t, 0, payload.Counts["squash_merged"])
	assert.Equal(t, 1, payload.Counts["needs_review"])
	assert.Equal(t, 0, payload.Counts["active"])
	assert.Equal(t, 3, payload.Counts["protected"])
}

func TestScanCommandHuman(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, stderr, err := runScanCLI(t, fixtureScanDeps(), "scan")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Deadwood scan — 5 local branches")
	assert.NotContains(t, stdout, "(local-only)")
	assert.Contains(t, stdout, "Squash-merged detection skipped; run `deadwood auth login`.")
	assert.Contains(t, stderr, "squash-merged detection skipped")
	assert.Contains(t, stdout, "Run `deadwood clean` to review and delete.")
}

func TestNoSubcommandRunsScan(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	stdout, _, err := runScanCLI(t, fixtureScanDeps())
	require.NoError(t, err)
	assert.Contains(t, stdout, "Deadwood scan —")
}

func TestBuildScanHonorsDefaultBranchOverride(t *testing.T) {
	r := localScanFixture(t)
	cfg := config.Defaults()
	cfg.DefaultBranch = "merged"

	report, err := buildScan(context.Background(), r.dir, cfg, fixtureScanDeps(), ioDiscard())
	require.NoError(t, err)
	assert.Equal(t, "merged", report.DefaultBranch)
}

func TestBuildScanUnknownDefaultBranchOverrideFails(t *testing.T) {
	r := localScanFixture(t)
	cfg := config.Defaults()
	cfg.DefaultBranch = "does-not-exist"

	_, err := buildScan(context.Background(), r.dir, cfg, fixtureScanDeps(), ioDiscard())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestBuildScanOverrideSkipsMissingOriginHead(t *testing.T) {
	r := newTestRepo(t)
	r.git("remote", "add", "origin", "https://github.com/Deadwood-cli/deadwood.git")
	deps := fixtureScanDeps()
	deps.listRemotes = func(string) (map[string]struct{}, error) {
		return map[string]struct{}{"main": {}}, nil
	}
	cfg := config.Defaults()
	cfg.DefaultBranch = "main"

	report, err := buildScan(context.Background(), r.dir, cfg, deps, ioDiscard())
	require.NoError(t, err)
	assert.Equal(t, "main", report.DefaultBranch)
}

func TestBuildScanCustomExcludePatterns(t *testing.T) {
	r := localScanFixture(t)
	cfg := config.Defaults()
	cfg.ExcludePatterns = []string{"unmerged"}

	report, err := buildScan(context.Background(), r.dir, cfg, fixtureScanDeps(), ioDiscard())
	require.NoError(t, err)

	got := bucketsByName(report.Results)
	assert.Equal(t, classify.BucketProtected, got["unmerged"])
	assert.Equal(t, classify.BucketSafeDelete, got["merged"])
}

func TestScanCommandLoadsRepoConfig(t *testing.T) {
	r := localScanFixture(t)
	writeConfig(t, r.dir, `
exclude_patterns:
  - main
  - master
  - develop
  - release/*
  - hotfix/*
  - unmerged
`)
	chdir(t, r.dir)

	stdout, _, err := runScanCLI(t, fixtureScanDeps(), "scan", "--json")
	require.NoError(t, err)

	var payload struct {
		Counts map[string]int `json:"counts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	assert.Equal(t, 0, payload.Counts["needs_review"])
	assert.Equal(t, 4, payload.Counts["protected"])
}

func TestScanCommandMalformedConfigFails(t *testing.T) {
	r := localScanFixture(t)
	writeConfig(t, r.dir, "exclude_patterns: [\n")
	chdir(t, r.dir)

	_, _, err := runScanCLI(t, fixtureScanDeps(), "scan")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed config file")
}

func TestScanCommandMissingOverrideConfigFails(t *testing.T) {
	r := localScanFixture(t)
	chdir(t, r.dir)

	missing := filepath.Join(t.TempDir(), "nope.yml")
	_, _, err := runScanCLI(t, fixtureScanDeps(), "scan", "--config", missing)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file")
}

func TestScanCommandConfigFlagOverridesRepoFile(t *testing.T) {
	r := localScanFixture(t)
	writeConfig(t, r.dir, "exclude_patterns: [\"unmerged\"]\n")
	other := filepath.Join(t.TempDir(), "other.yml")
	require.NoError(t, os.WriteFile(other, []byte("exclude_patterns: []\n"), 0o600))
	chdir(t, r.dir)

	stdout, _, err := runScanCLI(t, fixtureScanDeps(), "scan", "--json", "--config", other)
	require.NoError(t, err)

	var payload struct {
		Counts map[string]int `json:"counts"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	assert.Equal(t, 2, payload.Counts["needs_review"])
	assert.Equal(t, 2, payload.Counts["protected"], "main (current) and stashed; defaults were not merged")
}

func writeConfig(t *testing.T, repoDir, body string) {
	t.Helper()
	path := filepath.Join(repoDir, config.FileName)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
}

func runScanCLI(t *testing.T, sd *scanDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCommand(testBuildInfo(), defaultAuthRuntime(), sd, nil)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{}, args...))
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func ioDiscard() *bytes.Buffer { return &bytes.Buffer{} }

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
