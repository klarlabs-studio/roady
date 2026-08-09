# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Roady is a planning-first system of record for software work. It acts as a durable memory layer between **intent** (specs), **plans** (task DAGs), and **execution** (state tracking). Designed for individuals, teams, and AI agents via MCP (Model Context Protocol).

## Build & Test Commands

```bash
# Build main binary
go build -o roady ./cmd/roady

# Build every plugin binary (asana, github, jira, linear, mock, notion, trello).
# Enumerated rather than listed, so this does not go stale as plugins are added.
for p in cmd/roady-plugin-*; do go build -o "$(basename "$p")" "./$p"; done

# Run all tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...

# Run a single test
go test -run TestFunctionName ./path/to/package

# Run tests for a specific package
go test ./pkg/application/...
go test ./internal/infrastructure/cli/...

# Verbose test output
go test -v ./...
```

## Architecture

### Domain-Driven Design Structure

```
pkg/domain/           # Pure domain logic (no external dependencies)
├── spec/            # ProductSpec, Feature, Requirement entities
├── planning/        # Plan, Task, ExecutionState, DAG validation
├── drift/           # Issue, Report, drift detection types
├── policy/          # Policy rules (WIP limits, dependencies)
└── plugin/          # Syncer interface for external integrations

pkg/application/      # Use-case services orchestrating domain logic
├── init_service.go
├── spec_service.go
├── plan_service.go
├── drift_service.go
├── policy_service.go
├── task_service.go
├── audit_service.go
├── prompt_service.go
├── report_service.go
├── audit_trail_service.go
├── git_service.go
└── sync_service.go

internal/infrastructure/  # Adapters and framework integrations
├── cli/             # Cobra CLI commands (root, init, spec, plan, drift, etc.)
├── mcp/             # MCP server implementation
└── wiring/          # Service composition and dependency injection

pkg/storage/         # Filesystem repository (YAML/JSON in .roady/)
pkg/plugin/          # HashiCorp go-plugin loader for external syncers
```

### Key Dependencies

- **cobra**: CLI framework
- **bubbletea/lipgloss**: TUI dashboard
- **go.klarlabs.de/mcp**: MCP server protocol
- **statekit**: FSM for task state transitions
- **fortify**: Resilience (retry, timeout) for AI calls
- **go-plugin**: HashiCorp plugin system for external syncers

`go.mod` is authoritative; this list names what each is for, not what version
is pinned.

### Data Storage (.roady/)

All artifacts are git-friendly files:
- `spec.yaml` - Product specification (features, requirements)
- `spec.lock.json` - Pinned spec snapshot for drift detection
- `plan.json` - Task DAG with approval status
- `state.json` - Execution state (task statuses, paths)
- `policy.yaml` - Governance (max_wip, allow_ai, token_limit)
- `events.jsonl` - Immutable audit trail (hash-chained)

### Service Wiring

Services are composed via `internal/infrastructure/wiring`:
- `BuildAppServices(root)` returns all services with shared dependencies
- CLI and MCP share the same service instances
- `AuditService` is injected into all services for event logging

### Roady runs no inference

Roady does not call language models and needs no API key. `PromptService`
(`pkg/application/prompt_service.go`) assembles the context a model needs and
returns a `prompt.Request` — the caller, which already has a model, runs the
inference and writes results back through the named tool. See
`docs/prompts.md`.

### Task State Machine

Tasks follow strict FSM transitions via statekit:
```
pending → in_progress → done → verified
            ↓     ↑
         blocked
```

Guards enforce:
- WIP limits (policy.max_wip)
- Dependency completion before start
- Plan approval before execution

### MCP Tools

The MCP server lives in `internal/infrastructure/mcp/`. It currently exposes
about seventy tools, grouped roughly as:

- **spec / plan / state** — `roady_spec_get`, `roady_plan_get`, `roady_state_get`,
  `roady_plan_generate`, `roady_plan_approve`, `roady_spec_add`
- **drift** — `roady_drift_detect`, `roady_drift_accept`, `roady_drift_explain`
- **tasks** — `roady_task_transition`, `roady_tasks`, `roady_task_assign`
- **governance & audit** — `roady_policy_check`, `roady_audit_trail`, `roady_audit_verify`
- **cost, debt, deps, org, team, rates** — families prefixed `roady_cost_`,
  `roady_debt_`, `roady_deps_`, `roady_org_`, `roady_team_`, `roady_rate_`

