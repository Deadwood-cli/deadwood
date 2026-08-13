# Deadwood — End-to-End Implementation Spec

**Version:** 0.1 (MVP)
**Language:** Go
**Status:** Ready for agentic implementation

This document is the single source of truth for building Deadwood v0.1. It is written to be followed by an
AI coding agent with minimal ambiguity. Where a decision has been made, it is stated as a decision, not a
suggestion. Where something is explicitly deferred to a later version, it is marked **(v2+)** and must not be
built in v0.1.

---

## 1. Product Summary

**One-line pitch:** A CLI tool that safely identifies and deletes local Git branches that are dead on the
remote, without ever risking unmerged work.

**Primary user:** A developer with a local clone that has accumulated 50–500+ stale local branches because
remote branches are deleted automatically on PR merge, but local branches are never cleaned up.

**Core promise (non-negotiable):** Deadwood must never delete a branch that contains work not reachable from
the default branch, without explicit, informed user confirmation. Trust is the entire product. Every design
decision defers to safety over convenience when the two conflict.

### Out of scope for v0.1
- GitLab, Bitbucket, or any provider other than GitHub.
- VS Code extension or desktop GUI.
- Background daemon / scheduled scanning.
- SQLite cache (fresh scan every run in v0.1).
- Multi-repo / monorepo batch mode.

---

## 2. Tech Stack (decided)

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go (1.22+) | Single static binary, no runtime deps, fast startup, compiler safety for destructive logic |
| CLI framework | `cobra` | Standard for Go CLIs, subcommand structure, flag parsing |
| Interactive TUI | `charmbracelet/bubbletea` + `charmbracelet/bubbles` (list/checklist component) + `charmbracelet/lipgloss` (styling) | Best-in-class Go TUI ecosystem, matches devtool aesthetic expectations |
| Git operations | Shell out to system `git` via `os/exec` | Do NOT use `go-git`. Must respect user's credential helpers, hooks, LFS, SSH config, and match real `git` behavior exactly |
| GitHub API | REST v3 via `net/http` + manual structs, OR `go-github` client library | Use `go-github` (`github.com/google/go-github/v63`) — mature, avoids reinventing pagination/rate-limit handling |
| Auth storage | OS keychain via `github.com/zalando/go-keyring` | Never store PAT/token in plaintext config or env file |
| Config file | YAML via `gopkg.in/yaml.v3` | `.deadwood.yml`, human-editable |
| Testing | Standard `testing` package + `testify/assert` | Table-driven tests for classification engine |
| Distribution | `goreleaser` → Homebrew tap, npm wrapper optional later, raw binary releases on GitHub Releases | Not part of v0.1 build itself, but structure code so goreleaser works out of the box (single `main.go` entrypoint, semantic versioning via ldflags) |

---

## 3. Repository Structure

```
deadwood/
├── cmd/
│   └── deadwood/
│       └── main.go                 # entrypoint, wires cobra root command
├── internal/
│   ├── cli/
│   │   ├── root.go                 # root cobra command, global flags
│   │   ├── scan.go                 # `deadwood scan` command
│   │   ├── clean.go                # `deadwood clean` command
│   │   ├── undo.go                 # `deadwood undo` command
│   │   └── auth.go                 # `deadwood auth login/logout/status`
│   ├── git/
│   │   ├── runner.go                # thin wrapper around exec.Command("git", ...)
│   │   ├── branches.go               # list local branches, get metadata per branch
│   │   ├── merge.go                  # ancestor-merge detection (git merge-base --is-ancestor)
│   │   ├── backup.go                 # create/list/restore refs/deadwood-backup/*
│   │   └── repo.go                   # repo root detection, default branch detection
│   ├── github/
│   │   ├── client.go                 # go-github client construction, auth
│   │   ├── remote_branches.go        # list remote branches for repo
│   │   ├── pull_requests.go          # find merged PR by head ref (squash-merge detection)
│   │   └── ratelimit.go              # rate-limit aware request wrapper
│   ├── classify/
│   │   ├── classifier.go             # THE CORE ENGINE — see Section 5
│   │   ├── bucket.go                 # Bucket enum + BranchResult struct
│   │   └── classifier_test.go        # heaviest test coverage in the repo
│   ├── config/
│   │   ├── config.go                 # load/parse .deadwood.yml
│   │   └── defaults.go
│   ├── auth/
│   │   └── keyring.go                # token get/set/delete via OS keychain
│   ├── tui/
│   │   ├── checklist.go              # bubbletea model for interactive delete selection
│   │   └── styles.go                 # lipgloss style definitions
│   └── output/
│       └── report.go                 # non-interactive report printer (for `scan`, CI mode)
├── testdata/
│   └── fixtures/                     # scripted throwaway git repos for integration tests
├── go.mod
├── go.sum
├── .goreleaser.yml
├── README.md
└── LICENSE
```

