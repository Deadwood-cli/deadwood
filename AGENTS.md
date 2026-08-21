# AGENTS.md — Standing Instructions for Agentic Work on Deadwood

This file is for any coding agent (or human) working on this repo across sessions. Read this before
starting any task. It is glue between `deadwood-spec.md` (the implementation spec) and day-to-day execution.

## 0. Source of truth

`deadwood-spec.md` at the repo root is the canonical spec. If anything in this file conflicts with it,
the spec wins. This file only adds process/convention guidance the spec doesn't cover.

## 1. Non-negotiable safety rules

These apply regardless of what task you're currently working on, even if the task seems unrelated:

- Never implement, wire up, or enable a code path that calls `DeleteBranch` without a preceding, successful
  `CreateBackupRef` call for that exact branch. This is a hard sequencing rule, not a style preference.
- Never call `git branch -D` (force delete). Only `-d` is permitted anywhere in this codebase. If a delete
  fails because git considers it unmerged, that is a signal to report and skip — not to escalate.
- Never change `clean`'s default `--dry-run` behavior away from `true` without an explicit instruction from
  the project owner in the current conversation. Don't infer this is fine because "the tests pass."
  Real deletion behavior is out of scope until Build Phase 9 (see spec Section 12) and Section 11's safety
  checklist is fully satisfied.
  - "Fully satisfied" means: `undo` is implemented and has passing tests restoring a deleted branch from a
  backup ref before enabling real deletion in any code path.
- The classification engine (`internal/classify`) must remain testable in isolation from `internal/git` and
  `internal/github`. If a change requires importing those packages into `classify`, stop and reconsider —
  that's a sign the decision logic is leaking into I/O code, which is what Section 4/5 of the spec exists to
  prevent.
- Never store a GitHub token in plaintext anywhere in the repo, in test fixtures, in logs, or in committed
  config. Keychain or the `DEADWOOD_GITHUB_TOKEN` env var only.

## 2. Build order discipline

Follow the phase order in spec Section 12. Do not skip ahead to a later phase because it seems easy or
related — in particular, do not implement real branch deletion (Phase 9) before Phases 1–8 have passing
tests and Phase 8's dry-run output has been reviewed by the project owner. If you believe skipping ahead is
justified, ask first rather than proceeding.

Each phase should end with:
1. Passing tests for the code written in that phase (`go test ./...` clean)
2. `go vet ./...` clean
3. A short summary of what was completed and what the next phase depends on

## 3. Testing conventions

- Table-driven tests for `internal/classify` — every branch of the Section 5 decision tree in the spec needs
  an explicit test case, including the priority-ordering case (protected checks win over safe_delete).
- Integration tests for `internal/git` use scripted fixture repos under `testdata/fixtures/`, created in a
  temp directory and torn down after the test — never mutate a fixture repo in place, never touch the real
  working repo.
- Mock all GitHub API calls in tests (`httptest.NewServer`). No live network calls in the test suite, ever.
- Before marking any phase "done," run the full suite, not just tests for files touched in that session.

## 4. Branching conventions

`main` is protected (see Section 4a). All work happens on a branch, merged via PR — no direct pushes to
`main`, including by whoever is doing the agentic work.

Branch naming, matching the commit-type prefixes below:
- `feat/<short-description>` — new functionality (e.g. `feat/classify-engine`, `feat/scan-command`)
- `fix/<short-description>` — bug fixes
- `test/<short-description>` — test-only additions/changes
- `docs/<short-description>` — spec/README/AGENTS.md updates
- `chore/<short-description>` — tooling, CI, dependency bumps

One branch per logical unit of work — prefer matching a branch to a single build phase (spec Section 12) or
a clearly-scoped sub-piece of one, rather than combining unrelated changes. Small, reviewable PRs are
preferred over large ones, especially for anything touching `internal/classify` or `internal/git`'s delete
path.

Delete the branch after merge (GitHub's "delete branch" button on merge, or manually) — yes, this project is
itself the reason to keep local branch hygiene tight. Once `deadwood` itself is functional, it's fair game to
use it on its own repo.

### 4a. Branch protection (configure once, in GitHub repo settings → Branches)

- Require a pull request before merging to `main` — no direct pushes, including from the repo owner
- Require status checks to pass before merging: both the `test` matrix jobs and the `lint` job from
  `.github/workflows/test.yml`
- Require branches to be up to date before merging
- Do not require signed commits or a minimum number of approvals for now (solo project) — revisit if/when
  collaborators join

### 4b. Release tags (configure once, in GitHub repo settings → Rules / Tags)

Protect `v*` tags so only trusted actors can create them. goreleaser publishes binaries from any `v*`
tag push; a stolen tag is a supply-chain incident. Prefer SHA-pinned Actions (see `.github/workflows`).

## 5. Commit conventions

- Conventional commit style: `feat:`, `fix:`, `test:`, `docs:`, `refactor:`, `chore:`.
- One phase (per spec Section 12) may span multiple commits — commit at logical checkpoints within a phase,
  not just once at the end.
- Never commit with failing tests. If a phase is left incomplete at the end of a session, commit working,
  tested partial progress and leave a note in the commit body about what's unfinished — don't commit broken
  intermediate states.

## 6. Repo/ownership context

- Module path: `github.com/Deadwood-cli/deadwood` (see spec Section 13) — use this exact casing everywhere.
- Repo lives under the `Deadwood-cli` GitHub org, not the personal `DelaMN1` account. Any OAuth App, future
  Homebrew tap, or CI secrets should be registered/configured under the org.
- License: not yet finalized (spec Section 14) — do not add a `LICENSE` file or SPDX headers until this is
  confirmed by the project owner.

## 7. When in doubt

If a task requires a decision this file and the spec don't cover — especially anything touching deletion
behavior, auth/token handling, or config parsing fallbacks — stop and ask rather than choosing the
convenient interpretation. This project's entire value proposition is trust; a wrong guess here costs more
than a delayed answer.