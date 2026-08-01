# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Roady is a planning-first system of record for software work. It acts as a durable memory layer between **intent** (specs), **plans** (task DAGs), and **execution** (state tracking). Designed for individuals, teams, and AI agents via MCP (Model Context Protocol).

## Build & Test Commands

```bash
# Build main binary
go build -o roady ./cmd/roady

# Build plugin binaries
go build -o roady-plugin-mock ./cmd/roady-plugin-mock
go build -o roady-plugin-github ./cmd/roady-plugin-github
go build -o roady-plugin-jira ./cmd/roady-plugin-jira
go build -o roady-plugin-linear ./cmd/roady-plugin-linear

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
- **mcp-go**: MCP server protocol
- **statekit**: FSM for task state transitions
- **fortify**: Resilience (retry, timeout) for AI calls
- **go-plugin**: HashiCorp plugin system for external syncers

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

The MCP server (`internal/infrastructure/mcp/server.go`) exposes these tools:
- `roady_init`, `roady_get_spec`, `roady_get_plan`, `roady_get_state`
- `roady_generate_plan`, `roady_update_plan`, `roady_approve_plan`
- `roady_detect_drift`, `roady_accept_drift`, `roady_explain_drift`
- `roady_transition_task`, `roady_check_policy`, `roady_status`
- `roady_forecast`, `roady_git_sync`, `roady_sync`

Run MCP server:
```bash
roady mcp                          # stdio (default)
roady mcp --transport http --addr :8080
roady mcp --transport ws --addr :8080
```

### Plugin System

Plugins use HashiCorp go-plugin over RPC:
- Interface: `pkg/domain/plugin/Syncer`
- Loader: `pkg/plugin/loader.go`
- Examples: `cmd/roady-plugin-github`, `cmd/roady-plugin-jira`, `cmd/roady-plugin-linear`

## Common Workflows

```bash
# Initialize project
roady init my-project

# Analyze docs and generate spec
roady spec analyze docs/

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
- `roady_get_plan` - Fetch current plan with ready tasks
- `roady_transition_task` - Start/complete tasks
- `roady_detect_drift` - Check implementation vs plan
- `roady_get_snapshot` - Get full project state

Roady's MCP server works with Claude Code, OpenCode, Claude Desktop, OpenAI Codex, and Google Gemini. Use `roady setup <platform>` to configure.

