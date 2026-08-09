# Roady — everything else

The README hero ships a single workflow on purpose. Everything else
Roady can do lives here, grouped by who it's most useful to.

If a feature ever gets popular enough to be promoted into the headline
workflow, move it back into the README and trim accordingly.

---

## Solo dev / individual contributor

These are the features that show up *naturally* once you are using
Roady on more than one repo or for more than a week.

### Spec-driven inference

`roady spec analyze docs/` walks any directory of markdown and emits a
structured spec with feature/requirement source citations
(`from docs/auth.md:14`). Every task downstream carries the citation.
`--reconcile` was removed with the embedded providers: run `roady spec explain`
to get a prompt, reconcile with your own model, and write the result back with
`roady spec add`.

### AI planning workflows

- `roady plan generate --ai` — prints a decomposition prompt for your own
  model instead of running the heuristic planner. Roady calls no provider;
  write the answer back with `roady_plan_update`. Tasks that arrive that way
  are tagged `Origin=ai` and surface as `[AI]` in `roady status` so reviewers
  can scrutinise them.
- `roady plan smart-decompose` — the same, grounded in a file-tree analysis of
  the codebase.
- `roady plan prioritize` — AI suggestions for re-balancing task
  priority based on dependencies + spec.
- `roady query "how am I doing?"` — natural language Q&A over the
  current project state.
- `roady drift explain` — AI-generated narrative of every drift
  issue and a suggested fix.

All of the above stream tokens to your terminal in real time and abort
cleanly on Ctrl-C.

### AI cost guardrails

- `policy.yaml` has `max_wip`, `allow_ai`, `token_limit`. The token
  limit is a hard cap; AI calls fail with a clear error when reached.
- `roady_cost_estimate` MCP tool returns input / output token estimate
  and USD projection per AI operation, before you spend the tokens.
  Pricing table covers Anthropic / OpenAI / Gemini; Ollama and unknown
  models report `pricing_known: false`.
- `roady usage` shows actual token consumption to date.

### Predictive forecasting

- `roady forecast` with `--detailed`, `--burndown`, `--trend` flags.
  Multi-window velocity (7 / 14 / 30 day) with confidence intervals
  rather than a single point estimate.

### Watch mode

- `roady watch docs/ --reconcile` — fsnotify-based file watching
  (efficient OS-level events, not polling). Optional include/exclude
  glob patterns and an auto-sync flag that runs drift detect on each
  change.

### Interactive views

- `roady dashboard` — TUI built with bubbletea/lipgloss.
- **MCP Apps** — 15 inline UIs (`ui://roady/status`, `plan`, `drift`,
  `forecast`, `debt`, `deps`, …) rendered by your agent client when it
  calls the matching tool, with D3 visualisations and action buttons
  that call back into Roady. No server, no port, no login.

The web dashboard (`roady dashboard serve`) was removed; see
`roady report` and `roady notify digest` below.

### Shell completions + interactive setup

- `roady completion bash|zsh|fish|powershell`
- `roady config wizard` — interactive `ai.yaml` + `policy.yaml` setup
- `roady doctor` — health check across config, dependencies, plugins
- `roady init --interactive` (default in TTY) — template-driven
  onboarding

---

## Small team (2–10)

### Multi-user collaboration

- Task assignment with `Assignee` field
- Role-based access in `.roady/team.yaml` (admin / member / viewer)
- Optimistic locking for concurrent state edits
- `roady workspace push|pull` to share `.roady/` via git remote with
  conflict detection

### Notifications

- `roady notify add <name> webhook|slack <url>` — unified outbound
  notifications. Webhook delivery is HMAC-SHA256 signed with retry +
  dead-letter queue. Slack adapter ships out of the box.
- `roady notify test <name>` — dispatch a synthetic test event.
- Realtime event streaming via SSE for live UIs.

### Billing

- `roady rate add` / `roady rate set-default` — hourly rates with
  multi-currency and tax support.
- Time logging on task transitions; `roady cost report` produces
  estimated-vs-actual variance.
- `roady cost budget` — budget tracking against configured limits.

### Debt analysis

- `roady debt report` — quantified planning debt from recurring drift
  patterns.
- `roady debt trend` — historical debt curve.
- `roady drift recurring` (alias `roady_drift_recurring` MCP tool) —
  drift items unresolved for more than 7 days.

---

## Org / multi-repo

### Discovery + cross-project views

- `roady discover` — finds every `.roady/` project under a path.
- `roady org status --json` — aggregated metrics across all projects.
- `roady org policy` — shared policy inheritance; org defaults
  cascade, projects override per file.
- `roady org drift` — cross-project drift aggregation.

### Cross-repo dependency graph

- `roady deps add` / `roady deps list` — declare runtime, build, or
  data dependencies between repos.
- `roady deps graph` — render the dependency graph (with cycle
  detection).
- `roady deps scan` — health check across the dependency graph.

### Plugin system

- HashiCorp `go-plugin`-based syncer plugins. Examples:
  `roady-plugin-github`, `roady-plugin-jira`, `roady-plugin-linear`,
  `roady-plugin-asana`, `roady-plugin-notion`, `roady-plugin-trello`.
- Contract testing via `pkg/plugin/contract`.
- Registry + health monitoring (`roady plugin list|status|validate`).

### Audit + compliance

- Hash-chained `events.jsonl` immutable event log.
- `roady audit verify` to validate the chain.
- Live event handlers (logging, drift warnings, task transitions)
  registered via `EventDispatcher`.

---

## MCP integration

40+ MCP tools across all of the above (see `roady openapi` for the
full schema). One-command setup for every supported tool:

```bash
roady setup claude-code
roady setup claude-desktop
roady setup opencode
roady setup openai
roady setup gemini
```

Run the MCP server in any transport:

```bash
roady mcp                              # stdio (default — Claude Code, OpenCode)
roady mcp --transport http --addr :8080
roady mcp --transport ws   --addr :8080
```

AI-backed MCP tools forward streamed tokens as MCP progress
notifications when the client subscribes via a progress token.

---

## Architecture

Domain-Driven Design layout:

- `pkg/domain/` — pure business logic, no I/O dependencies.
- `pkg/application/` — orchestration services.
- `internal/infrastructure/` — adapters (CLI, MCP, storage,
  notifications).
- `pkg/storage/` — file repository over `.roady/`.
- `evals/` — regression test corpus over the planning pipeline.

Stack: `cobra`, `bubbletea`, `mcp-go`, `fortify`, `statekit`,
`go-plugin`. Web dashboard apps: Vue 3 + D3 compiled via Vite.

See the existing `docs/ddd-*.md` files for the deep architecture write-up.
