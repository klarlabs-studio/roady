# Capture

`roady capture` (MCP: `roady_capture`) records intent at whatever size it
arrives — one task, a few, or a whole plan with the features and requirements
behind it — in a single write.

It exists because writing to roady has to cost less than writing `plan.md`.
Recording a plan used to take a round trip — `spec_add`, `plan_generate --ai`,
run the prompt, `plan_update`, approve — and agents took the cheaper path.

## The document

```yaml
features:
  - id: invoices
    title: Invoice generation
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
  `feature_id`, and a `title` when new.
- **All or nothing.** The resulting spec and plan are validated together —
  spec rules, dependencies that exist, no cycles, valid priorities and checks.
  If anything fails, nothing is written, and every rejection is reported with
  its item and reason. The CLI exits non-zero.
- **Idempotent.** Sending the same document again writes nothing, records no
  event, and leaves the plan's approval as it was.
- **Unknown keys are refused**, so a misspelt field is an error rather than
  silently dropped.

The result names every item as `feature:`, `requirement:` or `task:` with what
happened to it, and the plan's approval afterwards.

## Approval: which changes need it

An approved plan stays approved when only **tasks** change — adding, splitting
or editing work within requirements that were already approved does not change
what was agreed. A created or edited **feature or requirement**, including its
acceptance check, changes the intent and returns the plan to pending; the
result names the item that did it.

```yaml
# .roady/policy.yaml
plan_approval: scope         # default
# plan_approval: every_change  # any change needs re-approval, as before
```

`roady plan generate` and `roady_plan_update` follow the same rule: the
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
  task. `--after` tasks become its dependencies, `--before` tasks come to
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
