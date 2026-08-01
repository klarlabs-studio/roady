# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed — BREAKING: Roady no longer calls language models

Roady embedded provider clients and ran inference itself. An agent invoking
Roady already has a model, a key, a budget, and more context than Roady can
reconstruct from files — so a second model meant a second credential, a
second bill, and a worse answer.

The model-assisted operations survive as prompt builders: Roady assembles
the context and returns it, the caller runs inference, and results come back
through the named write-back tool. See `docs/prompts.md`.

- **Removed** `pkg/ai` and the Anthropic / OpenAI / Gemini / Ollama clients,
  `pkg/domain/ai`, `AIPlanningService`, `.roady/ai.yaml`, `ROADY_AI_PROVIDER`
  / `ROADY_AI_MODEL`, and all API-key handling. No key is needed for anything.
- **Changed** `roady_plan_decompose`, `roady_explain_spec`, `roady_review_spec`,
  `roady_query`, `roady_suggest_priorities`, and `roady_explain_drift` to
  return a prompt request (`operation`, `system`, `prompt`, `expected_format`,
  `write_back`, `guidance`) instead of model output. These now work with no
  configuration at all.
- **Changed** the matching CLI commands to print the prompt on stdout and the
  framing on stderr, so it pipes cleanly. `--json` emits the whole request.
- **Removed** `roady_cost_estimate` — Roady spends no tokens and cannot
  project a bill for a model it does not call.
- **Removed** `roady spec parse` and `roady spec analyze --reconcile`, whose
  only purpose was having a model structure text.
- **Changed** `roady watch --auto-sync` to regenerate with the deterministic
  planner. An unattended file watcher is the last place that should silently
  spend tokens.
- `allow_ai: false` still refuses these operations; the intent behind it does
  not change because inference moved to the caller.

## [0.14.1] - 2026-08-01

Patch release so the published tag sits on a commit with green CI. No
behaviour change for anyone using the CLI or MCP server.

### Fixed

- `AuditTrailService.BuildTrail` dereferenced the event-sourced audit
  service unconditionally, while `NewAuditTrailService` documents every
  collaborator except plan as optional — passing nil panicked. Both audit
  implementations now reduce to a normalised entry shape, so a nil
  event-sourced service falls back to the plain `AuditService`, which holds
  the same events. The CLI always passed a non-nil service, so this was
  unreachable from the shipped binaries.
- `.gitignore`'s bare `dist/` rule matched
  `internal/infrastructure/mcp/dist/`, silently dropping new MCP App
  artifacts from the index. This is why `ui://roady/billing` shipped
  registered but missing.

### Added

- Test coverage for the service entry points and the four new command
  runners, restoring the coverage policy gate (application 83.2%,
  infrastructure 77.2%).

## [0.14.0] - 2026-08-01

Coordination and stakeholder reporting without a UI, agent-traceable audit
trails, and bidirectional tracker sync. Three breaking changes, all listed
below.

MCP schema version is now **2.0.0** (see `docs/mcp-schema-changelog.md`).
`pkg/sdk` moves to `SupportedSchemaMajor = "2"` to match; an SDK build
pinned to major 1 will refuse to talk to a v0.13 server, by design.

### Added — coordination

- `roady task mine`, `roady task assigned <name>`, and `roady task unassigned`
  make assignment readable. Previously `roady task assign` wrote an owner that
  nothing could query back, so "who is working on what" had no answer.
- MCP `roady_tasks` gains an `assignee` filter and an `unassigned` status.
- `policy.max_wip_per_owner` caps in-progress work per person. The existing
  project-wide `max_wip` let one person hold the entire allowance.
- `policy.enforce_team_roles` (default `false`) makes `.roady/team.yaml` a
  guard rather than documentation — a listed viewer can no longer transition
  tasks. Only actors present in the roster are checked, so unlisted actors and
  existing projects are unaffected.

### Added — stakeholder reporting

