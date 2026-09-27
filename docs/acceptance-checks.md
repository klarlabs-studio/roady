# Acceptance checks

A task is done when something other than the claim says so. A **check** on a
requirement states how that is shown; roady runs it, records the result as
evidence, and refuses to verify a task whose check does not pass.

This exists because "done" used to be whatever the caller reported. Agents
report work as finished when it is not — premature completion is the failure
long-running agent harnesses are built around — and roady stored the claim as
fact.

## Defining a check

Add `check` to a requirement in `.roady/spec.yaml`. It is one of:

```yaml
requirements:
  - id: seq-numbers
    title: Sequential gap-free invoice numbers per year
    check:
      run: go test ./internal/invoice -run TestNumberingIsGapFree
  - id: pdf-layout
    title: Invoice PDF layout
    check:
      manual: Open a generated invoice and confirm the layout matches the template
```

- **`run`** — a command executed with `sh -c` in the project root. Exit status 0
  passes. Keep it focused: the test that proves this requirement, not the whole
  suite.
- **`manual`** — a check a person performs. Only a person's confirmation
  satisfies it; an agent cannot, over MCP or otherwise.

`roady spec validate` rejects a check that sets both or neither. `roady plan
generate` copies the check onto the requirement's task.

A check is part of the intent, so it is part of the spec hash: changing or
loosening one shows up as spec drift against the lock.

## Running a check

```bash
roady task check task-seq-numbers            # run it and record the result
roady task check task-pdf-layout --confirm   # confirm a manual check
```

Every run is recorded on the task in `state.json` — kind, command, pass or fail,
exit code, the commit it ran at (and whether there were uncommitted changes
outside `.roady/`), who ran it, and the tail of its output — and in the audit
log as `task.check`, which `roady audit trail` shows as passed, failed, or
confirmed. A failing check exits non-zero but is a recorded result, not an
error.

Over MCP: `roady_task` with action `check` runs a `run` check. It cannot confirm a manual one.

## Verification is gated on the check

`roady task verify <id>` (and `roady_task` with action `verify`) runs
a `run` check against the code as it is now and refuses to verify if it fails.
It does not trust an earlier pass: verification is a claim about the current
state. A `manual` check needs a confirmation already recorded with
`roady task check <id> --confirm`.

## Catching regressions: `roady drift detect --checks`

Verification proves a task at the commit it was verified at; the code keeps
moving. `roady drift detect --checks` re-runs the `run` check of every
verified task against the code as it is now. Each run is recorded in the
task's check history, and a check that fails is reported as `REGRESSION`
drift (severity high) naming the task, the command, the commit it failed at
and the commit it last passed at — and logged as `task.regression`. The
task's status is left alone: fix the code, or `roady task reopen` it if the
requirement changed. Manual checks are skipped; only a person can confirm
them.

```bash
roady drift detect --checks --fail-on high   # in CI: fail on a regression
```

Over MCP: `roady_drift` with action `detect` and `checks: true`.

## Requiring evidence: `verify_requires_evidence`

With this policy on, **verified means proven**. A task can be verified only
when it has

1. an acceptance check that passes now, and
2. a linked commit — recorded by `roady git sync` from a `[roady:<task-id>]`
   marker, or passed with `roady task complete <id> --evidence <sha>`.

```yaml
# .roady/policy.yaml
verify_requires_evidence: true
```

`roady init` turns it on for new projects. Existing projects keep verifying the
old way until they add the line, so an upgrade never refuses work that was
already accepted.

Missing evidence is reported before the check runs, so a refusal does not cost
a test suite. `roady git sync` links a marked commit to a task that was
completed before the commit existed, rather than skipping it.

### Overrides

A person can verify without the evidence, on the record:

```bash
roady task verify task-b --override "reviewed by hand; no automated test possible"
```

The audit log records `override: true` and the reason, so a trail shows the
task was accepted on judgement rather than evidence. An override lifts only
*missing* evidence — never a check that runs and fails, since that has just
shown the task is not done. Overrides are CLI-only; there is no MCP equivalent.

Without the policy, a task that has a check is still gated on it; a task
without one verifies as before.

## Changing a check after work started

Once a task is in progress, blocked, done or verified, removing or changing
its check is refused — whichever way it is attempted:

- editing the task's check (`roady capture`, `roady edit`, `roady_capture`),
- editing the requirement's check (`roady capture`, `roady plan generate`),
- editing `spec.yaml` by hand and then re-locking it (`roady spec lock`,
  `roady drift accept`, `roady spec add`), which is how a loosened check
  would otherwise stop being reported as drift.

Adding a check is always allowed; it only tightens. So is any change to a task
that has not started — that is still planning. Roady cannot tell whether a new
command is stricter than the old one, so every modification counts.

A person can make the change on the record:

```bash
roady drift accept --change-checks       # likewise: capture, spec lock, plan generate
```

Done or verified tasks whose check changed are reopened, and each change is
logged as `task.check_changed` with the old and new check. There is no MCP
equivalent, and nothing accepts such a change automatically.

**What this does not do.** Roady does not authenticate people: an agent
running the CLI in a shell can pass `--change-checks` too. The guarantee is
that loosening a check is never silent — it takes an explicit flag, reopens
the work, and leaves an entry in the audit trail that a reviewer will see.
