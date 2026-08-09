# MCP Schema Changelog

## Schema Evolution Rules

- **Patch** (1.0.x): Description fixes only
- **Minor** (1.x.0): New optional fields (`omitempty`), new tools, fields deprecated
- **Major** (x.0.0): Required fields added/removed, tool signatures changed

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