- `roady report` renders progress, forecast, risks, ownership, and a change
  log as Markdown, self-contained HTML (~5KB, no scripts or external
  requests, light/dark aware, printable), or JSON. `--since` accepts `7d`,
  `2w`, or an absolute date.
- `roady notify digest` sends one chat-sized progress summary through
  configured adapters instead of a message per domain event. Supports
  `--dry-run`, `--adapter`, and `--since`.
- Completion estimates are withheld below three velocity data points rather
  than printing a date nobody should plan around.

### Added — audit trails for GRC

- `roady audit trail [task-id] [--agent X] [--session Y] [--since 30d]`
  produces an evidence trail: chain-integrity status, findings, the task's
  evidence and spec citation, who acted, and every recorded event. Markdown
  or JSON. Exits non-zero when chain verification fails, so it can gate CI.
- Every event now records the agent and session behind it. Resolution:
  `ROADY_SESSION_ID` / `ROADY_AGENT`, then runtime detection (Claude Code,
  Cursor, Codex, Gemini CLI), plus the surface (`cli` / `mcp` / `plugin`).
  A session is minted once per process, so one MCP server run groups an
  agent's whole conversation. Previously every agent recorded as the literal
  string `ai-agent`, making "which agent did this?" unanswerable.
- MCP `roady_transition_task` accepts `session_id` and `agent`; a
  caller-supplied identity overrides the ambient process one.
- `docs/audit-grc.md` documents what a trail attests — **a tamper-evident
  record of what was asserted, not proof of identity**, since actor and agent
  are caller-supplied and unauthenticated.

### Removed — BREAKING

- Stale duplicate docs: `docs/roadmap.md` (superseded by `ROADMAP.md`,
  and whitespace-corrupted — 178 of 232 lines were blank) and
  `docs/small.md` (a truncated copy of `docs/vision.md`).
- The web dashboard is gone: `roady dashboard serve`, `roady dashboard
  open`, the `/kanban` and `/org/kanban` boards, `/api/*` endpoints, the
  SSE stream, action endpoints, and the shared-token auth. The whole
  `pkg/infrastructure/dashboard` package and `docs/dashboard.md` are
  removed.

  Rationale: an agent can render and update a human-readable view from
  the same data through Roady's MCP Apps, and people who are not in an
  agent client are better served by a document than by a server they
  have to reach. Replacements:

  - `roady report --format html -o status.html` — shareable progress
  - `roady notify digest` — push a summary to a channel
  - `roady dashboard` — the interactive TUI, unchanged

### Added — parallel collaboration

- `roady audit verify` treats the event log as a hash-linked graph instead
  of a strict sequence. Two people appending concurrently produced branches
  that union-merge cleanly but failed verification, which made parallel work
  impossible. Nothing is given up: each event's hash already covers its own
  content and its parent reference, so tampering and reparenting are still
  caught, and a removed event still leaves a dangling parent.
- `.gitattributes` marks `events.jsonl` `merge=union` so git merges it
  instead of conflicting.
- `LoadEvents` deduplicates by event ID so a line reproduced by a union
  merge cannot double-count in velocity, cost, or task projections.
  Verification reads the raw log via `LoadEventsRaw` and still reports
  duplicates.
- `roady state rebuild [--dry-run]` reconstructs `state.json` by replaying
  the log. `state.json` is a whole-file document and still conflicts in git;
  it is derived data, so the resolution is to keep either side and replay.
  The replay is idempotent and order-independent.

### Changed — MCP agent experience

- Every MCP tool now carries behaviour annotations (`readOnlyHint`,
  `destructiveHint`, `idempotentHint`, `openWorldHint`). None did before,
  and the spec's defaults are pessimistic — an unannotated tool is assumed
  not read-only and potentially destructive, so ~30 pure read tools were
  telling every client that reading a plan might destroy something.
  Classification is accurate rather than convenient: the AI tools are
  deliberately *not* marked read-only because they record token usage to
  the audit log, and plan generation / drift acceptance are marked
  destructive because they overwrite state that cannot be recovered.
- Tests fail the build if a tool is registered without a classification,
  if a classification outlives its tool, or if the judgement calls above
  are reversed.

