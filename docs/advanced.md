# Roady — everything else

The README shows the core loop on purpose: capture intent, keep the agent on
it, prove the work done. This page lists what else Roady does in service of
that loop. Features outside it — billing, team rosters, org rollups, tracker
sync, chat notifications, debt scoring, forecasting, dashboards — were
removed; git history has them.

---

## Capturing intent

- `roady spec analyze docs/` walks a directory of markdown and emits a
  structured spec with source citations (`from docs/auth.md:14`). Every task
  downstream carries the citation.
- `roady capture` records features, requirements and tasks in one write, from
  a single task to a whole plan ([capture.md](capture.md)). `roady add`,
  `edit`, `split` and `move` do the same for one task at a time.
- `roady plan import <file>` turns a plan an agent already wrote — Claude
  Code plan mode, Kiro `tasks.md`, a Codex ExecPlan, any markdown plan — into
  tasks ([plan-import.md](plan-import.md)).
- `roady plan generate --ai`, `roady plan smart-decompose`, `roady plan
  prioritize`, `roady spec explain`, `roady spec review` and `roady query`
  build prompts for your own model; roady runs no inference
  ([prompts.md](prompts.md)). Tasks written back that way are tagged
  `Origin=ai`.

## Keeping the agent on the plan

- `roady next` — the current task, why it exists, what done means, what it
  unblocks. `roady setup <agent>` injects it at session start (and after
  compaction where the agent allows), imports approved plan-mode plans, and
  refuses ROADMAP/TODO/plan markdown files ([mcp-integration.md](mcp-integration.md)).
- `roady task dispatch` hands a ready task to a subagent with its intent and
  completion contract.
- Task owners and `max_wip_per_owner` keep parallel agents off each other's
  work; `roady task mine` lists yours.
- Nested sub-projects (`--project` / `project`) keep separate plans in one
  repository ([rfcs/0001-nested-projects.md](rfcs/0001-nested-projects.md)).

## Proving it done

- Acceptance checks on requirements and tasks; `roady task check` runs them
  and records the result; `verify` re-runs them and refuses on failure
  ([acceptance-checks.md](acceptance-checks.md)).
- `verify_requires_evidence` in `policy.yaml`: a task is verified only with a
  passing check and a linked commit, unless a person overrides it on the
  record.
- `roady drift detect` / `explain` / `accept` — intent, plan, code and policy
  drift; `roady drift semantic` builds the prompt for judging whether an
  implementation still means what its requirement says.
- Hash-chained `events.jsonl`; `roady audit verify` checks the chain against
  the committed baseline, and `roady audit trail` produces the evidence for a
  task, agent or session ([audit-grc.md](audit-grc.md)).

## Setup and maintenance

- `roady completion bash|zsh|fish|powershell`
- `roady config wizard` — interactive `policy.yaml` setup
- `roady doctor` — health check of the project files and audit chain
- `roady state rebuild` — recover state from the event log

## MCP

```bash
roady mcp                               # stdio (default)
roady mcp --transport http --addr :8080
roady mcp --transport ws   --addr :8080
```

One tool per CLI noun, the verbs as actions, so an agent can do anything the
CLI can. Decisions — plan approve/reject/prune, drift accept, spec
lock/import/analyze, state rebuild — run only after the user confirms them in
their client. See [mcp-guide.md](mcp-guide.md).

## Architecture

- `pkg/domain/` — pure business logic, no I/O.
- `pkg/application/` — use-case services.
- `internal/infrastructure/` — CLI, MCP server, hooks, wiring.
- `pkg/storage/` — file repository over `.roady/`.
- `evals/` — regression corpus over the planning pipeline.

Stack: `cobra`, `mcp-go`, `statekit`, `fortify`. See `docs/ddd-*.md` for the
architecture write-up.
