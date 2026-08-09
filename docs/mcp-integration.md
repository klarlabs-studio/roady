# AI Tool Integration Guide

Roady provides a unified planning layer for any AI coding tool via the Model Context Protocol (MCP). This guide covers integration with Claude Code, OpenCode, Claude Desktop, OpenAI Codex, and Google Gemini.

## Why Use Roady as Your Planning Layer?

| Built-in Tasks | Roady |
|----------------|-------|
| Lost on context reset | Survives resets (`.roady/` on disk) |
| No spec tracking | Spec → Plan → Execution pipeline |
| No drift detection | Intent/Plan/Code/Policy drift detection |
| No cross-session memory | Durable, git-versioned state |
| Single-user | Team, billing, dependencies |
| Tool-specific | Works with any MCP-compatible AI |

## One-Command Setup

```bash
# Claude Code CLI
roady setup claude-code

# OpenCode
roady setup opencode

# Claude Desktop
roady setup claude-desktop

# OpenAI Codex
roady setup openai

# Google Gemini
roady setup gemini

# All platforms (commands + MCP config)
roady setup global
```

## Claude Code

### Setup
```bash
roady setup claude-code
```

This installs:
- Custom commands (`/roady-task`, `/roady-status`, `/roady-review`)
- MCP server configuration
- CLAUDE.md instructions

### Usage
```
/roady-task              # Start next ready task
# Claude implements the task
/roady-review            # Check for drift
```

### Manual Configuration

**Commands:** Copy from `.claude/commands/` to `~/.claude/commands/`

**MCP:** Add to `~/.claude/settings.local.json`:
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

**CLAUDE.md:** Add task management instructions

## OpenCode

### Setup
```bash
roady setup opencode
```

### Manual Configuration

Add to `~/.opencode/config.json`:
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

### Usage
```
/roady-task              # Start next ready task
/roady-status           # Check project status
```

## Claude Desktop

### Setup
```bash
roady setup claude-desktop
```

### Manual Configuration

Edit `~/Library/Application Support/Claude/claude_desktop_config.json`:
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

## OpenAI Codex

### Setup
```bash
roady setup openai
```

### Python Integration

```python
from agents import Agent
import subprocess

# Start Roady MCP server
roady_process = subprocess.Popen(
    ["roady", "mcp", "--transport", "stdio"],
    stdout=subprocess.PIPE,
    stdin=subprocess.PIPE,
)

# Use with Codex agent
agent = Agent(
    name="Developer",
    mcp_servers=[roady_process],
)

# Now the agent can use:
# - roady_plan_get
# - roady_get_ready_tasks
# - roady_task_transition
# - roady_drift_detect
```

## Google Gemini

### Setup
```bash
roady setup gemini
```

### Configuration

Via Google AI Studio or Vertex AI Agent Builder:
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

Note: Gemini MCP support varies by platform.

## MCP Tools Reference

All platforms have access to 40+ MCP tools:

### Planning
| Tool | Description |
|------|-------------|
| `roady_spec_get` | Get current specification |
| `roady_plan_get` | Get task list with dependencies |
| `roady_plan_generate` | Generate plan from spec |
| `roady_plan_approve` | Approve plan for execution |
| `roady_plan_update` | Smart injection of tasks |

### Execution
| Tool | Description |
|------|-------------|
| `roady_get_ready_tasks` | Tasks ready to start |
| `roady_task_transition` | Start/complete/block tasks |
| `roady_task_assign` | Assign tasks |

### Verification
| Tool | Description |
|------|-------------|
| `roady_drift_detect` | Check implementation vs plan |
| `roady_drift_explain` | AI explanation of drift |
| `roady_drift_accept` | Lock spec snapshot |

### Analysis
| Tool | Description |
|------|-------------|
| `roady_status` | Project status overview |
| `roady_forecast` | Completion predictions |
| `roady_debt_report` | Planning debt analysis |
| `roady_spec_explain` | AI architectural overview |

## Workflow Example

### 1. Plan (Human)

```bash
roady init my-project
roady spec add "User Authentication" "Implement JWT login/logout"
roady plan generate --ai
roady plan approve
```

### 2. Execute (AI Tool)

```
/roady-task
# AI implements the task
/roady-review
```

### 3. Verify (Human or AI)

```bash
roady drift detect
roady task complete task-user-auth
git commit -m "feat: user auth [roady:task-user-auth]"
```

## MCP Server Options

```bash
# Stdio (Claude Code, OpenCode)
roady mcp

# HTTP (web apps, remote access)
roady mcp --transport http --addr :8080

# WebSocket (real-time)
roady mcp --transport ws --addr :8080
```

## Tips

1. **Commit with task IDs**: `git commit -m "feat: ... [roady:task-id]"`
2. **Check drift before starting**: `roady drift detect`
3. **Sync workspace**: `roady workspace push` for team sharing
4. **Disable built-in tasks**: Set `CLAUDE_CODE_ENABLE_TASKS=false` to prevent conflicts