### Removed — BREAKING (MCP)

- The five deprecated tool aliases from v0.10.0 are gone:
  `roady_get_ready_tasks`, `roady_get_blocked_tasks`,
  `roady_get_in_progress_tasks` (use `roady_tasks` with `status`),
  `roady_sticky_drift` (use `roady_drift_recurring`), and
  `roady_smart_decompose` (use `roady_plan_decompose`).
  Tool definitions cost context in every agent session: the surface was
  ~11,000 tokens, of which these five were ~900. Duplicate tools also
  degrade tool-selection accuracy. Now 54 tools, ~10,200 tokens.
  `pkg/sdk` is repointed at the canonical tools; its method signatures are
  unchanged, so SDK consumers need no edit.

### Added — bidirectional tracker sync

- `roady sync` now writes Roady's status back to the external tracker.
  `Syncer.Push` was implemented by all seven plugins but called from
  nothing, so sync was read-only despite the docs claiming otherwise.
  `--no-push` restores pull-only behaviour.
- Inbound sync honours all five statuses. Previously only `done` and
  `in_progress` were mapped and `blocked`/`pending`/`verified` were
  silently dropped, so a task blocked in Jira stayed pending in Roady.
- `planning.EventForTransition` / `planning.PathToStatus` reverse-map the
  FSM, so a status the tracker reports is reached by walking real
  transitions rather than being written past the state machine.

### Fixed

- All 14 committed MCP App artifacts were stale, built against older
  dependencies. Rebuilt, and CI now rebuilds them on every run and fails
  on drift — nothing previously verified that the committed HTML matched
  `app/src`, which is how they went stale unnoticed.
- The website still advertised the removed web dashboard (`roady
  dashboard serve`, a "Live Kanban" feature card, and a `/org/kanban`
  reference). Replaced with `roady report` and `roady audit trail`.
- `ui://roady/billing` was registered as an MCP App but its built file was
  never committed, so every request for it failed at runtime. The app is
  now built and shipped, with a test asserting every registered app
  resource is readable.
- Plugin provider inference matched only github/jira/linear, so trello,
  asana, and notion all filed task links under `external` and collided in
  `ExternalRefs`. All six are recognised, and custom plugins derive a
  stable name from their binary.

- Service-load warnings printed to stdout, corrupting `--json` output on every
  command that offers it. They now go to stderr.
- `roady task start` resolved the actor from `USER` while `roady task mine`
  used `ROADY_USER` then `git user.name`; ownership and the new per-owner
  guards disagreed about identity. Both now share one resolver.

## [0.12.0] - 2026-05-16

Four polish features on top of v0.11.3's Kanban. All backward-compatible.

### Added — Reopen action + DnD transition

- New `TaskService.ReopenTask` + `POST /actions/task/reopen` move a Done or Verified task back to Pending.
- Done cards on `/kanban` get a **↺ Reopen** button.
- DnD adds Done → Backlog and Done → Ready transitions (both call reopen).

### Added — Live updates over Server-Sent Events

- New `GET /events` SSE stream emits a `task-changed` event after every successful task transition. The board reloads within ~200 ms of the change instead of waiting on the meta-refresh.
- Live indicator in the header reflects connection state.
- Meta-refresh kept as fallback (now 60 s) for browsers without `EventSource`.
- 25 s heartbeat keeps the stream alive through proxies (Cloudflare, nginx).

### Added — Cross-project Kanban actions (DnD on `/org/kanban`)

- Cards on `/org/kanban` are now draggable. Drops route the mutation to the right sub-project's `TaskService` via the new `OrgTaskActions` resolver.
- Action endpoints accept optional `project_path` and `project` form fields; absent → defaults to the server-default `TaskActions`, present → routes through `OrgTaskActions.ResolveTaskActions`.
- Wired automatically by `roady dashboard serve`.

### Added — Dashboard auth token

