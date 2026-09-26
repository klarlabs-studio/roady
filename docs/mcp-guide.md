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

By default the server **lists** seven essential tools and keeps every other
tool registered and callable by name. Set `ROADY_MCP_TOOLS=all` to list
everything.

### Essential Tools (listed by default)

| Tool | Description |
|------|-------------|
| `roady_next` | Brief for the task in progress, or the one to start next |
| `roady_capture` | Record intent of any size in one write |
| `roady_plan_import` | Import an agent's plan file as roady tasks |
| `roady_task_transition` | Transition task state (start/complete/block/stop/unblock/verify) |
| `roady_task_check` | Run a task's acceptance check |
| `roady_status` | High-level project summary |
| `roady_query` | Ask a question about the plan and state |

### Spec, Plan & State Tools

| Tool | Description |
|------|-------------|
| `roady_init` | Initialize a new roady project |
| `roady_spec_get` / `roady_spec_add` / `roady_spec_import` / `roady_spec_analyze` | Read, extend, or import the spec |
| `roady_spec_explain` / `roady_spec_review` / `roady_spec_validate` / `roady_spec_lock` | Explain, review, validate, or lock the spec |
| `roady_plan_get` / `roady_plan_generate` / `roady_plan_update` | Read or build the plan |
| `roady_plan_approve` / `roady_plan_reject` / `roady_plan_prune` | Plan approval lifecycle |
| `roady_plan_decompose` / `roady_plan_prioritize` | Break down or reorder tasks |
| `roady_state_get` / `roady_state_rebuild` / `roady_snapshot_get` | Execution state and full snapshot |
| `roady_tasks` / `roady_task_dispatch` | List tasks, dispatch work |

### Drift, Governance & Audit Tools

| Tool | Description |
|------|-------------|
| `roady_drift_detect` / `roady_drift_explain` / `roady_drift_accept` | Detect, explain, or accept drift |
| `roady_semantic_drift` / `roady_drift_record_semantic` | Semantic drift prompt and result |
| `roady_policy_check` | Validate against WIP limits and policy |
| `roady_audit_trail` / `roady_audit_verify` | Read and verify the hash-chained audit log |
| `roady_git_sync` | Sync task state from `[roady:<task-id>]` commit markers |

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

# Get a brief for the next task
brief = await mcp.call("roady_next")
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
3. **Run acceptance checks** (`roady_task_check`) before verifying tasks
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
