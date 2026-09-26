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
