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

Every CLI command that works on a project has an MCP tool: one tool per CLI
noun, with the CLI verbs as its `action`. Fourteen tools, under 4k tokens of
every agent's prompt.

| Tool | Actions (CLI) |
|------|---------------|
| `roady_next` | — the task in progress, or the one to start: why it exists (doc:line), what done means, dependencies |
| `roady_capture` | — goals, features, requirements and tasks in one write, from one task to a whole plan (`roady capture`, `add`, `edit`, `split`, `move`) |
| `roady_task` | `start` `complete` `block` `unblock` `stop` `reopen` `verify` `check` `dispatch` `list` `renew` — start claims the task with an expiring lease that `roady_next` renews; `block` with `reason: spec-conflict` or `cannot-complete` hands work that cannot be done as specified to a person |
| `roady_plan` | `get` `generate` `import` `prioritize` `decompose` · **`approve` `reject` `prune`** |
| `roady_spec` | `get` `add` `explain` `review` `validate` · **`analyze` `import` `lock`** |
| `roady_drift` | `detect` (`checks: true` re-runs verified tasks' checks) `explain` `semantic` `record` · **`accept`** |
| `roady_state` | `get` · **`rebuild`** |
| `roady_audit` | `verify` `trail` |
| `roady_goal` | `list` `add` `edit` `render` (ROADMAP.md; replacing a hand edit needs the user) — the roadmap: goals on now, next or later, shipped or out of scope, with the features serving them |
| `roady_policy` | `check` |
| `roady_git` | `sync` |
| `roady_status` | — progress and tasks; `snapshot: true` for the task ids in each lifecycle bucket |
| `roady_query` | — project context for a question, for your model to answer |
| `roady_init` | — a new project |

### Decisions need the user

The actions in bold are decisions: approving or rejecting a plan, pruning it,
accepting drift, re-baselining the spec, rebuilding state. An agent may ask
for them, but they run only when **the user** says yes. Roady asks the user
directly in their client (MCP elicitation), and the confirmation is recorded
in the audit log as `approval.confirmed` next to the operation it allowed.

If the user declines or cancels — or the client cannot ask — nothing changes,
and the agent gets a refusal naming the CLI command (`roady plan approve`) so
the decision still reaches a person. An agent can never approve its own plan
or accept its own drift on its own say-so.

`TestEveryCLICommandHasAnMCPTool` walks the CLI and fails when a command has
no MCP tool or action; `TestGatedActionsNeedTheUser` pins the gate. Host
commands — `setup`, `hook`, `mcp`, `completion`, `config`, `doctor` — are
about the machine, not the project, and stay CLI-only.

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

### roady_task
```json
{
  "action": "start",          // start|complete|block|unblock|stop|reopen|verify|check|dispatch|list
  "task_id": "task-jwt",
  "evidence": "commit-sha"    // complete/verify: proof of completion
}
```

### roady_plan (a gated action)
```json
{"action": "approve"}
```
The user is asked in their client; the plan is approved only on their yes.

---

## Example Workflow

```python
# What am I doing, and what does done mean?
brief = await mcp.call("roady_next")

# Record the plan the agent made (all or nothing, idempotent)
await mcp.call("roady_capture", {"features": [...], "tasks": [...]})

await mcp.call("roady_plan", {"action": "approve"})   # the user is asked; runs only on their yes

await mcp.call("roady_task", {"action": "start", "task_id": "task-jwt"})
# ... implement, commit with [roady:task-jwt] ...
await mcp.call("roady_git", {"action": "sync"})
await mcp.call("roady_task", {"action": "check", "task_id": "task-jwt"})
await mcp.call("roady_task", {"action": "complete", "task_id": "task-jwt", "evidence": "abc123"})

# Has reality diverged from intent?
drift = await mcp.call("roady_drift", {"action": "detect"})
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
3. **Run the acceptance check** (`roady_task` action `check`) before calling a task done
4. **Provide evidence** when completing tasks, and commit with `[roady:task-id]`
5. **Let the user decide** — approval, drift acceptance and re-baselining run only on the user's confirmation

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
