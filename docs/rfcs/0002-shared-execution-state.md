# RFC 0002: Execution state shared across worktrees

- Status: accepted (2026-09-27)
- Task: `task-shared-execution-state`

## Problem

Parallel agents usually work in separate git worktrees, one branch each.
Everything in `.roady/` is a file in the checkout, so each worktree has its
own `state.json`. An agent in worktree A claims a task; the agent in worktree
B cannot see the claim and starts the same task. A completion in A is
invisible to B until the branches merge — and then `state.json` conflicts.
Claims (`task-leased-claims`) only work if everyone reads the same
state.

## Options

1. **A git ref** (`refs/roady/state`). State is versioned and travels with
   `git push`, so other machines can see it. But every write is a commit
   through git plumbing, concurrent writers race on the ref, and reads cost a
   `git` process. Heavy for a file rewritten on every transition.
2. **A file in the common git directory** (`git rev-parse --git-common-dir`,
   i.e. the main `.git/`). Every worktree of the repository shares it, so a
   claim is visible to all of them the moment it is written, with the same
   locking and atomic writes as today. It is local to the machine and not
   versioned.
3. **Keep `state.json` in the checkout** (status quo). Forked per branch.

## Decision

Option 2. The goal is that parallel agents on one machine see each other's
claims immediately; that is exactly what the common git directory gives, at
no extra cost. What it gives up — versioning and other machines — is already
covered: `events.jsonl` stays in the checkout and is committed, and
`roady state rebuild` reconstructs state from it anywhere.

## Design

- The authoritative state lives at
  `<git-common-dir>/roady/<project-dir>/state.json`, where `<project-dir>` is
  the project's directory relative to the worktree root (`.roady`, or
  `.roady/projects/<name>` for a sub-project). The same path in every
  worktree resolves to the same file; two roady projects in one repository
  stay apart.
- The common directory is found without running git: walk up from the
  project root to `.git`; a directory is the common dir, a file names the
  worktree's git dir, whose `commondir` file points at the common dir.
- Writes take the lock and version check next to the shared file, so a claim
  made in one worktree conflicts with a start in another exactly as two
  processes in one checkout do.
- `.roady/state.json` in the checkout is still written after each save, as
  a mirror, so a commit still shows progress to reviewers. It is never read
  while the shared file exists.
- First use seeds the shared file from the checkout's `state.json`, so
  adopting this loses nothing.
- Outside a git repository, or with `shared_state: false` in `policy.yaml`,
  state stays in `.roady/state.json` as before.

## Consequences

- Worktrees see each other's claims and completions immediately.
- A fresh clone starts from the committed mirror, or from `roady state
  rebuild`.
- Deleting `.git/roady/` loses nothing that the event log cannot rebuild.
