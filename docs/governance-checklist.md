# Governance Checklist

## Event log quick start
- `.roady/events.jsonl` is the single source of truth for governance decisions. Each newline is a self-contained JSON event with `action`, `actor`, `metadata`, `prev_hash`, `hash`, and `timestamp`.
- A typical plan approval entry looks like:
  ```json
  {
    "action": "plan.approve",
    "actor": "cli",
    "metadata": {
      "plan_id": "plan-gov-170",
      "spec_id": "gov"
    }
  }
  ```
- Every CLI and MCP command that touches the plan, drift, or AI runtime flows through `internal/infrastructure/wiring.BuildAppServices`, so both transports log the same events.

## Plan lifecycle steps
1. **Update the spec** (`docs/PLUGIN_ROADMAP.md`, `docs/`, or `.roady/spec.yaml`), then run `roady spec analyze docs/ --reconcile` if you keep the docs as the source of truth.
2. **Regenerate the plan** (`roady plan generate --ai` or `roady plan generate`). The action `plan.generate` writes the spec snapshot hash and plan ID so reviewers can see what triggered the change.
3. **Approve the plan** once it matches the roadmap (`roady plan approve`). Look for `plan.approve` to ensure the approval metadata (plan_id/spec_id) lines up with the latest snapshot.
4. **Prune or reject** if you experiment (`roady plan prune`, `roady plan reject`). Those commands emit `plan.prune` and `plan.reject`, making it easy to audit why plan structure changed.
5. **Accept drift intentionally** by running `roady drift accept`; this logs `drift.accepted` with the locked `spec_hash` and actor, proving governance consent for spec/plan divergence.

## Drift, AI, and retries
- A plan written back after a prompt records `plan.update_smart` with the agent
  and session behind it, so a reviewer can tell which agent produced it.
- Any governance event that forms part of a transition should include the `plan_id`/`spec_id` metadata so you can correlate log entries across commands.
- Roady calls no model, so there is no provider to record or override. A plan
  built from a prompt is attributed to the agent and session that ran it — see
  `docs/audit-grc.md` — which is what a reviewer needs to know instead.

## Auditing commands
- List all governance actions: `jq -c '.action' .roady/events.jsonl`.
- Inspect a specific plan approval: `jq -c 'select(.action=="plan.approve")' .roady/events.jsonl`.
- Follow a single plan lifecycle: `jq -c 'select(.metadata.plan_id=="plan-your-id")' .roady/events.jsonl`.
- Verify drift acceptance: `jq -c 'select(.action=="drift.accepted")' .roady/events.jsonl`.
- For coverage gaps or disputes, compare `.roady/events.jsonl` entries with the spec hash/timestamp in `.roady/spec.yaml`.

## See also
- `docs/prompts.md` for the operations that hand you a prompt to run, and where to write the answer back.
- `internal/infrastructure/wiring` so you know which services record audit events for both CLI and MCP.
