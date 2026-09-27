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

The server carries the agent loop and nothing else — ten tools, about 3k
tokens of every agent's prompt:

| Tool | Does |
|------|------|
| `roady_next` | The task in progress, or the one to start: why it exists (doc:line), what done means, dependencies |
| `roady_capture` | Record features, requirements and tasks in one write, from one task to a whole plan |
| `roady_plan_import` | Import a plan file an agent wrote (markdown, Kiro `tasks.md`, Codex ExecPlan) as tasks |
| `roady_task_transition` | start, complete, block, unblock, stop, reopen, verify |
| `roady_task_check` | Run a task's acceptance check and record the result |
| `roady_task_dispatch` | Hand a ready task to a subagent with its intent and completion contract |
| `roady_status` | Progress and tasks, filterable by ready, blocked, active, status and priority |
| `roady_query` | Project context for a question, for your model to answer |
| `roady_drift_detect` | Drift between spec, plan, code and policy; `semantic: true` returns the semantic-drift prompt |
| `roady_drift_record_semantic` | Record the judgements on that prompt |

Everything else is a CLI command for a person: `roady init`, `roady plan
approve|reject|prune|generate`, `roady drift accept|explain`, `roady spec
lock|validate|import|analyze|explain|review`, `roady state rebuild`, `roady
audit verify|trail`, `roady git sync`, `roady policy check`. In particular an
agent cannot approve its own plan or accept its own drift — deciding what was
agreed is not the agent's call.

---

## Tool Parameters

### roady_capture
```json
{
  "features": [
    {"id": "auth", "title": "Authentication", "requirements": [
      {"id": "jwt", "title": "JWT sessions", "priority": "high",
       "check": {"run": "go test ./auth -run TestJWT"}}
    ]}
  ],
  "tasks": [
    {"id": "task-auth-docs", "title": "Document the login flow",
     "requirement": "jwt", "depends_on": ["task-jwt"]}
  ],
  "dry_run": false
}
```

### roady_task_transition
```json
{
  "task_id": "task-jwt",
  "event": "start",           // start|complete|block|unblock|stop|reopen|verify
  "evidence": "commit-sha"    // optional: proof of completion
}
```

---

## Example Workflow

```python
# What am I doing, and what does done mean?
brief = await mcp.call("roady_next")

# Record the plan the agent made (all or nothing, idempotent)
await mcp.call("roady_capture", {"features": [...], "tasks": [...]})
# ...a person approves new intent: `roady plan approve`

await mcp.call("roady_task_transition", {"task_id": "task-jwt", "event": "start"})
# ... implement, commit with [roady:task-jwt], `roady git sync` ...
await mcp.call("roady_task_check", {"task_id": "task-jwt"})
await mcp.call("roady_task_transition", {"task_id": "task-jwt", "event": "complete", "evidence": "abc123"})

# Has reality diverged from intent?
drift = await mcp.call("roady_drift_detect")
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

1. **Start from `roady_next`** at the beginning of a session and after compaction
2. **Record new work with `roady_capture`**, never in a markdown file
3. **Run the acceptance check** (`roady_task_check`) before calling a task done
4. **Provide evidence** when completing tasks, and commit with `[roady:task-id]`
5. **Leave approval and drift acceptance to a person** — the CLI is where they happen

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
