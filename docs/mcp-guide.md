# Roady MCP (Model Context Protocol) Guide

This guide documents how to integrate Roady with AI agents via the Model Context Protocol (MCP).

## Overview

Roady is a first-class MCP server that exposes deterministic project state, planning capabilities, and drift analysis to AI agents. All MCP tools share the same service layer as the CLI, ensuring consistent behavior and audit trails.

## Transport Options

Roady supports three MCP transport modes:

### 1. stdio (Default)

The standard transport for local AI tool integration. Recommended for Claude Desktop and similar applications.

```bash
# Start in stdio mode (default)
roady mcp
```

**Claude Desktop Configuration** (`~/.config/claude/claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "roady": {
      "command": "roady",
      "args": ["mcp"]
    }
  }
}
```

### 2. HTTP

RESTful transport for web integrations and remote agents.

```bash
# Start HTTP server on port 8080
roady mcp --transport http --addr :8080

# Custom port
roady mcp --transport http --addr :3000
```

**Use Cases:**
- Remote AI agents accessing a central planning server
- Web-based dashboards
- CI/CD pipeline integrations
- Multi-project orchestration

### 3. WebSocket

Bidirectional transport for real-time streaming and long-running sessions.

```bash
# Start WebSocket server
roady mcp --transport ws --addr :8080
```

**Use Cases:**
- Interactive AI assistants requiring real-time updates
- Streaming drift detection results
- Long-running planning sessions with progress updates

---

## Available Tools

### Core State Tools

| Tool | Description | Returns |
|------|-------------|---------|
| `roady_init` | Initialize a new roady project | Confirmation message |
| `roady_spec_get` | Retrieve the current product specification | JSON ProductSpec |
| `roady_plan_get` | Retrieve the current execution plan | JSON Plan with tasks |
| `roady_state_get` | Retrieve task execution states | JSON ExecutionState |
| `roady_status` | Get a high-level project summary | Status summary text |

### Planning Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `roady_plan_generate` | Generate plan using 1:1 heuristic | None |
| `roady_plan_update` | Update with specific task list | `tasks[]` - Task definitions |
| `roady_plan_approve` | Approve plan for execution | None |
| `roady_spec_explain` | AI architectural walkthrough | None |

### Drift Detection Tools

| Tool | Description | Returns |
|------|-------------|---------|
| `roady_drift_detect` | Detect spec/plan discrepancies | DriftReport JSON |
| `roady_drift_accept` | Accept drift, lock spec snapshot | Confirmation |
| `roady_drift_explain` | AI explanation of drift causes | Analysis text |

### Task Lifecycle Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `roady_task_transition` | Transition task state | `task_id`, `event` (start/complete/block/stop), optional `evidence` |
| `roady_policy_check` | Validate against WIP limits | None |

### Forecasting & Analytics Tools

| Tool | Description | Returns |
|------|-------------|---------|
| `roady_forecast` | Predict completion based on velocity | Velocity, remaining tasks, estimated days |
| `roady_usage_get` | Get AI token consumption stats | UsageStats JSON |

### Dependency Management Tools (Horizon 5)

| Tool | Description | Returns |
|------|-------------|---------|
| `roady_deps_list` | List cross-repo dependencies | Dependencies JSON |
| `roady_deps_scan` | Scan dependent repo health | Health status |
| `roady_deps_graph` | Get dependency graph | Graph with cycle detection |

### Debt Analysis Tools (Horizon 5)

| Tool | Description | Returns |
|------|-------------|---------|
| `roady_debt_report` | Comprehensive debt analysis | DebtReport JSON |
| `roady_debt_summary` | Quick debt overview | Summary text |
| `roady_sticky_drift` | Items unresolved >7 days | Sticky items list |
| `roady_debt_trend` | Drift trend over time | Trend analysis |

### Integration Tools

| Tool | Description | Parameters |
|------|-------------|------------|
| `roady_spec_add` | Add feature to spec | `title`, `description` |
| `roady_git_sync` | Sync via commit markers | None |
| `roady_sync` | External plugin sync | `plugin_path` |
| `roady_org_status` | Multi-project overview | None |

---

## Tool Parameters

### roady_init
```json
{
  "name": "my-project"  // Optional: project name
}
```

### roady_plan_update
```json
{
  "tasks": [
    {
      "id": "task-auth",
      "title": "Implement authentication",
      "description": "Add JWT-based auth",
      "feature_id": "feat-security",
      "depends_on": [],
      "priority": "high",
      "estimate": "3d"
    }
  ]
}
```

### roady_task_transition
```json
{
  "task_id": "task-auth",
  "event": "start",           // start|complete|block|stop|unblock|verify
  "evidence": "commit-sha"    // Optional: proof of completion
}
```

### roady_spec_add
```json
{
  "title": "User Dashboard",
  "description": "A comprehensive dashboard showing user metrics and activity"
}
```

---

## Example Workflows

### 1. Initial Project Setup (AI Agent)

```python
# 1. Initialize project
await mcp.call("roady_init", {"name": "my-app"})

# 2. Generate initial plan from existing spec
await mcp.call("roady_plan_generate")

# 3. Review and approve
plan = await mcp.call("roady_plan_get")
# ... agent reviews plan ...
await mcp.call("roady_plan_approve")
```

### 2. Task Execution Loop

```python
# Check policy before starting
policy_ok = await mcp.call("roady_policy_check")

# Start task
await mcp.call("roady_task_transition", {
    "task_id": "task-api",
    "event": "start"
})

# ... agent implements feature ...

# Complete with evidence
await mcp.call("roady_task_transition", {
    "task_id": "task-api",
    "event": "complete",
    "evidence": "PR #123"
})
```

### 3. Drift Detection & Resolution

```python
# Detect drift
drift = await mcp.call("roady_drift_detect")

if drift["has_issues"]:
    # Get AI explanation
    explanation = await mcp.call("roady_drift_explain")

    # If drift is intentional, accept it
    await mcp.call("roady_drift_accept")
```

### 4. Progress Monitoring

```python
# Get current status
status = await mcp.call("roady_status")

# Get velocity forecast
forecast = await mcp.call("roady_forecast")

# Check debt status
debt = await mcp.call("roady_debt_summary")
```

---

## Governance & Audit

All MCP tool invocations are logged to `.roady/events.jsonl` with:
- **Action**: The operation performed (e.g., `plan.approved`)
- **Actor**: `ai` for MCP calls, `cli` for command-line
- **Metadata**: Context-specific data (task IDs, spec hashes)
- **Timestamp**: ISO 8601 timestamp
- **Hash Chain**: Cryptographic verification

Example event:
```json
{
  "id": "evt-123",
  "action": "task.started",
  "actor": "ai",
  "metadata": {"task_id": "task-api", "owner": "claude"},
  "prev_hash": "abc...",
  "hash": "def...",
  "timestamp": "2025-01-15T10:30:00Z"
}
```

---

## Best Practices

1. **Always check policy** before starting tasks to respect WIP limits
2. **Provide evidence** when completing tasks for audit trails
3. **Monitor debt** periodically to catch sticky drift early
4. **Use git sync** after commits with `[roady:task-id]` markers
5. **Accept drift explicitly** rather than ignoring discrepancies

---

## Environment Variables

Roady calls no language model, so there is no provider or API key to configure.
The `ROADY_AI_PROVIDER` and `*_API_KEY` variables were removed in v0.15.0.

What an MCP session does read:

```bash
export ROADY_AGENT=claude-code            # who is acting, for the audit trail
export ROADY_SESSION_ID=run-7             # groups a conversation's events
export ROADY_USER="Ada Lovelace"          # task ownership
```

Agent and session are detected automatically for Claude Code, Cursor, Codex and
Gemini CLI when unset; setting them explicitly overrides the detection. See
`docs/audit-grc.md` for what a trail attests, and `docs/prompts.md` for the
operations that hand you a prompt instead of running one.
