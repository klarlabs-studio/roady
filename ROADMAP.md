# Roady — Roadmap

> Public, opinionated, subject to change. Issues with the `roadmap`
> label are the live tracking surface.

The CLI + MCP server is and stays free, MIT, and self-hostable. The
roadmap below describes what's coming next on the open core, and the
intended **open-core boundary** for a future hosted product.

---

## Now (v0.14.0 — shipped)

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

## Next (v0.15.x)

- **Cross-project task dependencies** — `@project:task-id` syntax in
  `DependsOn` so an org-level plan can express "feature-payments task
  X waits on feature-auth task Y".
- **Drift explainer follow-ups**: synthesised "explain + propose patch"
  output that lands a PR-ready diff for accepted drift.
- **Per-task subagent dispatch**: `roady task start` can hand a ready
  task to a subagent (Claude Code `Task` tool, Codex `agent run`, etc.)
  with the spec source attached and a deterministic completion hook.
- **Spec-to-PR loop**: CI integration that auto-detects drift on PR
  merge and either accepts or opens a follow-up issue.

## Soon (v0.14+)

- **Cross-repo planning**: a single `.roady/` workspace can declare
  member repos, share spec context, and aggregate plan state.

## Later

- **Plugin marketplace** for syncers and notifiers, opinionated quality
  bar (signed binaries, contract tests must pass).
- **Native source citations** through providers that surface them
  (Gemini grounding metadata, Anthropic citation API once stable).
- **Drift detection over code semantics**, not just structure — diff
  the implemented behaviour against the spec's natural-language
  requirement using a constrained AI checker.

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
