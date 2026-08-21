# Deadwood

A CLI that safely identifies and deletes local Git branches whose remotes are gone, without ever
risking unmerged work.

If remote branches disappear on PR merge and your local clone keeps them forever, Deadwood classifies
every local branch and deletes only the ones that are provably merged — after you confirm.

v0.1 supports **GitHub** remotes only.

## Install

Requires a system `git`. Released binaries are built with a current Go toolchain (needed for macOS
15+ `LC_UUID` support).

**From source** (Go 1.22+):

```sh
go install github.com/Deadwood-cli/deadwood/cmd/deadwood@latest
```

Or clone and build:

```sh
git clone https://github.com/Deadwood-cli/deadwood
cd deadwood
go build -o deadwood ./cmd/deadwood
```

GitHub Release archives will be produced by goreleaser from version tags (`v*`). A Homebrew tap is
planned at `Deadwood-cli/homebrew-deadwood` and is not part of v0.1.

## Safety model

Trust is the product. These rules are not optional:

- A branch with work unreachable from the default branch is never deleted without explicit,
  informed confirmation.
- `clean --dry-run` defaults to **true**. `deadwood clean` and `deadwood clean --yes` print a plan
  and change nothing. Real deletion requires `--dry-run=false` (one equals sign) **and** either
  typed `yes` or `--yes`. Scripts (stdin not a terminal) also need `--allow-nontty`.
- Classification is re-checked immediately before delete. If a branch is no longer safe or
  squash-merged (or, for needs-review, is now protected/active), it is skipped.
- The confirmation screen prints the repository path, config file, default branch, and exclude list.
- Every deletion is preceded by a successful backup at `refs/deadwood-backup/<branch>`, restorable
  with `deadwood undo <branch>`.
- Only `git branch -d` is used. Force-delete (`-D`) is never invoked. If git considers a branch
  unmerged, Deadwood reports it as skipped.
- The current branch and any branch checked out in another worktree are always protected. Config
  cannot override that.

## Commands

| Command | What it does |
|---|---|
| `deadwood` / `deadwood scan` | Read-only report. Default command. |
| `deadwood scan --verbose` | Include reason and confidence for every branch. |
| `deadwood scan --json` | Machine-readable report for scripts and CI. |
| `deadwood clean` | Checklist of candidates. Dry-run by default. |
| `deadwood clean --yes --dry-run=false --allow-nontty` | Scripted delete: skip the prompt, actually delete (after backup). |
| `deadwood undo <branch>` | Restore a deleted branch from its backup ref. |
| `deadwood undo --list` | List backup refs without restoring. |
| `deadwood auth login \| logout \| status` | GitHub device-flow credentials in the OS keychain. |

Keybindings in `clean`: `↑/↓` move, `space` toggle, `a` all in the current bucket, `n` none, `enter`
confirm, `q`/`esc` cancel.

## Classification buckets

| Bucket | Meaning |
|---|---|
| `safe_delete` | Remote gone, fully merged into the default branch. |
| `squash_merged` | Remote gone, matched to a merged pull request. |
| `needs_review` | Remote gone, but merge status could not be confirmed. |
| `active` | Remote branch still exists. |
| `protected` | Checked out, in a worktree, has a stash, or excluded by config. |

`clean` only offers `safe_delete`, `squash_merged`, and `needs_review`. Needs-review rows start
unchecked unless you pass `--include-needs-review`.

## Configuration

Optional `.deadwood.yml` at the repository root. A missing file uses the defaults below. A malformed
file is a hard error — never a silent fallback — because a silently ignored exclude pattern is a
safety problem. `--config <path>` overrides the location; a missing override path also errors.

Listed `exclude_patterns` **replace** the defaults; they are not merged. An explicit empty list
turns off those defaults (the current branch and worktrees stay protected) and prints a warning.

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

`stale_warning_days` is shown on needs-review rows in the checklist. It does not change the bucket.
`backup_retention_days` is stored for a future prune command and is not enforced in v0.1.

## Authentication

Device flow; token in the OS keychain, never in a file. The OAuth App requests the `repo` scope so
private pull-request heads are visible. Deadwood only reads the API; a stolen token can still write
to GitHub. `DEADWOOD_GITHUB_CLIENT_ID` is ignored unless you pass `--allow-client-id-override`.

```sh
deadwood auth login
deadwood auth status
```

`DEADWOOD_GITHUB_TOKEN` is accepted with a warning. Do not put tokens in `.env` or commit them.
Any program running as your OS user can read the keychain item.

Without login, scan still classifies remotes; squash-merged detection is skipped.

## JSON

`deadwood scan --json` (also `deadwood --json`) emits `default_branch`, `counts` per bucket, and a
`branches` array with `name`, `bucket`, `reason`, `confidence`, `last_commit_sha`, and
`last_commit_date`.

## Development

```sh
go build -o deadwood ./cmd/deadwood
go test ./...
go vet ./...
```

On macOS 15 or newer, build with Go 1.24+ so the binary includes `LC_UUID`. CI's `test` job uses
`stable` Go for that reason; `minimum-go` still holds the `go 1.22` floor in `go.mod`.

`internal/classify` is the decision engine and must not import `internal/git` or `internal/github`.
See `deadwood-spec.md` and `AGENTS.md`.
