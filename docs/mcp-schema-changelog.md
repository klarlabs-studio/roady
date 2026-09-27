# MCP Schema Changelog

## Schema Evolution Rules

- **Patch** (1.0.x): Description fixes only
- **Minor** (1.x.0): New optional fields (`omitempty`), new tools, fields deprecated
- **Major** (x.0.0): Required fields added/removed, tool signatures changed

## v5.0.0 — One tool per CLI noun

**Major**: per-verb tools replaced by noun tools with an `action`.

### Tools

`roady_task`, `roady_plan`, `roady_spec`, `roady_drift`, `roady_audit`,
`roady_state`, `roady_policy`, `roady_git`, `roady_goal` take a required `action` — the CLI
verb — plus that verb's arguments. `roady_next`, `roady_status`,
`roady_query`, `roady_capture` and `roady_init` keep their shape. Fourteen
tools in all; every CLI project command has a tool or action.

| Tool | Actions |
|------|---------|
| `roady_task` | start, complete, block, unblock, stop, reopen, verify, check, dispatch, list, renew |
| `roady_plan` | get, generate, approve, reject, prune, prioritize, decompose, import |
| `roady_spec` | get, add, analyze, explain, import, lock, review, validate |
| `roady_drift` | detect, accept, explain, semantic, record |
| `roady_audit` | verify, trail |
| `roady_state` | get, rebuild |
| `roady_policy` | check |
| `roady_git` | sync |
| `roady_goal` | list, add, edit, render (`force` asks the user) |

### Removed

Every per-verb tool: `roady_task_transition`, `roady_task_check`,
`roady_task_dispatch`, `roady_tasks`, `roady_plan_*`, `roady_spec_*`,
`roady_drift_detect`, `roady_drift_accept`, `roady_drift_explain`,
`roady_drift_record_semantic`, `roady_semantic_drift`, `roady_state_*`,
`roady_snapshot_get` (now `roady_status` with `snapshot: true`),
`roady_audit_*`, `roady_policy_check`, `roady_git_sync`.

### Changed

- Decisions — `plan` approve/reject/prune, `drift` accept, `spec`
  analyze/import/lock, `state` rebuild — ask the user through MCP
  elicitation and run only on an explicit yes. A decline, a cancel or a
  client without elicitation returns an error result naming the CLI command;
  nothing changes. A confirmation is logged as `approval.confirmed`.
- `roady_task` takes `event` as `action` and `agent` in place of `actor`.
  `start` claims the task with a lease (`agent` and `session_id` identify
  the holder); `renew` extends it; another agent starting a claimed task is
  refused with the holder and expiry. The brief from `roady_next` carries
  `claim`.
- `roady_drift` `record` takes `judgements` only (roady rebuilds the
  questions, so an invented requirement id is refused).
- Prompt requests name the noun tool in `write_back`: `roady_capture` for
  `decompose_spec`, `roady_drift` for `semantic_drift`. A dispatch brief's
  completion contract names `roady_task` with `action: complete`.
- No tool carries a `ui://roady/*` resource; the MCP App UIs are removed.
- `ROADY_MCP_TOOLS` and its groups are gone; every tool is listed.
- `roady_capture` takes `goals` (id, title, description, horizon, status,
  milestone), and features and requirements take `goal`.

## v4.0.0 — Narrowed to capture, keep, prove

**Major**: tools removed.

### Removed

`roady_cost_budget`, `roady_cost_report`, `roady_rate_add`, `roady_rate_list`,
`roady_rate_remove`, `roady_rate_set_default`, `roady_rate_tax`,
`roady_usage_get`, `roady_task_log_time`, `roady_forecast`, `roady_team_add`,
`roady_team_list`, `roady_team_remove`, `roady_task_assign`,
`roady_org_status`, `roady_org_members`, `roady_org_policy`,
`roady_org_detect_drift`, `roady_workspace_push`, `roady_workspace_pull`,
`roady_sync`, `roady_plugin_list`, `roady_plugin_status`,
`roady_plugin_validate`, `roady_messaging_list`, `roady_debt_report`,
`roady_debt_summary`, `roady_debt_score`, `roady_debt_trend`,
`roady_debt_history`, `roady_drift_recurring`, `roady_deps_graph`,
`roady_deps_list`, `roady_deps_scan`, `roady_report`, `roady_timeline`.

