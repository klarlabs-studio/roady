<p align="center">
  <img src="logo.svg" width="150" alt="Roady Logo">
</p>

[![Go Version](https://img.shields.io/github/go-mod/go-version/felixgeelhaar/roady?logo=go)](https://github.com/felixgeelhaar/roady)
[![Coverage](https://img.shields.io/badge/coverage-82%25-brightgreen?logo=coveralls)](https://github.com/felixgeelhaar/roady/actions)
[![Release](https://img.shields.io/github/v/release/felixgeelhaar/roady?include_prereleases&logo=github)](https://github.com/felixgeelhaar/roady/releases/latest)
[![nox Security](https://img.shields.io/badge/nox-A-brightgreen?logo=lock)](https://github.com/felixgeelhaar/roady/security)
[![nox Scan](https://img.shields.io/badge/scan-0%20findings-brightgreen)](https://github.com/felixgeelhaar/roady/security)

# Roady — the plan-of-record for AI coding agents

> Spec, plan, and drift detection that **survive context resets**.
> File-based, git-versioned, MCP-native.

> *"With multiple Claude agents running in parallel, I'd lose track of
> specs, dependencies, and history."* — verbatim from a 2026 [Show HN
> thread](https://news.ycombinator.com/item?id=44960594).

You pair with Claude Code, Codex, Cursor, or Gemini on a multi-day
feature. Three days in, the agent forgets what was decided, rewrites the
wrong thing, or quietly drifts off-spec. Roady is the durable layer
that holds the answer to *what are we building, what's next, and where
did reality diverge from the plan?* — readable by you and writable by
your agent.

## See it in 60 seconds

```bash
brew install felixgeelhaar/tap/roady     # or: go install github.com/felixgeelhaar/roady/cmd/roady@latest
roady demo                               # scaffolds a sample project + shows drift
```

The demo creates a `roady-demo/` directory with a deliberately drifted
spec/plan, runs `roady drift detect`, and prints the next steps. Zero
prerequisites, zero AI keys, zero signup.

## The actual workflow

```bash
# 1. Hook your agent to Roady (one command per supported tool)
roady setup claude-code           # or claude-desktop, opencode, openai, gemini

# 2. Initialise + import your existing docs
roady init my-project
roady spec analyze docs/          # parses markdown, captures source citations

# 3. Generate a plan (heuristic by default; --ai for richer decomposition)
roady plan generate
roady plan approve

# 4. Drive execution from inside your AI editor
/roady-task                       # agent picks the next ready task
# ...agent implements, commits with [roady:task-id] marker...
roady git sync                    # state moves forward automatically

# 5. Ask the question that matters
roady drift detect                # has reality diverged from intent?
```

Status, drift, and progress all show in `roady status` — including a
`from doc:line` citation for every task so the AI's choices stay
auditable.

## Keeping people informed — without a UI

Two jobs a tracker normally does with an app, Roady does with generated
artifacts and push notifications.

**Coordination — who is on what:**

```bash
roady task assign <task-id> alice
roady task mine                   # your tasks (ROADY_USER, git user.name, or USER)
roady task assigned alice         # someone else's
roady task unassigned             # work nobody owns
```

Add guardrails in `.roady/policy.yaml`:

```yaml
max_wip_per_owner: 2       # cap in-progress work per person, not just per project
enforce_team_roles: true   # a viewer in team.yaml can no longer move tasks
```

**Stakeholder reporting — a document, not a dashboard:**

```bash
roady report                                # Markdown to stdout
roady report --since 7d                     # just this week's changes
roady report --format html -o status.html   # ~5KB, no scripts, no requests
roady report --format json | jq .risks      # machine-readable
```

The report carries progress, a forecast with its confidence interval, a risk
register built from drift plus sticky debt, who is on what, and what changed.
Commit it, email it, attach it to a PR, or publish it to a static host —
nothing to install and nothing to log into. A completion estimate is withheld
until there is enough velocity data to justify one.

**Push it on a schedule:**

```bash
roady notify add team-chat slack https://hooks.slack.com/services/...
roady notify digest --since 7d --dry-run    # preview
roady notify digest --since 7d              # send
```

One chat-sized summary instead of a message per task transition. Run it from
cron or CI.

**Audit — proving what happened:**

```bash
roady audit trail task-42                          # evidence trail for one task
roady audit trail --agent claude-code --since 30d  # everything one agent did
roady audit trail --session <id>                   # everything one run did
```

Every event records the agent and session behind it, so "which agent worked on
this, and what proves it?" has an answer. A trail reports hash-chain integrity,
findings (a task marked done with no evidence, entries with no agent recorded),
the task's `doc:line` citation back to the spec, and every recorded event. It
exits non-zero when the chain fails verification, so it can gate CI.

Roady attests to **a complete, tamper-evident record of what was asserted** —
not to who acted, since actor and agent are caller-supplied and never
authenticated. See [`docs/audit-grc.md`](docs/audit-grc.md) before quoting a
trail to an auditor.

## Nested sub-projects

One repository can host many Roady projects in parallel:

```
repo/
  .roady/                          # root project
  .roady/projects/feature-auth/    # named sub-project
  .roady/projects/feature-payments/
```

```bash
roady -P feature-auth init --template minimal
roady -P feature-auth task ready
ROADY_PROJECT=feature-auth roady status
```

Tasks, spec, plan, and state are namespaced per project. Coding agents
switch context by passing `--project / -P <name>` (CLI) or `project`
(MCP). Existing flat `.roady/` repos stay unchanged. See
[`docs/rfcs/0001-nested-projects.md`](docs/rfcs/0001-nested-projects.md).

## What Roady is, and is not

| Roady is... | Roady is not... |
| --- | --- |
| The plan-of-record for an AI-paired feature | A Jira / Linear replacement |
| Memory that survives `/clear` and session resets | A chat history layer |
| File-based, git-friendly, local-first | A hosted SaaS (today) |
| MCP-native — every operation is a tool | A code-search or context-stuffing tool |

See [`docs/positioning.md`](docs/positioning.md) for the full positioning,
ICP, and category claim.

## How it compares

[`docs/vs.md`](docs/vs.md) — opinionated comparison vs Cursor rules,
Claude.md, spec-kit, Backlog.md, Linear, GitHub Projects.

## Everything else

The headline workflow is intentionally short. Roady supports billing
rates, debt scoring, dependency graphs, cross-project org views,
plugin syncers, fsnotify watch mode, an interactive TUI (`roady
dashboard`), inline MCP App UIs rendered by your agent, webhook +
Slack notifications, and more — see
[`docs/advanced.md`](docs/advanced.md) for the full catalogue grouped by
audience (solo dev / small team / org).

## Roadmap

[`ROADMAP.md`](ROADMAP.md) sketches what's next, including the planned
**Roady Cloud** open-core boundary (hosted MCP, multi-repo org
dashboard, audit retention, SOC2).

## Contributing & license

Contributions welcome — open an issue or PR. MIT License, see `LICENSE`.

Maintainers: see [`docs/maintainer-setup.md`](docs/maintainer-setup.md)
for the one-time GitHub repo settings the release pipeline depends on
(`HOMEBREW_TAP_TOKEN` secret, GitHub Pages source).

---

*Built with `cobra`, `bubbletea`, `mcp-go`, `fortify`. Domain-driven Go
with `pkg/domain` / `pkg/application` / `internal/infrastructure`.
Architecture notes in the DDD docs ([`docs/ddd-insights.md`](docs/ddd-insights.md),
[`docs/ddd-refactor-spec.md`](docs/ddd-refactor-spec.md)).*