**Rule for the agent:** `internal/classify` must have zero dependency on `internal/git` or `internal/github`
package types directly in its core decision function — it takes plain structs (`BranchInfo`, `RemoteStatus`,
`PRStatus`) as input so it can be unit tested with fixtures, not live repos. Wire the real data in at the
`cli` layer.

---

## 4. Data Models

```go
// internal/classify/bucket.go

package classify

type Bucket string

const (
    BucketSafeDelete   Bucket = "safe_delete"    // remote gone, fully merged (ancestor)
    BucketSquashMerged Bucket = "squash_merged"  // remote gone, merged via squash (PR match)
    BucketNeedsReview  Bucket = "needs_review"   // remote gone, cannot confirm merge status
    BucketActive       Bucket = "active"         // remote still exists
    BucketProtected    Bucket = "protected"       // excluded by config, current branch, or has stash/worktree

type BranchInfo struct {
    Name           string
    IsCurrent      bool
    HasWorktree    bool   // checked out in another worktree
    LastCommitSHA  string
    LastCommitDate time.Time
    AheadCount     int    // commits ahead of default branch merge-base
    BehindCount    int
    HasUpstream    bool   // had a tracked upstream configured at all
    UpstreamName   string // e.g. origin/feature-x
}

type RemoteStatus struct {
    Exists bool // does origin/<branch> still exist
}

type PRStatus struct {
    Found    bool
    Merged   bool
    Number   int
    MergedAt time.Time
}

type BranchResult struct {
    Branch     BranchInfo
    Bucket     Bucket
    Reason     string // human-readable explanation, always populated
    Confidence string // "high" | "medium" | "low" — surfaced in UI
}
```

---

## 5. Classification Engine — Full Decision Logic

This is the most important section of this spec. The agent must implement `Classify(branch BranchInfo,
remote RemoteStatus, pr PRStatus, defaultBranch string, cfg Config) BranchResult` exactly per this decision
tree. **Order matters — evaluate top to bottom, first match wins.**

```
1. IF branch.IsCurrent
   → BucketProtected, reason: "currently checked out"

2. IF branch.HasWorktree
   → BucketProtected, reason: "checked out in another worktree"

3. IF branch.Name matches any pattern in cfg.ExcludePatterns (glob match, e.g. "release/*", "main", "develop")
   → BucketProtected, reason: "matches exclude pattern in config"

4. IF branch has a git stash entry referencing it (see 5.1 below)
   → BucketProtected, reason: "has associated stash entry"

5. IF remote.Exists == true
   → BucketActive, reason: "remote branch still exists"

6. IF remote.Exists == false:
   a. Run: git merge-base --is-ancestor <branch> <defaultBranch>
      IF true (branch tip is an ancestor of default branch)
      → BucketSafeDelete, confidence: "high",
        reason: "fully merged into <defaultBranch>, remote deleted"

   b. ELSE IF pr.Found == true AND pr.Merged == true
      → BucketSquashMerged, confidence: "high",
        reason: "matched merged PR #<pr.Number> (merged <pr.MergedAt>), likely squash-merged"

   c. ELSE IF branch.AheadCount == 0 AND branch.BehindCount > 0
      # branch has no unique commits relative to default branch but isn't a strict ancestor
      # (can happen with rebased/amended history) — still flag but lower confidence
      → BucketSafeDelete, confidence: "medium",
        reason: "no unique commits relative to <defaultBranch>, remote deleted"

   d. ELSE
      → BucketNeedsReview, confidence: "n/a",
        reason: "remote deleted but branch has <AheadCount> unmerged commit(s), last activity <LastCommitDate>"
```

### 5.1 Stash detection
`git stash list` does not natively associate a stash with a branch by name reliably across all git versions.
v0.1 approach: parse `git stash list --format='%gd %s'` and match branch name substring in the stash message
(git auto-generates messages like `WIP on <branch>: ...`). This is a best-effort heuristic — document it as
such in code comments. False positives (flagging protected when not needed) are acceptable; false negatives
are not, so bias toward over-protecting.