`ROADY_MCP_TOOLS` groups `cost`, `team`, `org`, `debt`, `deps`, `plugin` and
`sync` are gone (`roady_git_sync` moved to `core`); an unknown group still
fails startup.

## v3.8.0 — Essential surface by default

**Minor**: no tool removed or changed; what `tools/list` shows by default is.

### Changed

- With `ROADY_MCP_TOOLS` unset, `tools/list` returns seven tools —
  `roady_next`, `roady_capture`, `roady_plan_import`, `roady_task_transition`,
  `roady_task_check`, `roady_status`, `roady_query` — and every other tool
  stays callable by name. `ROADY_MCP_TOOLS=all` lists everything, as before;
  `essential,<group>` adds groups to the list.
- Shorter descriptions for `project_path` / `project` on every tool, and for
  `roady_plan_import`, `roady_task_check` and `roady_task_transition`'s
  `actor`, `agent` and `session_id`.

## v3.7.0 — Plan import and task brief

**Minor**: new tools, no existing signature changed.

### Added

- `roady_plan_import` — imports a plan file (`path`; optional `format`,
  `feature_id`, `parallel`, `include_done`, `dry_run`) as a capture. Returns
  `format`, `title`, `steps`, `skipped_done` and the capture `result`.
- `roady_next` — returns `brief` (rendered text for context injection) and
  `detail` (the structured brief).

## v3.6.0 — Audit baseline and acceptance checks

**Minor**: new optional fields and a new tool, no existing signature changed.

### Added

- `roady_capture` — records features, requirements and tasks in one call,
  upserted by id; all or nothing, idempotent, with `dry_run`. Returns
  `created`, `updated`, `unchanged`, `rejected` (item and reason), `applied`
  and the plan's approval afterwards.
- `roady_task_check` — runs a task's acceptance check and returns the recorded
  result (`kind`, `command`, `passed`, `exit_code`, `commit`, `dirty`, `by`,
  `at`, `duration`, `output`). A failing check is a result, not an error.
  Manual checks cannot be confirmed over MCP.
- Tasks in `roady_plan_get` / `roady_plan_update` carry an optional `check`
  (`run` or `manual`). `roady_task_transition` with `event: verify` re-runs a
  task's check and returns an error result when it fails.

- `roady_audit_verify` takes an optional `baseline` (a git revision, default
  `HEAD`) and additionally reports entries of the log committed at that
  revision that are missing now. The result gains `baseline`: the revision,
  whether it was checked, and why not when it was not.

## v3.2.0 — Subagent dispatch

**Minor**: a new tool, no existing signature changed.

### Added

- `roady_task_dispatch` — hands a ready task to a subagent with the context it
  would otherwise have to reconstruct: the originating feature and
  requirement, the `doc:line` citation that motivated the task, what counts as
  done, and the exact call that records completion **against that agent**.

  Takes `task_id`, `agent`, an optional `session_id`, and `dry_run`. Claims the
  task unless `dry_run`. Only ready tasks are dispatchable — handing out work
  whose prerequisites are unmet produces an agent that blocks, or one that
  implements against something that does not exist yet.

  Not read-only: it claims the task by default. Reversible with `stop`.

55 tools in this version.

## v3.1.0 — Audit trails over MCP

**Minor** per the rules above: a new tool, no existing signature changed.

### Added

- `roady_audit_trail` — the evidence trail was CLI-only, which left the
  agents the feature was built for unable to ask "which agent worked on this,
  and what proves it". Takes `task_id`, `agent`, or `session_id` (combinable),
  plus an optional `since` window. Returns chain-integrity status, findings,
  the task's evidence and `doc:line` spec citation, who acted, and every
  recorded event.

  Read-only, idempotent, closed-world. It attests to a tamper-evident record
  of what was *asserted*, not to who acted — actor and agent are
  caller-supplied and unauthenticated. See `docs/audit-grc.md`.