- New `--auth-token <value>` flag on `roady dashboard serve|open` (env: `ROADY_DASHBOARD_TOKEN`) protects every dashboard request with a shared bearer token.
- Token accepted three ways: `Authorization: Bearer <t>`, `Cookie: roady_token=<t>`, or `?token=<t>` (one-time handshake: sets the cookie, redirects to strip the secret from the URL bar).
- Constant-time comparison; secure cookie on TLS / X-Forwarded-Proto.
- Empty token = public (backward-compatible).

## [0.11.3] - 2026-05-16

### Added — Drag-and-drop on the Kanban board

- Cards on `/kanban` are now draggable between columns. Drop a card on the target column to transition the task — no need to hunt for the right button.
- Valid transitions (mapped to existing POST endpoints): Ready → In Progress (start), In Progress → Done (complete), In Progress → Blocked (block), Blocked → In Progress (unblock).
- Visual feedback: target column outlines green for allowed drops, red for disallowed transitions (e.g. dragging a Done card onto Blocked is a no-op).
- Buttons remain available — drag-and-drop is additive, not a replacement. Server reloads on drop so the board reflects the new authoritative state.
- Vanilla HTML5 DnD; no client library. Only renders when task actions are wired (`Server.EnableTaskActions`); read-only boards stay read-only.

## [0.11.2] - 2026-05-16

### Added — Interactive Kanban (task action buttons)

- The `/kanban` board is no longer read-only. Cards now render contextual buttons by column: **Ready → Start**, **In Progress → Complete / Block**, **Blocked → Unblock**.
- New POST endpoints: `/actions/task/start`, `/actions/task/complete`, `/actions/task/block`, `/actions/task/unblock`. Form-encoded, sender redirected back to the referring page.
- Wired automatically by `roady dashboard serve` via the existing `TaskService`. No extra flags.
- Backward-compatible: if a custom server is constructed without `EnableTaskActions`, the action routes stay unregistered and the board is read-only.

## [0.11.1] - 2026-05-16

Adds the cross-project Kanban view that ties together v0.11.0's nested
sub-projects + per-project Kanban features. One agent, many feature
streams, one live board.

### Added — Cross-project Kanban (`/org/kanban`)

- New `/org/kanban` route on the dashboard renders every project under the workspace root — the root project plus every sub-project under `.roady/projects/<name>/` — merged into one five-column board.
- Cards are tagged with their origin project label so it's clear which row each task belongs to.
- New `/api/org/kanban` JSON endpoint exposes the same board for external tools.
- Header strip lists every discovered project with task and done counts.
- Auto-refreshes every 30s.
- Wired automatically by `roady dashboard serve`; no extra flags.

## [0.11.0] - 2026-05-16

Adds two user-facing surfaces that lay the groundwork for an agent
managing many parallel feature streams in one repo: nested sub-projects
and a live Kanban view. Both are fully backward-compatible.

### Added — Nested sub-projects

- A single repository can now host multiple Roady projects side-by-side under one `.roady/` directory by placing each named sub-project at `.roady/projects/<name>/`. See `docs/rfcs/0001-nested-projects.md`.
- New global CLI flag `--project / -P <name>` (env: `ROADY_PROJECT`) scopes every command to a named sub-project. With no flag, commands target the repo's root project, exactly as before.
- New MCP request field `project` (optional, alongside the existing `project_path`) routes tool calls to a sub-project. `AppServices` are cached per `(path, project)` key.
- `roady discover` and `roady org status` now surface sub-projects in addition to root projects.
- New storage constructor `storage.NewFilesystemRepositoryForProject(root, name)` plus helpers `ProjectBase()`, `SubProject()`, `IsSubProject()`. The legacy `NewFilesystemRepository(root)` continues to return a root-project repository unchanged.
- New `OrgService.DiscoverProjectsWithSub()` returns both root projects and sub-projects; legacy `DiscoverProjects()` is kept and unchanged in shape.
- Backward-compatible: existing flat `.roady/` repos continue to work unchanged. No data migration required.

### Added — Kanban dashboard view

