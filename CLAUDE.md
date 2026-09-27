# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Roady is a planning-first system of record for software work. It acts as a durable memory layer between **intent** (specs), **plans** (task DAGs), and **execution** (state tracking). Built for AI coding agents (via MCP and CLI) and the people who direct them.
The product is three things: **capture** intent at any size, **keep** the
agent on it across sessions and compaction, and **prove** work done with
acceptance checks and a hash-chained audit log. Anything that does not serve
those was removed (billing, teams/org, tracker sync, messaging, debt,
forecasting, dashboards); do not reintroduce it.

## Build & Test Commands

```bash
# Build main binary
go build -o roady ./cmd/roady

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
├── policy/          # Policy rules (WIP limits, evidence, approval mode)
├── audit/ events/ provenance/  # Hash-chained log, projections, who-did-what
└── project/ dispatch/ prompt/  # Coordinator, subagent dispatch, prompt building

pkg/application/      # Use-case services orchestrating domain logic
├── capture_service.go   # one write for intent of any size (capture, add/edit/split/move)
├── plan_import.go       # harness plan files -> tasks
├── brief.go             # roady next
├── check_service.go     # acceptance checks; evidence_gate.go, check_guard.go
├── plan_service.go / task_service.go / spec_service.go / drift_service.go
├── policy_service.go / git_service.go / dispatch_service.go
└── audit_service.go / audit_trail_service.go / prompt_service.go
```

```
internal/infrastructure/  # Adapters and framework integrations
├── cli/             # Cobra CLI commands, agent hooks (hook.go) and setup
├── mcp/             # MCP server implementation
└── wiring/          # Service composition and dependency injection

pkg/storage/         # Filesystem repository (YAML/JSON in .roady/)
pkg/sdk/             # Go client for the MCP server
```

### Key Dependencies

- **cobra**: CLI framework
- **go.klarlabs.de/mcp**: MCP server protocol
- **statekit**: FSM for task state transitions
- **fortify**: Resilience (retry, timeout) for AI calls

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

The MCP server lives in `internal/infrastructure/mcp/`. For the current set
of tools, ask the code rather than a list that goes stale:

```bash
grep -rhoE '"roady_[a-z_]+"' internal/infrastructure/mcp/*.go | tr -d '"' | sort -u
```

Run MCP server:
```bash
roady mcp                          # stdio (default)
roady mcp --transport http --addr :8080
roady mcp --transport ws --addr :8080
```

#### The surface is the agent loop

The server has ten tools — `roady_next`, `roady_capture`, `roady_plan_import`,
`roady_task_transition`, `roady_task_check`, `roady_task_dispatch`,
`roady_status`, `roady_query`, `roady_drift_detect` (with `semantic` for the
semantic-drift prompt) and `roady_drift_record_semantic` — about 3k tokens of
every agent's prompt. Everything else is a CLI command, and on purpose:
approving or rejecting a plan, accepting drift, re-locking the spec, pruning,
init, rebuild and audit are decisions or maintenance for a person, and an agent
that could approve its own plan would make approval meaningless.
`TestServer_ServesTheAgentLoopOnly` pins the exact set, and
`TestServer_GovernanceIsNotAnAgentTool` keeps governance off it. Add a tool
only when the loop cannot work without it.

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
1. Check current plan: roady status — or just `roady next` for a brief of
   the task you are on (or should start next)
2. Get next task: roady task ready
3. Start task: roady task start <task-id>
4. Complete task: roady task complete <task-id>
5. Run its acceptance check: roady task check <task-id>
   (verify re-runs it and refuses on failure; see docs/acceptance-checks.md)
6. Check drift: roady drift detect

When planning new work:
1. Review spec: roady spec explain
2. Record the plan in one write: roady capture -f plan.yaml (docs/capture.md)
   — or generate tasks: roady plan generate --ai  # emits a prompt; you run it
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

For projects with Roady MCP configured, the agent works through `roady_next`,
`roady_capture`, `roady_task_transition` and `roady_task_check`; see
`docs/mcp-guide.md` for all ten.

Roady's MCP server works with Claude Code, OpenCode, Claude Desktop, OpenAI Codex, and Google Gemini. Use `roady setup <platform>` to configure.