### 5.2 Default branch detection
Determine via, in order of preference:
1. `git symbolic-ref refs/remotes/origin/HEAD` (strip `refs/remotes/origin/`)
2. If that fails (not set locally), call GitHub API `GET /repos/{owner}/{repo}` → `default_branch` field
3. If both fail, prompt the user interactively to select from `main`/`master`/`develop` if any exist locally,
   otherwise error out with a clear message — do not guess silently.

### 5.3 Squash-merge PR matching
Query GitHub: `GET /repos/{owner}/{repo}/pulls?state=closed&head={owner}:{branchName}` — if `go-github`
doesn't support head-filtering directly for this pattern, fall back to `search/issues` API with query
`repo:{owner}/{repo} type:pr head:{branchName} is:merged`. Cache results per-run in memory (map keyed by
branch name) to avoid duplicate calls — no persistent cache in v0.1.

**Rate limit handling:** batch branch names and check remaining rate limit before each call
(`client.RateLimits(ctx)`). If remaining budget is too low to check all branches individually, fall back to
listing all closed PRs once (paginated) and matching branch names against `head.ref` client-side, rather than
one API call per branch.

---

## 6. Git Operations Layer (`internal/git`)

All functions shell out to system `git` using `exec.Command`. Never use string-concatenated shell commands —
always pass args as a slice to avoid injection and quoting bugs.

Required functions:

```go
func ListLocalBranches(repoPath string) ([]BranchInfo, error)
func GetDefaultBranch(repoPath string) (string, error)
func IsAncestor(repoPath, branch, of string) (bool, error)     // git merge-base --is-ancestor
func AheadBehind(repoPath, branch, base string) (ahead, behind int, error)
func CurrentBranch(repoPath string) (string, error)
func ListWorktrees(repoPath string) ([]string, error)          // branch names checked out elsewhere
func ListStashRefs(repoPath string) ([]StashEntry, error)
func DeleteBranch(repoPath, branch string, force bool) error   // git branch -d or -D
func CreateBackupRef(repoPath, branch string) error            // tag refs/deadwood-backup/<branch> at current tip, BEFORE delete
func ListBackupRefs(repoPath string) ([]string, error)
func RestoreFromBackup(repoPath, branch string) error           // git branch <branch> refs/deadwood-backup/<branch>
func DeleteBackupRef(repoPath, branch string) error              // used by a future prune command, NOT called automatically in v0.1
func RemoteBranchExists(repoPath, branch string) (bool, error)  // git ls-remote --heads origin <branch>, or use cached remote list
```

**Important sequencing rule:** `CreateBackupRef` MUST be called and MUST succeed before `DeleteBranch` is
ever invoked. If `CreateBackupRef` fails for any branch, skip deleting that branch and report the failure —
never proceed to delete without a confirmed backup ref in place.

**Remote branch existence check performance:** Do not call `git ls-remote --heads origin <branch>` once per
local branch (slow, N round trips). Instead call `git ls-remote --heads origin` once to get the full remote
branch list, then do local set-membership checks against local branch names.

---

## 7. GitHub Integration (`internal/github`)

### 7.1 Auth flow
Use GitHub CLI-style **device flow** (OAuth Device Authorization Grant):
1. `deadwood auth login` → request device code from GitHub, print user code + verification URL
2. Poll for token per RFC 8628 until user authorizes in browser
3. Store resulting token in OS keychain under service name `deadwood`, account `github-token`
4. `deadwood auth status` reads keychain and (if present) validates token via a lightweight `GET /user` call
5. `deadwood auth logout` deletes the keychain entry

Fallback for environments without keychain access (headless CI, some Linux setups without a secret service
running): accept a `DEADWOOD_GITHUB_TOKEN` environment variable, checked before keychain lookup, with a
warning printed that this is less secure.

OAuth App requirements: register a GitHub OAuth App scoped to `repo` (read-only usage only — never write to
the repo via the API in v0.1). Client ID is compiled into the binary as a public constant (standard practice
for device flow — device flow does not require a client secret).

### 7.2 Repo resolution
Determine `owner/repo` from `git remote get-url origin`, supporting both SSH (`git@github.com:owner/repo.git`)
and HTTPS (`https://github.com/owner/repo.git`) forms. If origin is not a GitHub URL, print a clear error:
"Deadwood v0.1 only supports GitHub. GitLab support is planned." and exit non-zero — do not silently no-op.