- New `/kanban` route on the web dashboard (`roady dashboard serve`) renders the project's tasks across five status columns: **Backlog · Ready · In Progress · Blocked · Done**. Auto-refreshes every 30s.
- "Ready" computes dependency satisfaction so unblocked pending tasks surface separately from backlog items waiting on upstream work. "Done" rolls Verified into Done.
- New JSON endpoint `/api/kanban` returns the same board for external tools / IDE plugins / CI.
- Nav bar across dashboard pages gains a Kanban link.

## [0.10.1] - 2026-05-03

Patch release. No user-facing feature changes. Closes audit gaps flagged before the public launch wave.

- Plugin builds (`asana`, `github`, `mock`, `notion`, `trello`) added to GoReleaser. Every plugin now ships in the bundled release archive (previously only `linear` and `jira` shipped).
- CI gains a `golangci-lint` job. Full repo on main pushes; `--new-from-rev=origin/main` on PRs to avoid drowning new contributions in pre-existing warnings.
- New `release-smoke-test` CI job downloads the freshly published Linux/amd64 archive, runs `roady --version` and `roady demo` against it, fails the workflow on any error.
- Cloud waitlist form replaced its hidden-localStorage backend with a transparent `mailto:` fallback. No more form-without-a-server pattern.
- New GitHub Pages workflow (`.github/workflows/website.yml`) auto-deploys the Astro site on every main push affecting `website/`, `README.md`, or `docs/`.
- This `CHANGELOG.md` extended through v0.10.
- `fix(ci)`: website deploy uses Node 22 for Astro 5+ compatibility.
- `fix(ci)`: pin golangci-lint to `@latest` for go 1.26 compatibility.

## [0.10.0] - 2026-05-03

Bundles the v0.9 + v0.10 cycles. v0.9 was cut as a soft release and never tagged.

### Added — v0.9.0 Activation & Clarity

- `roady --help` grouped by user intent (Get Started / Track & Report / Integrate / Admin) via cobra command groups.
- `roady init` defaults to interactive wizard in a TTY (proper isatty check); `--non-interactive` flag for CI; next-step CTA on completion.
- `roady demo` command — scaffolds a pre-seeded sample with intentional spec/lock divergence, runs drift detect. Sub-second aha for first-time visitors.
- `roady status` empty-state ladder (`uninitialised` / `no-spec` / `no-plan`) with actionable next-step hints.

### Added — v0.10.0 AI Quality & Telemetry

- Eval harness (`evals/`) over the planning pipeline: golden fixtures (cli-tool / web-api / multi-feature), AI planner contract via programmable mock, drift precision/recall corpus. Opt-in real-provider matrix via `-tags evals_ai`.
- `Task.Origin` provenance (`heuristic | ai | human`) with source-doc citations propagated end-to-end (`from doc:line` shown in `roady status`).
- Native streaming via `OnToken` callback on `ai.CompletionRequest`. Real SSE / NDJSON wiring for Anthropic, OpenAI, Gemini, Ollama. AI service auto-routes via `ai.WithOnToken` context helper. CLI prints streamed tokens live; MCP forwards as progress notifications.
- `Confidence` and `Sources` on `CompletionResponse`. Real providers populate `Confidence` from natural stop signal.
- MCP tool consolidation: parameterised `roady_tasks` (status enum) supersedes the three legacy `roady_get_*_tasks` tools, kept as deprecation aliases. Canonical `roady_plan_decompose` and `roady_drift_recurring` aliases for off-pattern names.
- `roady_cost_estimate` MCP tool — pre-flight token + USD projection with pricing table for Anthropic / OpenAI / Gemini.
- Unified `roady notify` namespace (add/list/test/remove). `roady messaging` and `roady webhook notif` retained as deprecation aliases.
- AI command progress + clean cancellation via shared `withAIProgress` wrapper across the five AI CLI surfaces.

### Changed — v0.11 Positioning

- New positioning: "planning memory for AI coding agents" (treated as hypothesis pending real ICP validation; see `docs/positioning.md`).
- README rewritten around a single 5-step workflow.
- `docs/positioning.md`, `docs/vs.md`, `docs/advanced.md`, `ROADMAP.md` shipped.
- Astro website hero / definition / features / commands / MCP / integrations sections realigned with the new positioning.

