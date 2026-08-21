# Deadwood

A CLI that safely identifies and deletes local Git branches whose remotes are gone, without ever
risking unmerged work.

If remote branches disappear on PR merge and your local clone keeps them forever, Deadwood classifies
every local branch and deletes only the ones that are provably merged — after you confirm.

v0.1 supports **GitHub** remotes only. You need a system `git`.

---

## For users

**Start here.** This is the whole loop: install, look, then delete only if you mean it.
`deadwood clean` is dry-run by default and will not delete anything until you pass `--dry-run=false`.

### 1. Install

Pick one.

**Release binary** (no Go required) — download the archive for your OS from
[the latest GitHub Release](https://github.com/Deadwood-cli/deadwood/releases/latest):

| You are on | Download |
|---|---|
| Windows (Intel / AMD) | `deadwood_*_windows_amd64.zip` |
| Windows (ARM) | `deadwood_*_windows_arm64.zip` |
| macOS Apple Silicon | `deadwood_*_darwin_arm64.tar.gz` |
| macOS Intel | `deadwood_*_darwin_amd64.tar.gz` |
| Linux (x86_64) | `deadwood_*_linux_amd64.tar.gz` |
| Linux (ARM64) | `deadwood_*_linux_arm64.tar.gz` |

Unpack `deadwood` (or `deadwood.exe`) and put it on your `PATH`. Optionally verify the file against
`checksums.txt` on the same release.

**Go 1.22+** (puts the binary in `$(go env GOPATH)/bin` — that directory must be on `PATH`):

```sh
go install github.com/Deadwood-cli/deadwood/cmd/deadwood@latest
```

Pin a version with `@v0.1.0` instead of `@latest` if you want a specific release.

A Homebrew tap is planned at `Deadwood-cli/homebrew-deadwood` and is not available yet.

Check it:

```sh
deadwood --help
```

### 2. Go to the clone you want to clean

Run Deadwood **inside** that repository, not from your home directory or a parent folder that
happens to be another git repo.

```sh
cd /path/to/your/github-clone
```

The remote must be GitHub. Other hosts are out of scope for v0.1.

### 3. Log in once (per machine)

```sh
deadwood auth login
```

GitHub shows a code; you approve it in the browser. The token is stored in the OS keychain, not in
a file. Deadwood only *reads* the API, but the OAuth app requests the `repo` scope so private
pull-request heads are visible — a stolen token can still write to GitHub.

Without login, `scan` still works. Squash-merged detection is skipped.

```sh
deadwood auth status
```

### 4. See what would happen (read-only)

```sh
deadwood scan
deadwood scan --verbose
```

| Bucket | Meaning | Offered for delete? |
|---|---|---|
| `safe_delete` | Remote gone, fully merged into the default branch | Yes |
| `squash_merged` | Remote gone, matched to a merged pull request | Yes |
| `needs_review` | Remote gone, merge status not confirmed | Yes, but unchecked unless `--include-needs-review` |
| `active` | Remote branch still exists | No |
| `protected` | Checked out, other worktree, stash, or excluded | No |

Nothing is deleted at this step.

### 5. Clean — dry-run first, then delete only if you intend it

```sh
deadwood clean
```

That prints a checklist and **changes nothing**. In the checklist: `↑`/`↓` move, `space` toggle,
`a` all in the current bucket, `n` none, `enter` confirm, `q`/`esc` cancel.

To actually delete (backup is created first; you type `yes`):

```sh
deadwood clean --dry-run=false
```

Use **one** equals sign. On Windows PowerShell, `--dry-run false` (a space) does **not** turn
dry-run off.

Skip the `yes` prompt with `--yes` only when you already understand the plan:

```sh
deadwood clean --yes --dry-run=false
```

Scripts (stdin is not a terminal) also need `--allow-nontty`. Do not add that flag in a normal
interactive session.

### 6. Undo if you need a branch back

```sh
deadwood undo --list
deadwood undo <branch>
```

Every real delete is preceded by a backup at `refs/deadwood-backup/<branch>`.

---

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
git clone https://github.com/Deadwood-cli/deadwood
cd deadwood
go build -o deadwood ./cmd/deadwood
go test ./...
go vet ./...
```

On macOS 15 or newer, build with Go 1.24+ so the binary includes `LC_UUID`. CI's `test` job uses
`stable` Go for that reason; `minimum-go` still holds the `go 1.22` floor in `go.mod`. Released
binaries are built with a current Go toolchain for the same reason.

`internal/classify` is the decision engine and must not import `internal/git` or `internal/github`.
See `deadwood-spec.md` and `AGENTS.md`.