---

## 8. CLI Commands — Exact Specification

### `deadwood scan`
- Default command behavior (also runs if `deadwood` invoked with no subcommand).
- Read-only. Never prompts for deletion.
- Flags:
  - `--json` — machine-readable output (for scripting/CI)
  - `--verbose` — show reason + confidence for every branch, including Active/Protected
  - `--config <path>` — override default `.deadwood.yml` location
- Output: grouped report by bucket, branch count summary at top, e.g.:
  ```
  Deadwood scan — 214 local branches

    ✅ Safe to delete       142
    🟡 Squash-merged        31
    ⚠️  Needs review         12
    🔵 Active (remote live)  25
    🔒 Protected              4

  Run `deadwood clean` to review and delete.
  ```

### `deadwood clean`
- Runs a scan, then launches the bubbletea interactive checklist for `BucketSafeDelete` +
  `BucketSquashMerged` branches (pre-checked) and `BucketNeedsReview` (unchecked, shown but requires manual
  check-in to include).
- Keybindings: `↑/↓` navigate, `space` toggle, `a` select all in current bucket, `n` select none, `enter`
  confirm, `q`/`esc` cancel without deleting.
- Confirmation step: after checklist submit, print final list + count, require typing `yes` (not just enter)
  before any deletion occurs, unless `--yes` flag passed (for scripting; still requires `--dry-run=false`
  explicitly to be extra safe — see below).
- Flags:
  - `--dry-run` (default `true`) — must be explicitly set to `false` OR paired with `--yes` to actually
    delete. **v0.1 default behavior deletes nothing unless the user takes an explicit additional action beyond
    just running the command.**
  - `--yes` — skip interactive confirmation (still respects `--dry-run`)
  - `--include-needs-review` — allow needs-review branches to be pre-checked (still requires individual
    confirmation via checklist)
- On confirmed delete: for each branch, `CreateBackupRef` → `DeleteBranch(force=false)`. If `-d` (safe
  delete) fails because git itself thinks it's unmerged (belt-and-suspenders — git's own safety check is a
  second line of defense on top of ours), report it as skipped, do NOT escalate to `-D` automatically. Print
  a summary at the end: deleted count, skipped count with reasons, backup ref location reminder.

### `deadwood undo <branch-name>`
- Restores a branch from `refs/deadwood-backup/<branch-name>` if it exists.
- If ambiguous or not found, list available backup refs.
- Flag: `--list` — just list all available backups with their original deletion context if determinable
  (commit date/message), without restoring.

### `deadwood auth login | logout | status`
Per Section 7.1.

---

## 9. Config File — `.deadwood.yml`

Located at repo root, optional (sensible defaults if absent).

```yaml
# .deadwood.yml
exclude_patterns:
  - "main"
  - "master"
  - "develop"
  - "release/*"
  - "hotfix/*"

# Branches older than this many days since last commit are flagged with extra caution
# in needs_review bucket (surfaced in UI, does not change bucket assignment in v0.1)
stale_warning_days: 90

# Default branch override — only needed if auto-detection fails or is ambiguous
default_branch: ""  # empty = auto-detect

# How long backup refs are kept before being eligible for a future `prune` command (v2+, not enforced in v0.1)
backup_retention_days: 30
```

Config parsing must be tolerant of a missing file (use `config.Defaults()`), and must fail loudly and
specifically on a malformed file (bad YAML) rather than silently falling back to defaults — a silently
ignored exclude pattern is a safety issue.

---

## 10. Testing Strategy

### 10.1 Classification engine (`internal/classify`) — highest priority, highest coverage
Table-driven tests covering every branch of the decision tree in Section 5, including:
- Current branch → protected
- Worktree branch → protected
- Exclude pattern match (exact + glob) → protected
- Stash-associated branch → protected
- Remote exists → active
- Remote gone + is ancestor → safe_delete/high
- Remote gone + not ancestor + matched merged PR → squash_merged/high
- Remote gone + not ancestor + no PR match + zero ahead count → safe_delete/medium
- Remote gone + not ancestor + no PR match + nonzero ahead count → needs_review
- Priority ordering: a branch that is BOTH current AND would otherwise be safe_delete must resolve to
  protected (protected checks run first) — explicit test for this ordering.

