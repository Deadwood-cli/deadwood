package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	fixtureGitOnce sync.Once
	fixtureGitPath string
)

func fixtureGit() string {
	fixtureGitOnce.Do(func() {
		path, err := exec.LookPath("git")
		if err != nil {
			fixtureGitPath = "git"
			return
		}
		fixtureGitPath = path
	})
	return fixtureGitPath
}

// TestMain points git at an empty configuration file so fixture repos created
// by this package are not affected by the developer's global git config
// (commit.gpgsign, hooks, init.defaultBranch).
func TestMain(m *testing.M) {
	code, err := isolateGitConfig(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "test setup:", err)
		os.Exit(1)
	}
	os.Exit(code)
}

func isolateGitConfig(m *testing.M) (int, error) {
	dir, err := os.MkdirTemp("", "deadwood-clienv-")
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

func gitCeiling() string {
	dirs := []string{os.TempDir()}
	if resolved, err := filepath.EvalSymlinks(os.TempDir()); err == nil && resolved != os.TempDir() {
		dirs = append(dirs, resolved)
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

type testRepo struct {
	t       *testing.T
	dir     string
	commits int
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()

	r := &testRepo{t: t, dir: t.TempDir()}
	r.git("init", "--initial-branch=main")
	r.git("config", "user.name", "Deadwood Test")
	r.git("config", "user.email", "test@deadwood.invalid")
	r.git("config", "commit.gpgsign", "false")
	r.commit("initial commit")
	return r
}

func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command(fixtureGit(), append([]string{"-C", r.dir}, args...)...) //nolint:gosec // test fixture; argv slice
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("fixture: git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *testRepo) commit(message string) {
	r.t.Helper()
	r.commits++
	name := fmt.Sprintf("file-%02d.txt", r.commits)
	path := filepath.Join(r.dir, name)
	if err := os.WriteFile(path, []byte(message+"\n"), 0o600); err != nil {
		r.t.Fatalf("fixture: writing %s: %v", name, err)
	}
	r.git("add", name)
	r.git("commit", "-m", message)
}

func (r *testRepo) branchWithCommit(branch, message string) {
	r.t.Helper()
	previous := r.git("rev-parse", "--abbrev-ref", "HEAD")
	r.git("checkout", "-b", branch)
	r.commit(message)
	r.git("checkout", previous)
}

func (r *testRepo) hasRef(ref string) bool {
	r.t.Helper()
	cmd := exec.Command(fixtureGit(), "-C", r.dir, "show-ref", "--verify", "--quiet", ref) //nolint:gosec // test fixture; argv slice
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	return cmd.Run() == nil
}

func (r *testRepo) mergeNoFF(branch string) {
	r.t.Helper()
	r.git("merge", "--no-ff", "-m", "merge "+branch, branch)
}

func (r *testRepo) addOriginAndHead() {
	r.t.Helper()
	bare := filepath.Join(r.t.TempDir(), "origin.git")
	cmd := exec.Command(fixtureGit(), "init", "--bare", "--initial-branch=main", bare) //nolint:gosec // test fixture; argv slice
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("fixture: bare origin: %v: %s", err, out)
	}
	r.git("remote", "add", "origin", filepath.ToSlash(bare))
	r.git("push", "origin", "main")
	r.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
}

func (r *testRepo) stashOn(branch string) {
	r.t.Helper()
	previous := r.git("rev-parse", "--abbrev-ref", "HEAD")
	r.git("checkout", branch)
	tracked := filepath.Join(r.dir, "file-01.txt")
	if err := os.WriteFile(tracked, []byte("dirty stash contents\n"), 0o600); err != nil {
		r.t.Fatalf("fixture: dirtying worktree: %v", err)
	}
	r.git("stash", "push", "-m", "WIP on "+branch+": test stash")
	r.git("checkout", previous)
}
