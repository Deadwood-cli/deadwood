package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// defaultBranchName is the branch every fixture repository starts on.
const defaultBranchName = "main"

// fixtureEpoch anchors commit timestamps so date assertions are exact rather
// than approximate.
var fixtureEpoch = time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)

// TestMain points git at an empty configuration file for the whole package.
// Without this, the developer's own global config leaks into the fixtures:
// commit.gpgsign would block commits on a passphrase prompt, init.defaultBranch
// would rename the starting branch, and global hooks would run against
// throwaway repositories.
func TestMain(m *testing.M) {
	code, err := runPackageTests(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test setup:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func runPackageTests(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "deadwood-gitenv-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(dir)

	empty := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		return 0, err
	}

	for name, value := range map[string]string{
		"GIT_CONFIG_GLOBAL":       empty,
		"GIT_CONFIG_SYSTEM":       empty,
		"GIT_TERMINAL_PROMPT":     "0",
		"GIT_CEILING_DIRECTORIES": gitCeiling(),
	} {
		if err := os.Setenv(name, value); err != nil {
			return 0, err
		}
	}

	return m.Run(), nil
}

// gitCeiling stops git from searching for a repository above the temp
// directory. Without it, a fixture inherits whatever repository happens to sit
// higher up the tree, and "this is not a repository" becomes untestable: the
// machine this was written on has a git repository in the home directory, which
// is an ancestor of the system temp directory.
func gitCeiling() string {
	dirs := []string{os.TempDir()}
	// Git ignores ceiling entries that traverse symlinks, and macOS reaches the
	// temp directory through one, so offer the resolved path as well.
	if resolved, err := filepath.EvalSymlinks(os.TempDir()); err == nil && resolved != os.TempDir() {
		dirs = append(dirs, resolved)
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

// repo is a throwaway git repository built for a single test. It lives in the
// test's temp directory and is removed with it, so no fixture is ever reused or
// mutated in place.
type repo struct {
	t       *testing.T
	dir     string
	commits int
}

// newEmptyRepo initialises a repository with an identity but no commits.
func newEmptyRepo(t *testing.T) *repo {
	t.Helper()

	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "--initial-branch="+defaultBranchName)
	r.git("config", "user.name", "Deadwood Test")
	r.git("config", "user.email", "test@deadwood.invalid")
	r.git("config", "commit.gpgsign", "false")
	return r
}

// newRepo initialises a repository with one commit on the default branch.
func newRepo(t *testing.T) *repo {
	t.Helper()

	r := newEmptyRepo(t)
	r.commit("initial commit")
	return r
}

// git runs a git command in the fixture and fails the test if it errors.
func (r *repo) git(args ...string) string {
	r.t.Helper()

	out, err := r.tryGit(nil, args...)
	if err != nil {
		r.t.Fatalf("fixture: git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// tryGit runs a git command and returns its combined output and error.
func (r *repo) tryGit(extraEnv []string, args ...string) (string, error) {
	r.t.Helper()

	cmd := exec.Command(binary, append([]string{"-C", r.dir}, args...)...)
	cmd.Env = append(commandEnv(), extraEnv...)

	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		return text, fmt.Errorf("%w: %s", err, text)
	}
	return text, nil
}

// commit adds a new file and commits it at a deterministic timestamp, returning
// the resulting commit's object ID.
func (r *repo) commit(message string) string {
	r.t.Helper()

	r.commits++
	name := fmt.Sprintf("file-%02d.txt", r.commits)
	r.writeFile(name, message)
	r.git("add", name)
	r.commitStaged(message)
	return r.head()
}

// commitStaged commits whatever is currently staged at a deterministic time.
func (r *repo) commitStaged(message string) {
	r.t.Helper()

	stamp := fixtureEpoch.Add(time.Duration(r.commits) * time.Hour).Format(time.RFC3339)
	env := []string{"GIT_AUTHOR_DATE=" + stamp, "GIT_COMMITTER_DATE=" + stamp}
	if _, err := r.tryGit(env, "commit", "-m", message); err != nil {
		r.t.Fatalf("fixture: commit %q: %v", message, err)
	}
}

// commitTime returns the timestamp the nth commit in this fixture was given.
func (r *repo) commitTime(n int) time.Time {
	return fixtureEpoch.Add(time.Duration(n) * time.Hour)
}

func (r *repo) writeFile(name, content string) {
	r.t.Helper()

	if err := os.WriteFile(filepath.Join(r.dir, name), []byte(content+"\n"), 0o600); err != nil {
		r.t.Fatalf("fixture: writing %s: %v", name, err)
	}
}

func (r *repo) head() string {
	r.t.Helper()
	return r.git("rev-parse", "HEAD")
}

func (r *repo) checkout(branch string) {
	r.t.Helper()
	r.git("checkout", branch)
}

// branchWithCommit creates branch off the current HEAD, adds one commit to it
// and returns to the branch that was checked out before.
func (r *repo) branchWithCommit(branch, message string) string {
	r.t.Helper()

	previous := r.git("rev-parse", "--abbrev-ref", "HEAD")
	r.git("checkout", "-b", branch)
	sha := r.commit(message)
	r.checkout(previous)
	return sha
}

// mergeNoFF merges branch into the current branch with a merge commit, leaving
// the merged branch's tip as a genuine ancestor of the current one.
func (r *repo) mergeNoFF(branch string) {
	r.t.Helper()

	r.commits++
	stamp := fixtureEpoch.Add(time.Duration(r.commits) * time.Hour).Format(time.RFC3339)
	env := []string{"GIT_AUTHOR_DATE=" + stamp, "GIT_COMMITTER_DATE=" + stamp}
	if _, err := r.tryGit(env, "merge", "--no-ff", "-m", "merge "+branch, branch); err != nil {
		r.t.Fatalf("fixture: merging %s: %v", branch, err)
	}
}

// addOrigin creates a bare repository, wires it up as origin, and pushes the
// given branches to it. It returns the bare repository's path.
func (r *repo) addOrigin(branches ...string) string {
	r.t.Helper()

	bare := filepath.Join(r.t.TempDir(), "origin.git")
	cmd := exec.Command(binary, "init", "--bare", "--initial-branch="+defaultBranchName, bare)
	cmd.Env = commandEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("fixture: creating bare origin: %v: %s", err, out)
	}

	// Git reads a local path as a URL, where backslashes are not separators.
	r.git("remote", "add", remoteName, filepath.ToSlash(bare))
	if len(branches) > 0 {
		r.git(append([]string{"push", remoteName}, branches...)...)
	}
	return bare
}

// setOriginHead points origin/HEAD at a branch, as a real clone would.
func (r *repo) setOriginHead(branch string) {
	r.t.Helper()
	r.git("symbolic-ref", originHeadRef, "refs/remotes/"+remoteName+"/"+branch)
}

// requireBranchPresence asserts whether a branch exists, and is used mainly to
// prove that a rejected delete left the branch alone.
func requireBranchPresence(t *testing.T, r *repo, branch string, want bool) {
	t.Helper()

	got, err := LocalBranchExists(r.dir, branch)
	require.NoError(t, err)
	if want {
		require.True(t, got, "branch %q should exist", branch)
		return
	}
	require.False(t, got, "branch %q should not exist", branch)
}

// indexByName keys branches by name for assertions that do not care about order.
func indexByName(branches []BranchInfo) map[string]BranchInfo {
	byName := make(map[string]BranchInfo, len(branches))
	for _, branch := range branches {
		byName[branch.Name] = branch
	}
	return byName
}
