// Package git wraps the system git binary.
//
// Every operation shells out to real git through os/exec rather than using a
// pure-Go implementation, so the user's credential helpers, hooks, LFS filters
// and SSH configuration behave exactly as they do on the command line.
// Arguments are always passed as a slice; no command is ever assembled as a
// shell string.
package git

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

const fallbackGit = "git"

var (
	gitPathOnce sync.Once
	gitPath     string

	// secretPattern matches common GitHub token prefixes so they never appear
	// in CommandError output if a credential helper echoes them to stderr.
	secretPattern = regexp.MustCompile(`(?i)\b(?:gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]{8,})`)
)

// gitBinary is the git executable to invoke, resolved through PATH once.
func gitBinary() string {
	gitPathOnce.Do(func() {
		names := []string{fallbackGit}
		if runtime.GOOS == "windows" {
			// Prefer git.exe over git.cmd; the latter is a cmd.exe wrapper that
			// can hang when stdin is a pipe.
			names = []string{"git.exe", fallbackGit}
		}
		for _, name := range names {
			path, err := exec.LookPath(name)
			if err == nil {
				gitPath = path
				return
			}
		}
		gitPath = fallbackGit
	})
	return gitPath
}

// CommandError describes a git invocation that ran to completion and reported
// failure. Errors from git failing to start are returned unwrapped instead.
type CommandError struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *CommandError) Error() string {
	detail := strings.TrimSpace(e.Stderr)
	if detail == "" {
		detail = fmt.Sprintf("exit status %d", e.ExitCode)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), redactSecrets(detail))
}

// exitCodeOf reports the exit status behind err when git ran and failed. Git
// uses the exit status as a return value in places (merge-base --is-ancestor,
// symbolic-ref --quiet), so callers need to tell "answered no" from "broke".
func exitCodeOf(err error) (code int, ok bool) {
	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		return cmdErr.ExitCode, true
	}
	return 0, false
}

// stderrContains reports whether a failed git invocation printed the given
// text. Command environments force LC_ALL=C so this text is stable.
func stderrContains(err error, text string) bool {
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		return false
	}
	return strings.Contains(strings.ToLower(cmdErr.Stderr), strings.ToLower(text))
}

func redactSecrets(s string) string {
	return secretPattern.ReplaceAllString(s, "[redacted]")
}

// run invokes git inside repoPath and returns stdout with trailing newlines
// stripped. Read-only commands skip optional index locks.
func run(repoPath string, args ...string) (string, error) {
	return runWith(repoPath, false, args...)
}

// runMutating is run for commands that update refs. It does not set
// GIT_OPTIONAL_LOCKS=0, so ref updates take their required locks.
func runMutating(repoPath string, args ...string) (string, error) {
	return runWith(repoPath, true, args...)
}

func runWith(repoPath string, mutating bool, args ...string) (string, error) {
	full := make([]string, 0, len(args)+3)
	full = append(full, "-C", repoPath, "--no-pager")
	full = append(full, args...)

	// gitBinary is a PATH lookup of git/git.exe, never a shell string.
	cmd := exec.Command(gitBinary(), full...) //nolint:gosec // argv slice; binary from LookPath
	cmd.Env = commandEnv(mutating)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &CommandError{
				Args:     full,
				ExitCode: exitErr.ExitCode(),
				Stderr:   redactSecrets(stderr.String()),
			}
		}
		return "", fmt.Errorf("running git %s: %w", strings.Join(full, " "), err)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// commandEnv keeps git non-interactive and its output stable enough to parse.
// DEADWOOD_GITHUB_TOKEN is stripped so a GitHub credential never leaks into
// child git processes that do not need it.
func commandEnv(mutating bool) []string {
	var incomingCeiling string
	env := make([]string, 0, 16)
	for _, kv := range os.Environ() {
		key, value, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		switch {
		case strings.EqualFold(key, "DEADWOOD_GITHUB_TOKEN"):
			continue
		case strings.EqualFold(key, "GIT_OPTIONAL_LOCKS"),
			strings.EqualFold(key, "GIT_TERMINAL_PROMPT"),
			strings.EqualFold(key, "GIT_PAGER"),
			strings.EqualFold(key, "LC_ALL"):
			continue
		case strings.EqualFold(key, "GIT_CEILING_DIRECTORIES"):
			incomingCeiling = value
			continue
		}
		env = append(env, kv)
	}

	env = append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"LC_ALL=C",
	)
	if !mutating {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}

	ceiling := incomingCeiling
	if ceiling == "" {
		if home, err := os.UserHomeDir(); err == nil {
			ceiling = home
		}
	}
	if ceiling != "" {
		env = append(env, "GIT_CEILING_DIRECTORIES="+ceiling)
	}
	return env
}

// lines splits git output into records, discarding the empty trailing record
// that a final newline would otherwise produce.
func lines(out string) []string {
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// fieldSep separates fields inside a single --format record. Git forbids ASCII
// control characters in ref names, so the unit separator cannot collide with
// any branch name.
//
// The two format placeholders produce the same byte but are not
// interchangeable: for-each-ref takes bare hex digits, while the pretty formats
// used by log and stash list require an %x prefix and pass %1f through as
// literal text.
const (
	fieldSep = "\x1f"

	fieldSepForEachRef = "%1f"
	fieldSepPretty     = "%x1f"
)

const refForbidden = "~^:?*[]\\"

// validateRefArgument rejects names git would parse as an option or a rev
// walk. Ref names cannot begin with a dash; ".." would turn AheadBehind's
// `base...branch` argument into an extra range.
func validateRefArgument(name string) error {
	switch {
	case name == "":
		return errors.New("branch name is empty")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("refusing to use %q as a branch name: it would be read as an option", name)
	case strings.Contains(name, ".."):
		return fmt.Errorf("refusing to use %q as a branch name: it contains '..'", name)
	}
	for _, r := range name {
		if r < 32 || r == 127 || unicode.IsSpace(r) {
			return fmt.Errorf("refusing to use %q as a branch name: it contains a control or space character", name)
		}
		if strings.ContainsRune(refForbidden, r) {
			return fmt.Errorf("refusing to use %q as a branch name: it contains %q", name, r)
		}
	}
	return nil
}