### Fixed

- TTY detection used `os.ModeCharDevice` which returned `true` for `/dev/null` on Linux, breaking the e2e test on CI by silently triggering the interactive wizard. Switched to `go-isatty` for proper termios check.
- Pre-existing `readStdin` bug in `cli/spec.go` that always returned `""`.
- 40 `errcheck` lint warnings on writer outputs across v0.10 additions.

### Security

- All open dependabot advisories closed (postcss XSS, astro XSS, astro allowlist bypass).

### Breaking

- None. Every deprecated MCP tool name continues to work.

## [0.9.2] - 2026-03-24

Patch release. See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.9.2).

## [0.9.1] - 2026-03-24

Patch release. See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.9.1).

## [0.9.0] - 2026-03-21

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.9.0).

## [0.8.0] - 2026-03

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.8.0).

## [0.7.3] - 2026-02-17

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.7.3).

## [0.7.0] - 2026-02

See [GitHub release notes](https://github.com/felixgeelhaar/roady/releases/tag/v0.7.0).

## [0.6.3] - 2026-02-08

### Fixed

- Propagate actual error details in `roady_transition_task` and `roady_assign_task` MCP handlers instead of returning generic messages ([#3](https://github.com/felixgeelhaar/roady/issues/3))
- Initialize task status to `pending` when `SetTaskOwner`, `AddEvidence`, or `SetExternalRef` creates a new entry in the task states map, preventing broken transitions on tasks assigned before their first status write

## [0.6.2] - 2026-02-03

### Fixed

- Fix undefined label in spec MCP app visualization (title vs name mismatch)
- Fix undefined `has_drift` field in drift MCP app (derive from issues array length)

## [0.6.1] - 2026-02-01

### Fixed

- Fix burndown chart dipping below zero on completed projects
- Enable Vue runtime compiler and remove TypeScript syntax from templates

## [0.6.0] - 2026-02-01

### Added

- Org-level dashboard aggregating status across multiple Roady projects
- fsnotify-based file watching replacing polling
- Plugin contract testing for Syncer interface
- Reliable webhook delivery with retry
- Org policy inheritance with child project overrides
- Cross-project drift detection
- Plugin registry with versioning and health monitoring
- Pluggable messaging adapters (Slack support)
- Realtime event streaming via SSE
- Selective watch patterns with include/exclude globs
- Auto-sync workflows on file changes
- Shell completions for bash/zsh/fish/powershell
- Structured CLI error types with actionable hints
- Interactive config wizard for `ai.yaml` and `policy.yaml`
- Guided onboarding with starter templates (minimal, web-api, cli-tool, library)
- Versioned MCP tool schema with backward-compatible evolution
- Public Go SDK client package (`pkg/sdk/`)
- OpenAPI 3.0 spec generation from MCP tools
- Typed SDK request/response helpers
- Task assignment with owner field
- Role-based access control (`team.yaml`)
- Optimistic locking for concurrent state modifications
- Git-based workspace sync (push/pull)
- AI spec quality review with completeness scoring
- AI priority suggestions from dependency analysis
- Context-aware task decomposition with codebase scanning
- Natural language query interface for project status
- Interactive D3.js MCP app visualizations (org, sync, forecast, git-sync, and 10 others)

### Fixed

- Move roady-sync example out of workflows to prevent CI execution

## [0.5.0] - 2026-01-30

### Added

- Event-sourced audit in production with `FileEventStore` and `InMemoryEventPublisher`
- Domain event dispatcher with `LoggingHandler`, `DriftWarningHandler`, and `TaskTransitionHandler`
- Live velocity projection subscribing to event publisher for real-time updates
- D3.js interactive visualizations across 10 MCP apps (donut, force-directed graph, arc gauge, horizontal bars, line chart, collapsible tree, swimlane)
- Vue 3 + D3.js app source with Vite build pipeline and `apps.go` embed directive

### Changed

- `InitService` and `DebtService` now accept `domain.AuditLogger` interface instead of concrete `*AuditService`
- `BaseEvent` includes `Action` field for backward-compatible JSON serialization
- `FileEventStore` defers directory creation to first write, avoiding interference with `IsInitialized()` checks
- Deduplicated `BuildAppServices` into shared `buildServicesWithProvider` helper

### Fixed

- User-friendly MCP errors replacing raw Go error strings
- FlexBool/FlexInt types accepting both native and string JSON values
- Embedded MCP app dist files committed for CI `go:embed` compatibility

## [0.4.1] - 2026-01-19

### Fixed

- Patch release with minor fixes (see [v0.4.0...v0.4.1](https://github.com/felixgeelhaar/roady/compare/v0.4.0...v0.4.1))

## [0.4.0] - 2026-01-16

### Added

- GitHub Actions CI integration
- Web dashboard for plan visualization
- Event sourcing for audit trail
- gRPC transport for plugin communication and MCP
- Push method on Syncer interface for bidirectional sync
- Notion, Asana, and Trello plugins
- Push support for Linear, GitHub, Jira, and mock plugins
- Interactive TUI for plugin configuration with auto-install
- Per-plugin configuration file support
- HTTP webhook server for real-time sync
- Marketing website migrated to Astro with Vue

### Changed

- MCP wiring refactored with split AI config
- Phase 3 and Phase 4 code quality improvements

### Fixed

- CI builds binary to `dist/` for e2e tests
- Website `.nojekyll` for GitHub Pages compatibility

## [0.3.0] - 2026-01-13

### Added

- Drift accept command for acknowledging intentional spec divergence

## [0.2.1] - 2026-01-13

### Changed

- Expose core packages in `pkg/` to enable library usage
- Inject version flags in goreleaser config

## [0.2.0] - 2026-01-13

### Added

- Jira plugin for bidirectional sync
- Linear plugin with external task linking
- External refs on task state for plugin integration
- Interactive dashboard (`roady dashboard`) TUI for visualizing plans and drift
- Dynamic policy engine with configurable `.roady/policy.yaml`
- Plugin architecture with gRPC-based Syncer interface
- Smart plan injection via `roady_update_plan` MCP tool
- GoReleaser for multi-platform builds and Homebrew distribution
- Release automation for GitHub and Homebrew

## [0.1.0] - 2026-01-13

### Added

- Core domain models: Spec, Plan, Task, Drift
- Spec ingestion from Markdown (`roady spec import`)
- Spec locking (`.roady/spec.lock.json`) for immutable planning boundaries
- Plan reconciliation merging new specs without destroying existing task state
- Drift detection for Spec vs Plan and Plan vs Code
- Audit trail with structured logging to `.roady/events.jsonl`
- MCP server (`roady mcp`) exposing core tools to AI agents
- Resilience via `fortify` integration for filesystem retries
- State management via `statekit` FSM for task transitions

[Unreleased]: https://github.com/felixgeelhaar/roady/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/felixgeelhaar/roady/compare/v0.9.2...v0.10.0
[0.9.2]: https://github.com/felixgeelhaar/roady/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/felixgeelhaar/roady/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/felixgeelhaar/roady/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/felixgeelhaar/roady/compare/v0.7.3...v0.8.0
[0.7.3]: https://github.com/felixgeelhaar/roady/compare/v0.7.0...v0.7.3
[0.7.0]: https://github.com/felixgeelhaar/roady/compare/v0.6.3...v0.7.0
[0.6.3]: https://github.com/felixgeelhaar/roady/compare/v0.6.2...v0.6.3
[0.6.2]: https://github.com/felixgeelhaar/roady/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/felixgeelhaar/roady/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/felixgeelhaar/roady/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/felixgeelhaar/roady/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/felixgeelhaar/roady/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/felixgeelhaar/roady/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/felixgeelhaar/roady/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/felixgeelhaar/roady/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/felixgeelhaar/roady/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/felixgeelhaar/roady/releases/tag/v0.1.0
