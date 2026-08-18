# Deadwood

A CLI that safely identifies and deletes local Git branches that are dead on the remote, without ever
risking unmerged work.

If you work on a repo where remote branches are deleted automatically on PR merge, your local clone
slowly fills with branches that no longer mean anything. Deadwood tells you which of them are provably
merged, which need a human look, and which to leave alone.

> **Status: pre-release.** v0.1 is under active development. `deadwood clean` deletes nothing by
> default and real deletion is not enabled yet.

## Safety model

Trust is the whole product, so the guarantees come before the features:

- A branch containing work unreachable from the default branch is never deleted without explicit,
  informed confirmation.
- `clean` runs in `--dry-run` mode by default. Deleting requires an additional deliberate action.
- Every deletion is preceded by a backup ref under `refs/deadwood-backup/`, restorable with
  `deadwood undo`.
- Deadwood only ever uses `git branch -d`. Force deletion is never invoked, and a branch git itself
  considers unmerged is reported and skipped rather than escalated.
- The branch you have checked out, and any branch checked out in another worktree, are always
  protected. No configuration can override this.

## Commands

| Command | What it does |
|---|---|
| `deadwood scan` | Read-only report grouping every local branch into a bucket. Runs by default. |
| `deadwood clean` | Interactive checklist to review and delete dead branches. |
| `deadwood undo <branch>` | Restore a deleted branch from its backup ref. |
| `deadwood auth` | Manage GitHub credentials (`login`, `logout`, `status`). |

## Classification buckets

| Bucket | Meaning |
|---|---|
| `safe_delete` | Remote gone, fully merged into the default branch. |
| `squash_merged` | Remote gone, matched to a merged pull request. |
| `needs_review` | Remote gone, but merge status could not be confirmed. |
| `active` | Remote branch still exists. |
| `protected` | Checked out, in a worktree, has a stash, or excluded by config. |

## Configuration

Optional `.deadwood.yml` at the repository root. A missing file is fine; a malformed one is a hard
error rather than a silent fallback to defaults, because a silently ignored exclude pattern is a
safety problem.

```yaml
exclude_patterns:
  - "main"
  - "master"
  - "develop"
  - "release/*"
  - "hotfix/*"

stale_warning_days: 90
default_branch: "" # empty = auto-detect
backup_retention_days: 30
```

## Authentication

Deadwood uses the GitHub device flow and stores the resulting token in your OS keychain. It is never
written to disk in plaintext. For headless environments without a keychain, set
`DEADWOOD_GITHUB_TOKEN` instead.

```sh
deadwood auth login
deadwood auth status
```

## Development

Requires Go 1.22 or newer and a system `git`.

On macOS 15 or newer, build with Go 1.24+. Earlier toolchains omit the `LC_UUID` load command that
Apple's loader now requires, so the binaries they produce refuse to start. Released binaries are
unaffected.

```sh
go build ./cmd/deadwood
go test ./...
go vet ./...
```

`internal/classify` holds the decision engine and carries the heaviest test coverage in the repo. It
depends on neither `internal/git` nor `internal/github` so it stays unit-testable against fixtures.
See `deadwood-spec.md` for the full specification and `AGENTS.md` for contribution conventions.
