# Importing a harness's plan

Agent harnesses already plan, in markdown: Claude Code's plan mode, Cursor,
Gemini CLI, Devin, a Codex ExecPlan (`PLANS.md`), Kiro's
`.kiro/specs/<name>/tasks.md`. Those files are written once and drift out of
date the moment work starts. `roady plan import` (MCP: `roady_plan` with action `import`)
turns one into roady tasks, where progress is tracked, checked and audited.

```bash
roady plan import ~/.claude/plans/rate-limits.md --dry-run
roady plan import .kiro/specs/user-auth/tasks.md
roady plan import PLANS.md --feature offline --parallel
```

## Formats

The format is detected from the path and content, or forced with `--format`.

| Format | Recognised by | Steps are |
|---|---|---|
| `kiro` | a `.kiro/specs/*/tasks.md` path, or two or more numbered checkbox items | the checkbox tasks; `2.1` sub-tasks become dependencies of `2`; `_Requirements: …_` lines go into the description |
| `execplan` | `## Progress` plus a Decision Log / Surprises & Discoveries / Outcomes section | the Progress checklist (timestamps stripped), or Concrete Steps / Plan of Work before any progress is recorded |
| `markdown` | anything else | list items under a Steps / Implementation / Tasks / Plan heading; else list items outside narrative sections (Context, Risks, Verification, …); else one step per sub-heading |

Code fences are ignored. Nested lines fold into the step above as its
description; a bold lead (`**Add a token bucket** in bucket.go`) is the title.

## What an import produces

- **A task per step**, id `task-<plan>-<step>`, citing the plan file and line
  as its source (project-relative when the plan is inside the project).
  `roady next` shows that citation.
- **Order as dependencies.** Each step depends on the one before it, since a
  plan is written in the order it runs. `--parallel` leaves them independent.
- **Checked-off steps are skipped.** A finished step is history, and roady does
  not mark work done without its own evidence. `--include-done` imports them
  as pending work.
- **A feature for the plan**, named after its title — new intent, so the plan
  goes back to pending approval. `--feature <id>` attaches the tasks to a
  feature that already exists instead; under the default
  `plan_approval: scope` adding tasks there keeps an approved plan approved.

The import is a [capture](capture.md): validated all or nothing, and
idempotent. Edit the plan and import it again, and the same tasks are updated
rather than duplicated — as long as a step's wording (and so its id) stays the
same. A renamed step becomes a new task; the old one stays until removed.

Acceptance checks are not inferred from the plan. Add them with
`roady capture` (see [acceptance-checks.md](acceptance-checks.md)).
