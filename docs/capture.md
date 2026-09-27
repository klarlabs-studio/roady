# Capture

`roady capture` (MCP: `roady_capture`) records intent at whatever size it
arrives — one task, a few, or a whole plan with the features and requirements
behind it — in a single write.

It exists because writing to roady has to cost less than writing `plan.md`.
Recording a plan used to take a round trip — `spec_add`, `plan_generate --ai`,
run the prompt, `plan_update`, approve — and agents took the cheaper path.

## The document

```yaml
goals:
  - id: goal-billing
    title: Customers can be billed
    horizon: now            # now | next | later
    milestone: v1.0
features:
  - id: invoices
    title: Invoice generation
    goal: goal-billing
    requirements:
      - id: seq-numbers
        title: Sequential gap-free invoice numbers per year
        priority: high
        check:
          run: go test ./internal/invoice -run TestNumbering
      - id: pdf
        title: Generate PDF from YAML line items
        depends_on: [seq-numbers]
tasks:
  - id: task-migrate
    title: Migrate existing invoice numbers
    requirement: seq-numbers
    depends_on: [task-seq-numbers]
```

```bash
roady capture -f plan.yaml      # or pipe it on stdin; JSON works too
roady capture --dry-run < plan.yaml
```

## Rules

- **Upsert by id.** An item that exists is updated; one that does not is
  created. Fields left out keep their current value, so the same shape edits:
  `tasks: [{id: task-pdf, priority: high}]` changes one field.
- **Requirements bring their task.** Each requirement has a task
  `task-<requirement id>`, created and kept in step with it (title, priority,
  estimate, check). A requirement's `depends_on` becomes the task's
  dependencies only when the capture states it, so edges set on a task
  directly survive an unrelated edit to its requirement.
- **Extra tasks** name a `requirement` (which implies the feature) or a
  `feature_id`, and a `title` when new. A task with neither is **unplanned
  work** — a chore, a quick fix — kept in the inbox, or under a roadmap
  `goal`. Drift notes it as `UNPLANNED` (severity `info`, below every gate)
  instead of an orphan, and `plan prune` keeps it.
- **All or nothing.** The resulting spec and plan are validated together —
  spec rules, dependencies that exist, no cycles, valid priorities and checks.
  If anything fails, nothing is written, and every rejection is reported with
  its item and reason. The CLI exits non-zero.
- **Idempotent.** Sending the same document again writes nothing, records no
  event, and leaves the plan's approval as it was.
- **Unknown keys are refused**, so a misspelt field is an error rather than
  silently dropped.

The result names every item as `goal:`, `feature:`, `requirement:` or `task:`
with what happened to it, and the plan's approval afterwards. A feature or
requirement linked to a different goal shows as `link:feature:<id>`.

## Goals: the roadmap

Goals hold what a ROADMAP.md would: outcomes on the `now`, `next` or `later`
horizon with an optional `milestone`, and a `status` — `idea` (needs no
features yet), `planned` (the default), `shipped` or `out_of_scope` (a
deliberate no, kept so it is not proposed again). Features link to the goal
they serve with `goal`; a requirement can link to a different one.

```bash
roady goal add "Offline mode" --horizon next --milestone v2.0
roady goal edit goal-offline-mode --horizon now --feature sync
roady goal edit goal-offline-mode --status shipped
roady goal list        # now / next / later / ideas / shipped / out of scope, with progress
roady goal render      # write ROADMAP.md from the goals
```

`roady goal render` writes ROADMAP.md at the repository root. Its first line
marks it as generated and carries a hash of the rest, so `roady drift detect`
reports an edit made to the file instead of to the goals (and a file the
goals have moved past). Render will not replace a hand-edited file, or one
roady did not write, without `--force` — move what it says into goals first.
`roady goal render --check` fails unless the file is up to date, for CI.
A project that keeps its roadmap elsewhere sets `roadmap: memory/roadmap.md`
(relative to the repository root) in `.roady/policy.yaml`: render writes
there, and drift checks that file instead of ROADMAP.md.
Task progress is left out of the file, so it changes only when the roadmap
does.

