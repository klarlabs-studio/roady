# Roady — Roadmap

> Public, opinionated, subject to change. Issues with the `roadmap`
> label are the live tracking surface.

The CLI + MCP server is and stays free, MIT, and self-hostable. The
roadmap below describes what's coming next on the open core, and the
intended **open-core boundary** for a future hosted product.

---

## Now (v0.21.0 — shipped)

- **Derived-state reconciliation** — `roady spec lock` re-captures the drift
  baseline from the current spec and reconciles execution state with it, and
  `spec validate` reports when they disagree instead of answering "valid".
  Found by adopting Roady in an existing project (#77).

## Earlier (v0.20.0 — shipped)

- **Correctness sweep** — one audit-chain verifier instead of two that
  disagreed on the same log; `roady_timeline` reading the same source as the
  CLI; dead AI telemetry removed; and the CLI help and docs corrected to
  describe a Roady that calls no model.

## Earlier (v0.19.0 — shipped)

- **Semantic drift** — `roady drift semantic` frames the question of whether
  an implementation still means what its requirement says; the caller's model
  answers it and `roady_record_semantic_drift` records divergence.
- **CLI/MCP parity** — 69 tools; every CLI operation an agent should be able
  to perform is reachable over MCP.

## Earlier (v0.18.0 — shipped)

- Five field-reported defects: id sanitisation, feature-link resolution,
  documentation written to the project rather than the server's cwd, paged
  task listings, and MCP errors carrying the actionable message.

## Earlier (v0.17.0 — shipped)

- **`roady_audit_trail` over MCP** — the evidence trail was CLI-only, so
  the agents the GRC work targets could not ask "which agent worked on
  this, and what proves it" without shelling out. MCP schema 3.1.0.

## Earlier (v0.16.0 — shipped)

- **Staleness drift** — `roady drift detect` reports a plan the repository
  has left behind, judged by commit volume rather than file timestamps.
  Found by running Roady on Roady: 55 commits and seven releases past a
  plan marked complete, reported as healthy.
- **Audit verification correctness** — three distinct causes were all
  being reported as "possible tampering": two hash functions writing one
  log, a shell script appending unhashed entries, and a hash-algorithm
  change that invalidated history. Events now carry `hash_algo`.
- **`plan prune` cleans execution state**, which it previously orphaned.

## Earlier (v0.15.0 — shipped)

- **Roady runs no inference.** The provider clients are gone. The
  model-assisted operations return the assembled prompt plus the tool
  that accepts the result; the caller, which already has a model, runs
  it. No API key is needed for anything. See
  [`docs/prompts.md`](docs/prompts.md).
- **MCP tool errors reach the agent.** Failures are returned as results
  carrying `isError` with a readable message, rather than as JSON-RPC
  protocol faults the transport replaced with "internal error".
- MCP schema 3.0.0; `pkg/sdk` requires major 3.

## Earlier (v0.14.0 — shipped)

- **Coordination**: owner-scoped task queries (`roady task mine |
  assigned <name> | unassigned`), per-owner WIP limits
  (`policy.max_wip_per_owner`), and enforceable `team.yaml` roles
  (`policy.enforce_team_roles`).
- **Stakeholder reporting without a UI**: `roady report` renders
  progress, forecast, a risk register, ownership, and a change log to
  Markdown, self-contained HTML, or JSON. `roady notify digest` pushes
  one chat-sized summary through configured adapters.
- **Removed the web dashboard** (BREAKING). Agents render and update a
  human-readable view from the same data through MCP Apps, and
  stakeholders get a document rather than a server they must reach.
  `roady dashboard` remains as the TUI.

## Recently (v0.11.x — shipped)

- **Nested sub-projects** under `.roady/projects/<name>/`. One repo
  hosts many projects in parallel; coding agents switch context with
  `--project / -P <name>` (CLI) or `project` (MCP). Existing flat
  `.roady/` repos unchanged. See
  [`docs/rfcs/0001-nested-projects.md`](docs/rfcs/0001-nested-projects.md).
- `roady discover` and `roady org status` surface sub-projects.

## Earlier (v0.10.x — shipped)

- Eval harness over heuristic + AI planners + drift corpus
- Task provenance: `Origin` (heuristic / ai / human) + source citations
  back to the doc that motivated each task
- Provider streaming end-to-end (Anthropic / OpenAI / Gemini / Ollama)
  with MCP progress notifications and CLI `withAIProgress` integration
- MCP tool consolidation (`roady_tasks` parameterised + deprecation
  aliases for the legacy task-listing tools)
- `roady_cost_estimate` pre-flight token + USD projection per AI op
- Unified `roady notify` namespace; `messaging` and `webhook notif`
  retained as deprecation aliases
- AI command progress + clean SIGINT cancellation across the five AI
  CLI surfaces
- `roady demo` for <1s aha; `roady init --interactive` default in TTY;
  empty-state ladder on `roady status`

## Next

Nothing declared. The roadmap below is what remains after v0.19.0 shipped the
cross-project dependencies, subagent dispatch, spec-to-PR loop, drift patch
prompts, cross-repo planning and semantic drift that were listed here.

The most useful input now is field use: v0.18.0 came entirely from one person
running Roady on a real 118-feature project for a day.

## Later

- **Plugin marketplace** for syncers and notifiers, opinionated quality
  bar (signed binaries, contract tests must pass).
- **Semantic drift prompts** — a `drift.semantic` prompt request that
  hands the caller the implemented behaviour alongside the spec's
  natural-language requirement, so their model can judge whether the two
  still agree. Roady frames the question; it does not answer it.

---

## Roady Cloud (future, no committed date)

Open-core boundary, intended scope:

- **Hosted MCP** — managed multi-tenant MCP server so teams without
  a per-developer install can plug their agents into a shared
  workspace.
- **Multi-repo org dashboard** with persistent storage and historical
  metrics across all member projects.
- **Audit log retention** beyond what fits comfortably in `events.jsonl`,
  with structured search and export.
- **SSO / RBAC / SCIM** for enterprise IdP integrations.
- **SOC 2** compliance posture for the hosted plane.

What stays open and free, forever:

- The full CLI, MCP server, and every planning / drift / spec /
  notify / billing capability.
- The `.roady/` file format and all storage adapters.
- Plugin contract tests + reference syncer plugins.

If Cloud lands, opting in is a `roady cloud login` away. Opting out is
the existing local workflow with no behavioural change.

---

## Out of scope

- **Matching Linear / Jira feature-for-feature.** No sprints, custom
  fields, configurable workflows, or non-engineer intake queues. Roady
  covers two of the jobs those tools do — coordinating who is on what,
  and keeping stakeholders informed — with generated documents and push
  notifications rather than an app. Where non-engineers create and
  triage work daily, use a tracker; Roady syncs with it bidirectionally.
- A web-based code editor or AI agent of our own.
- Hosted general-purpose memory for non-coding workflows.
- **Authenticated identity.** Actors and agents are asserted by the
  caller and never verified. Roady's audit trail is tamper-evident about
  what was recorded, not proof of who acted — see
  [`docs/audit-grc.md`](docs/audit-grc.md).

If you want any of these, Roady is the wrong tool — we are deliberately
narrow.