54 tools in this version.

## v3.0.0 — No inference, and errors that reach the agent

**Major** per the rules above: a tool was removed and six changed their
response shape.

### Removed (breaking)

| Removed | Why |
| --- | --- |
| `roady_cost_estimate` | Roady no longer calls a model, so it cannot project a token bill for one. |

### Changed (breaking)

Six tools returned model output. Roady no longer runs inference — the caller
already has a model — so they now return the assembled request instead:

`roady_plan_decompose`, `roady_spec_explain`, `roady_spec_review`,
`roady_query`, `roady_plan_prioritize`, `roady_drift_explain`

```json
{
  "operation": "decompose_spec",
  "system": "...",
  "prompt": "...",
  "expected_format": "{\"tasks\": [...]}",
  "write_back": "roady_plan_update",
  "guidance": "Produce the tasks yourself, then call roady_plan_update."
}
```

`write_back` names the tool that accepts the result. These tools now work
with no configuration at all — previously they failed without a provider.

### Changed — tool errors are results, not protocol faults

Every tool reported failure as a JSON-RPC error, which clients surface as
`-32603 "internal error"` with Roady's actual message discarded. Failures are
now returned as a normal result with `isError: true` and the message as
readable content, per the MCP spec. Protocol errors are reserved for
malformed requests.

Clients that only inspected the JSON-RPC `error` field will now see a
successful response carrying `isError` — check that flag.

53 tools in this version.

## v2.0.0 — Alias removal + behaviour annotations

**Major** per the rules above: tools were removed.

### Removed (breaking)

Five aliases deprecated since v0.10.0 are gone. Tool definitions occupy
context in every agent session — the surface was ~11,000 tokens, of which
these were ~900 — and duplicate tools degrade tool-selection accuracy.

| Removed | Use instead |
| --- | --- |
| `roady_get_ready_tasks` | `roady_tasks` with `status=ready` |
| `roady_get_blocked_tasks` | `roady_tasks` with `status=blocked` |
| `roady_get_in_progress_tasks` | `roady_tasks` with `status=in_progress` |
| `roady_sticky_drift` | `roady_drift_recurring` |
| `roady_smart_decompose` | `roady_plan_decompose` |

### Added (backward-compatible)

- Every tool now carries `readOnlyHint`, `destructiveHint`,
  `idempotentHint`, and `openWorldHint`. Previously none did, so the
  spec's pessimistic defaults applied and read-only tools were advertised
  as potentially destructive.
- `roady_tasks` gains an optional `assignee` filter and an `unassigned`
  status value.
- `roady_task_transition` gains optional `session_id` and `agent`, recorded
  in the audit trail so work can be traced to a specific agent run.

54 tools in this version.

## v1.0.0 — Baseline

Initial schema version. All 37 existing MCP tools and their argument structs are frozen as the v1 contract:

`roady_init`, `roady_spec_get`, `roady_plan_get`, `roady_state_get`,
`roady_plan_generate`, `roady_plan_update`, `roady_drift_detect`,
`roady_drift_accept`, `roady_status`, `roady_policy_check`,
`roady_task_transition`, `roady_spec_explain`, `roady_plan_approve`,
`roady_usage_get`, `roady_drift_explain`, `roady_spec_add`,
`roady_forecast`, `roady_org_status`, `roady_git_sync`, `roady_sync`,
`roady_deps_list`, `roady_deps_scan`, `roady_deps_graph`,
`roady_debt_report`, `roady_debt_summary`, `roady_sticky_drift`,
`roady_debt_trend`, `roady_org_policy`, `roady_org_detect_drift`,
`roady_plugin_list`, `roady_plugin_validate`, `roady_plugin_status`,
`roady_messaging_list`, `roady_snapshot_get`, `roady_get_ready_tasks`,
`roady_get_blocked_tasks`, `roady_get_in_progress_tasks`

No deprecated fields.