A roadmap already kept by hand moves in with `roady goal import <file>`
(MCP: `roady_goal` action `import`). It reads `## Now`, `Next`, `Later`,
`Ideas`, `Done`/`Shipped` and `Out of scope` sections; each `###` heading or
top-level bullet under one is a goal — a bold lead, or the text before a
dash, is its title, the rest its description, a trailing `(v1.2)` its
milestone. Other sections are skipped and named. Ids are `goal-<title>`, so
a second import changes nothing. Then render the file from the goals (with
`--force`, since roady did not write it) or delete it.

Planning kept in prose — a decisions log, open threads, a status page — is
for a model to read, and roady runs none. `roady capture --from-notes
<file|dir>` (MCP: `roady_capture` with `from_notes`) prints a prompt holding
those notes, what the project already records (so ids are reused, not
duplicated) and the capture format. Your model answers with the document;
capture it, `--dry-run` first. Each file is capped at 40 KB and the prompt
at 120 KB; a cut file is marked truncated.

Goals order work; they are not part of the intent a plan is approved for.
Adding or moving a goal, or linking a feature to one, never returns an
approved plan to pending, and does not show as spec drift.

## History

Every capture — and so every `add`, `edit`, `split`, `move`, `goal` and
`decide` — appends a `plan.capture` event recording the fields it changed on
each item (from → to), the full shape of anything it created, and what it
was for ("Split task-x into 2 parts"). Transitions, checks, blocks and
expired claims have events of their own. `roady task history <id>` (MCP
`roady_task` action `history`) reads a task's story back:

```text
History of task-gen
2026-09-27 15:16  felix   created: "Generate a PDF", feature pdf
2026-09-27 15:16  felix   edited: estimate (none) → "2d"
2026-09-27 15:16  felix   edited: depends on (none) → [task-gen-lay-out, task-gen-embed-fonts] (Split task-gen into 2 parts)
2026-09-27 15:18  codex   start
2026-09-27 15:40  codex   block (cannot-complete): no font license
```

plan.json holds only the latest shape; the history is never rewritten.

## Decisions

A decision records a choice — `title`, `choice`, `context`,
`consequences`, `date` — linked to the `goals`, `features` and
`requirements` it constrains (none: project-wide). `supersedes: <id>` marks
an earlier one as replaced. `roady next` shows the standing decisions behind
the task you are on, newest first, so an agent does not reopen or silently
reverse them.

```bash
roady decide "Session storage" --choice "Signed cookies" --context "Stateless deploys" --req jwt
roady decide --list
```

Decisions, like goals, never reopen the plan's approval.

## Approval: which changes need it

An approved plan stays approved when only **tasks** (or goals) change — adding, splitting
or editing work within requirements that were already approved does not change
what was agreed. A created or edited **feature or requirement**, including its
acceptance check, changes the intent and returns the plan to pending; the
result names the item that did it.

```yaml
# .roady/policy.yaml
plan_approval: scope         # default
# plan_approval: every_change  # any change needs re-approval, as before
```

`roady plan generate` follows the same rule: the
approval stands when the spec still matches its lock and no task was dropped.

## One task at a time

For a single change, four commands build the capture for you. Each is a
capture underneath — validated the same way, all or nothing, and recorded as
one `plan.capture` event.

```bash
roady add "Handle an empty input file" --req pdf-gen --after task-pdf-gen
roady add "Load test it" --after task-rate-limits --before task-release -p high
roady edit task-pdf-gen --estimate 2d --add-dep task-fonts --drop-dep task-old
roady split task-pdf-gen "Lay out the page" "Embed fonts" "Write the file"
roady move task-load-test --req perf-budget
```

- **add** — the task belongs to `--req` (implying its feature) or
  `--feature`; with neither, it joins the feature of the first `--after`
  task, and with none of those it is unplanned work (`--goal` files it under
  a roadmap goal). `--after` tasks become its dependencies, `--before` tasks come to
  depend on it. Its id is `task-<title>` (or `--id`); adding the same title
  again changes nothing. `--check-run` / `--check-manual` give it a check.
- **edit** — changes only the fields given. `--depends-on` replaces the
  list, `--add-dep` / `--drop-dep` adjust it. Changing the check of started
  work needs `--change-checks`.
- **split** — each part becomes `task-<id>-<part>` in the same feature and
  takes over what the task waited for; the task now waits for its parts, so
  its dependents and its check stay where they were. `--sequential` chains
  the parts.
- **move** — re-homes a task under another requirement or feature.

All four take `--dry-run` and `--json`. They only touch tasks, so under
`plan_approval: scope` an approved plan stays approved.
