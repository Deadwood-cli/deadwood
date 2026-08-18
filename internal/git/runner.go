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
	"strings"
)

// binary is the git executable to invoke, resolved through PATH.
const binary = "git"

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
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), detail)
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

// run invokes git inside repoPath and returns stdout with trailing newlines
// stripped.
func run(repoPath string, args ...string) (string, error) {
	full := make([]string, 0, len(args)+3)
	full = append(full, "-C", repoPath, "--no-pager")
	full = append(full, args...)

	cmd := exec.Command(binary, full...)
	cmd.Env = commandEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &CommandError{
				Args:     full,
				ExitCode: exitErr.ExitCode(),
				Stderr:   stderr.String(),
			}
		}
		return "", fmt.Errorf("running git %s: %w", strings.Join(full, " "), err)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// commandEnv keeps git non-interactive and its output stable enough to parse.
func commandEnv() []string {
	return append(os.Environ(),
		// A scan must never block waiting for a password prompt.
		"GIT_TERMINAL_PROMPT=0",
		// Read-only commands have no business taking the index lock.
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_PAGER=cat",
		// Keep diagnostics in English so error matching stays reliable.
		"LC_ALL=C",
	)
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

// validateRefArgument rejects names git would parse as an option. Ref names
// cannot begin with a dash, so anything that does is a caller bug or an
// injection attempt rather than a real branch.
func validateRefArgument(name string) error {
	switch {
	case name == "":
		return errors.New("branch name is empty")
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("refusing to use %q as a branch name: it would be read as an option", name)
	default:
		return nil
	}
}