### 10.2 Git layer (`internal/git`)
Integration tests using real scripted throwaway repos in `testdata/fixtures/`, created via a setup script
(`testdata/fixtures/setup.sh`) that builds repos with known branch topologies (merged-ancestor branch,
squash-merge-simulated branch, branch with stash, branch checked out in a worktree, etc.) using plain `git`
commands, run in a temp directory, torn down after each test.

### 10.3 GitHub layer (`internal/github`)
Mock HTTP server (`httptest.NewServer`) returning canned responses for PR lookups and rate-limit headers — no
live network calls in tests. Verify rate-limit fallback behavior (Section 5.3) is triggered correctly when
mock returns low `X-RateLimit-Remaining`.

### 10.4 End-to-end smoke test
One test that runs `scan` against a fixture repo end-to-end (git layer + classify layer, GitHub layer mocked)
and asserts the report output groups branches into the expected buckets by count.

### 10.5 Manual beta validation (process, not code)
Before building the delete flow, ship `scan`-only builds to 3–5 real users against their real repos, per the
plan discussed earlier in this project. Do not proceed to building `clean`'s delete path until scan output
has been sanity-checked against at least one real 100+ branch repo.

---

## 11. Safety Checklist (must all be true before v0.1 ships)

- [ ] `clean` never deletes without an explicit `--yes` or typed `yes` confirmation
- [ ] `--dry-run` defaults to `true`
- [ ] Every delete is preceded by a successful backup ref creation
- [ ] `undo` is implemented and tested before `clean`'s real deletion is enabled in any release build
- [ ] Current branch is always protected, unconditionally, no config can override this
- [ ] Branches with worktrees are always protected, unconditionally
- [ ] `git branch -D` (force delete) is never called automatically — only `-d`, and failures are reported, not escalated
- [ ] Malformed config file fails loudly, never silently falls back
- [ ] Token never written to disk in plaintext (keychain only, or explicit env var opt-in with warning)
- [ ] Classification engine has 100% branch coverage of the Section 5 decision tree in tests

---

## 12. Build Phases (for the agent to execute in order)

1. **Scaffold**: repo structure, `go.mod`, cobra root command, empty subcommands that print "not implemented"
2. **Git layer**: implement and test everything in Section 6 against fixture repos, no GitHub involved yet
3. **Classification engine**: implement Section 5 against plain structs, full table-driven test suite, no
   real git/GitHub wiring yet — this can be built and fully tested in isolation
4. **Wire scan (local-only)**: connect git layer → classify layer, treat all branches as if remote check is
   skipped (stub `RemoteStatus`), get local `scan --verbose` output looking right
5. **GitHub auth + client**: device flow login, keychain storage, `auth status`
6. **Wire remote status + PR matching**: complete the classify inputs, `scan` now fully functional and accurate
7. **Config file support**: exclude patterns, default branch override
8. **`clean` command — dry run only**: interactive checklist UI, confirmation flow, but delete calls are
   stubbed/logged only, not executed
9. **Backup ref + real delete + `undo`**: implement together, test restore path before enabling real deletes
10. **Polish**: `--json` output, error messages, README, goreleaser config

Each phase should have passing tests before moving to the next. Do not implement phase 9 (real deletion)
until phases 1–8 are fully tested and phase 8's dry-run output has been manually reviewed.

---

## 13. Locked Decisions

- **Repo / module path:** `github.com/Deadwood-cli/deadwood`. This is the exact string to use in `go.mod`'s
  `module` directive and in all internal import paths (e.g. `github.com/Deadwood-cli/deadwood/internal/classify`).
  Note: Go module paths are conventionally lowercase, but GitHub's org slug is `Deadwood-cli` — GitHub's own
  routing is case-insensitive so this resolves fine either way, but the agent should use the exact casing
  above consistently everywhere it appears (go.mod, imports, goreleaser config, README badges/links) rather
  than mixing case.
- **Homebrew tap (future):** will live at `github.com/Deadwood-cli/homebrew-deadwood` when packaging is
  built — not part of v0.1, noted here so the org is set up with this in mind.
- **OAuth App:** register under the `Deadwood-cli` org (Settings → Developer settings → OAuth Apps), not
  under the personal `DelaMN1` account, so the app's ownership matches the project's ownership.

## 14. Open Decisions for the User (not yet decided — flag before Phase 5)

- License choice (MIT/Apache-2.0 assumed but not confirmed).
