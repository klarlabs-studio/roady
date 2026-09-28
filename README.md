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
brew trust klarlabs-studio/tap        # first time only
brew install --cask klarlabs-studio/tap/roady     # or: go install github.com/felixgeelhaar/roady/cmd/roady@latest
cd your-project && roady init my-project && roady setup claude-code
```

Homebrew refuses to load a cask from a third-party tap it has not been told
to trust, so the first install of anything from this tap needs
`brew trust klarlabs-studio/tap` once — per machine, not per tool.

`roady setup` registers roady with your agent (MCP server, hooks,
instructions, skill). Zero AI keys, zero signup: roady runs no inference.

## The actual workflow

```bash
# 1. Hook your agent to Roady (one command per supported tool)
roady setup claude-code           # or codex, gemini, cursor, opencode, copilot, kiro, all

# 2. Initialise + import your existing docs
roady init my-project
roady spec analyze docs/          # parses markdown, captures source citations

# 3. Generate a plan (deterministic by default; --ai emits a prompt for
#    your own model to run, then write the tasks back)
roady plan generate
roady plan approve
#    ...or record a plan your agent already made, in one write:
roady capture -f plan.yaml        # see docs/capture.md
roady plan import PLANS.md        # a Claude Code / Kiro / ExecPlan plan → tasks
roady add "Handle empty file" --after task-pdf-gen   # or edit / split / move

# 4. Drive execution from inside your AI editor
roady next                        # the task, why it exists, what done means
# ...agent implements, commits with [roady:task-id] marker...
roady git sync                    # state moves forward automatically
roady git suggest                 # commits made without a marker, and their likely task
roady git link <commit> <task-id> # ...record one as that task's evidence
roady task check <task-id>        # done means the acceptance check passes

# 5. Ask the question that matters
roady drift detect                # has reality diverged from intent?
```

Status, drift, and progress all show in `roady status` — including a
`from doc:line` citation for every task so the AI's choices stay
auditable.

## Who is on what

```bash
roady task mine                   # your tasks (ROADY_USER, git user.name, or USER)
roady task unassigned             # work nobody has started
```

`max_wip_per_owner` in `.roady/policy.yaml` caps in-progress work per person
or agent, not just per project. Starting a task claims it: two agents
starting the same task at once cannot both get it, and a claim nobody renews
(`roady next` and the agent hooks keep it alive while you work) lapses after `claim_lease`
(default `2h`) and frees the task, so a crashed agent does not hold it.

**Prove "done" instead of claiming it:**

```yaml
# .roady/spec.yaml — on a requirement
check:
  run: go test ./internal/invoice -run TestNumberingIsGapFree
```

```bash
roady task check task-seq-numbers    # run it, record pass/fail as evidence
roady task verify task-seq-numbers   # re-runs the check; refuses if it fails
```

A check can also be `manual:` — then only a person's `--confirm` satisfies it.
See [`docs/acceptance-checks.md`](docs/acceptance-checks.md).

**Gate CI on drift:**

```bash
roady drift detect --fail-on high    # exits non-zero only for high + critical
```

Everything found is still printed; the threshold changes only the exit code.
See [`docs/spec-to-pr.md`](docs/spec-to-pr.md) for pull-request gating and
opening follow-up issues after merge.

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

## Roady runs no inference

Roady does not call language models. It assembles the context one needs and
hands it back — you or your agent already has a model:

```bash
roady query "what is left to do?"        # prompt on stdout, pipeable
roady plan generate --ai --json          # the request as JSON for an agent
```

Requests that produce data Roady stores name the tool that accepts it
(`decompose_spec` → `roady capture`). No API key is needed for anything.
See [`docs/prompts.md`](docs/prompts.md).

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
| The plan-of-record for an AI-paired feature | A feature-for-feature Jira / Linear clone |
| Memory that survives `/clear` and session resets | A chat history layer |
| File-based, git-friendly, local-first | A hosted SaaS (today) |
| MCP-native — every operation is a tool | A code-search or context-stuffing tool |

See [`docs/positioning.md`](docs/positioning.md) for the full positioning,
ICP, and category claim.

## How it compares

[`docs/vs.md`](docs/vs.md) — opinionated comparison vs Cursor rules,
Claude.md, spec-kit, Backlog.md, Linear, GitHub Projects.

## Everything else

The headline workflow is intentionally short. Drift explanation, semantic
drift, spec review, subagent dispatch, audit trails for review, nested
sub-projects and more are in [`docs/advanced.md`](docs/advanced.md).
Roady deliberately has no billing, team roster, tracker sync, chat
notifications or dashboards: it is the plan an agent works from, and the
proof it did.

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

*Built with `cobra`, `mcp-go`, `statekit`, `fortify`. Domain-driven Go
with `pkg/domain` / `pkg/application` / `internal/infrastructure`.
Architecture notes in the DDD docs ([`docs/ddd-insights.md`](docs/ddd-insights.md),
[`docs/ddd-refactor-spec.md`](docs/ddd-refactor-spec.md)).*
