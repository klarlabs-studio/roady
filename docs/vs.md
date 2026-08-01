# Roady vs the alternatives

Opinionated, current as of 2026-05. PRs welcome to keep it honest.

## At a glance

| | Roady | Cursor rules / Claude.md / AGENTS.md | spec-kit | Backlog.md | Linear / Jira / GitHub Projects | Aider repo-map |
| --- | --- | --- | --- | --- | --- | --- |
| Survives `/clear` and session resets | ✅ | ✅ (static) | ✅ | ✅ | ✅ | ❌ |
| Plan that an agent can read **and write** | ✅ | ❌ | ❌ | partial | ❌ | ❌ |
| Drift detection (intent ↔ plan ↔ code) | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Source citation per task (`from doc:line`) | ✅ | ❌ | partial | ❌ | ❌ | ❌ |
| MCP-native, every operation a tool | ✅ | ❌ | ❌ | ❌ | (third-party) | ❌ |
| File-based, git-versioned, local-first | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ |
| Free / open source | ✅ | varies | ✅ | ✅ | freemium | ✅ |
| Designed for AI-paired workflow | ✅ | ✅ | partial | ❌ | ❌ | ✅ |
| Hash-chained audit log | ✅ | ❌ | ❌ | ❌ | (server-side) | ❌ |

## When you should use Roady

- You pair with an AI coding agent on multi-day work and lose state
  between sessions.
- You want the plan-of-record in the repo, not in a SaaS your agent
  cannot reach.
- You want drift detection — to know *exactly* when reality stopped
  matching what you said you'd build.

## When you should NOT use Roady

- You are running a 50-person engineering org with full PM ceremony
  around Linear / Jira. Roady is not a replacement; it is a
  complement at most.
- You don't use AI coding agents at all. Roady's value is much
  smaller; pick a lighter spec-kit-style tool.
- You want a hosted SaaS today. Roady is a local CLI + MCP server.
  Roady Cloud (open-core) is on the [roadmap](../ROADMAP.md), not
  available yet.

## Detailed comparisons

### vs Cursor rules / Claude.md / AGENTS.md

Cursor rules and `Claude.md` are *static instructions* the agent reads
each turn. They don't track what was decided yesterday, what's done,
what's blocked, or whether the code matches the spec. They are
constants; Roady is state.

Combine them. Use Roady for plan + drift; use Claude.md for stable
project conventions.

### vs spec-kit (GitHub)

spec-kit is for *writing* specs. Roady is for keeping spec, plan, and
execution in sync over time. Roady's `roady spec analyze` reads
spec-kit-style markdown without modification.

Combine them. Author specs however you like; let Roady track the
execution loop.

### vs Backlog.md

Backlog.md ships a markdown task file. Lightweight, no plan/spec
relationship, no drift detection, no MCP. If your workflow is "list of
tasks in a file", Backlog.md is fine. If you need provenance from
spec to task, drift between intent and code, or AI agent integration,
you need Roady.

### vs Linear / Jira / GitHub Projects

These are built for human PMs and human eng teams. They have
sprints, swimlanes, custom fields, and assignees. They do not have
drift detection or source citations from spec docs.

They **do** have MCP now, and this page previously claimed otherwise.
Linear ships an official hosted MCP server, and Atlassian's Rovo MCP
server reached GA in February 2026 covering Jira, Confluence, JSM,
Bitbucket, and Compass. "Agents cannot reach them" is no longer true
and has not been since early 2026. What Roady still has that they do
not is the spec-lock/drift loop and `from doc:line` provenance.

Roady ships a `roady-plugin-linear` / `roady-plugin-jira` /
`roady-plugin-github` syncer. Be aware of its current limits: it
**creates** issues from tasks and reads status back, but Roady does
not yet push status changes out — the `Syncer.Push` method exists and
is implemented by every plugin, but nothing in the CLI or MCP calls
it. Inbound mapping covers `done` and `in_progress` only; `blocked`,
`pending`, and `verified` are dropped. No field beyond title,
description, and status maps in either direction. Closing that gap is
tracked as the top integration priority.

### vs `dadbodgeoff/drift` (the GitHub project, not the concept)

A separate open-source project literally named `drift`:
[github.com/dadbodgeoff/drift](https://github.com/dadbodgeoff/drift),
*"Codebase intelligence for AI. Detects patterns & conventions +
remembers decisions across sessions. MCP server for any IDE. Offline
CLI."*

Different primitive: it remembers *coding conventions and decisions*
("we use this lib for X, we agreed not to do Y") so the agent stops
re-asking. Roady remembers *plans and specs* ("we agreed to build
feature X with requirements A, B, C; here's what's done and what
diverged"). They compose well — `drift` makes the agent stop
hallucinating conventions; Roady makes the agent stop drifting from
intent.

If you arrive here from `dadbodgeoff/drift` confused, the short
version: same MCP-server-for-AI category, different layer of the
stack. Use both.

### vs BMAD-METHOD

A larger, more ceremonious framework — clear role separation,
comprehensive documentation. Recommended for *large* projects with
formal-process tolerance. Roady is the lightweight option: spec /
plan / drift in `.roady/` files, no role hierarchy, no documentation
ceremony unless you want it. If your team has a Scrum Master and a
PMO, BMAD is probably a better cultural fit. If your team is one
human and three agents, Roady is closer to the metal.

### vs Vibe Kanban

Newer competitor in the AI-PM-layer space (surfaced by HN
commenters as the potential "winning tool"). Different UI metaphor
(Kanban board) and not yet broadly distributed. Worth tracking. We
will refresh this row as their public surface clarifies.

### vs Aider repo-map / Sweep

Aider's repo-map and Sweep's context tooling fetch *read-time*
context for the model. Roady tracks *write-time* state — what was
decided, what's done. They solve different problems and compose well.

### vs "nothing — chat history + memory"

The default. Lossy across `/clear`. No shared truth between sessions.
No way for a colleague (or another agent) to pick up your work
without a verbal handover. Works until it doesn't, usually around
day 3 of a feature.

## Honest limitations

- **Single-repo plans dominate.** `roady org` aggregates across
  repos but each repo still owns its `.roady/`. Cross-repo planning
  is on the roadmap.
- **No real-time multi-user UI.** State syncs via git push/pull plus
  optimistic locking. Two collaborators editing in parallel will
  conflict on `.roady/state.json` like any other file.
- **Heuristic planner is intentionally simple** (1 requirement = 1
  task). Real complexity needs `--ai`. The eval harness in
  `evals/` keeps both planners honest.
- **Provider streaming exists but Confidence/Sources from real
  providers is partial.** Confidence comes from `stop_reason`
  heuristics; native API source citations only land for providers
  that expose them (Gemini grounding metadata being the closest).