This deliberately does not enumerate them. An earlier version listed sixteen
by name; the server had grown to seventy and every one of the sixteen was still
correct, so the list was not wrong — just quietly four-fifths incomplete, which
reads the same as complete. For the current set, ask the code:

```bash
grep -rhoE '"roady_[a-z_]+"' internal/infrastructure/mcp/*.go | tr -d '"' | sort -u
```

Run MCP server:
```bash
roady mcp                          # stdio (default)
roady mcp --transport http --addr :8080
roady mcp --transport ws --addr :8080
```

#### Trimming the advertised surface

All seventy tools are advertised on every session, and a client pays for each
one in its prompt whether or not the project has a rate card or a debt ledger.
`ROADY_MCP_TOOLS` selects which groups are registered:

```bash
ROADY_MCP_TOOLS=core roady mcp        # 30 tools instead of 70
ROADY_MCP_TOOLS=core,debt roady mcp   # plus the debt ledger
```

Groups: `core`, `cost`, `team`, `org`, `debt`, `deps`, `plugin`, `sync`,
`analytics`, `audit`. `core` is always included — a server without the
spec/plan/execute loop cannot do the thing roady is for. Unset (or `all`)
registers everything, so this is opt-in: an existing client keeps the surface
it already calls. An unknown group name fails startup rather than quietly
starting a smaller server.

The grouping lives in `internal/infrastructure/mcp/profiles.go`, and a tool
missing from it fails the build — the same guarantee the behaviour
annotations have. Otherwise an unclassified tool would silently disappear
from every profile, which looks exactly like a tool that does not exist.

### Plugin System

Plugins use HashiCorp go-plugin over RPC:
- Interface: `pkg/domain/plugin/Syncer`
- Loader: `pkg/plugin/loader.go`
- Implementations: `cmd/roady-plugin-*` — asana, github, jira, linear, mock,
  notion, trello

## Common Workflows

```bash
# Initialize project
roady init my-project

# Analyze docs and generate spec
roady spec analyze docs/

# Re-capture the drift baseline after replacing the generated spec.
# `roady init` writes spec.yaml, spec.lock.json and state.json together, so
# they agree. Adopting roady in an existing project means replacing spec.yaml —
# and nothing re-derives the other two, so drift is then measured against a
# spec the project never had while `spec validate` still answers "valid".
roady spec lock

# Generate plan (heuristic or AI)
roady plan generate
roady plan generate --ai      # emits a prompt; you run it

# Check drift and accept if intentional
roady drift detect
roady drift accept

# Task lifecycle
roady task start <task-id>
roady task complete <task-id>

# Git-based sync
git commit -m "Implement feature [roady:task-id]"
roady git sync
```

## Testing Patterns

- Unit tests alongside source files (`*_test.go`)
- Table-driven tests preferred
- Test helpers in `internal/infrastructure/cli/test_helpers_test.go`

## Claude Code Integration

When running Claude Code in this project, use Roady for all task management instead of Claude Code's built-in Task tools.

### Why Roady over Claude Code Tasks?

- **Durable**: Tasks survive context resets, stored in `.roady/` (git-versioned)
- **Traceable**: Spec → Plan → Execution with drift detection
- **Collaborative**: Works across sessions, users, and AI agents
- **Audit-ready**: Hash-chained event log for compliance

### Workflow

```markdown
## Task Management

When working on features:
1. Check current plan: roady status
2. Get next task: roady task ready
3. Start task: roady task start <task-id>
4. Complete task: roady task complete <task-id>
5. Check drift: roady drift detect

When planning new work:
1. Review spec: roady spec explain
2. Generate tasks: roady plan generate --ai      # emits a prompt; you run it
3. Approve plan: roady plan approve

Never use Claude's TaskWrite/TaskCreate/TaskUpdate tools.
Use CLI commands instead.
```

### Custom Commands

See `.claude/commands/` for pre-configured Claude Code commands:
- `/roady-task` - Start next ready task
- `/roady-status` - Full project status
- `/roady-review` - Check for drift

### MCP Server

For projects with Roady MCP configured, these tools are available:
- `roady_plan_get` - Fetch current plan with ready tasks
- `roady_task_transition` - Start/complete tasks
- `roady_drift_detect` - Check implementation vs plan
- `roady_snapshot_get` - Get full project state

Roady's MCP server works with Claude Code, OpenCode, Claude Desktop, OpenAI Codex, and Google Gemini. Use `roady setup <platform>` to configure.

