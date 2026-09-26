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
- MCP server configuration (`.mcp.json`)
- Hooks (`.claude/settings.json`) — see [Hooks](#hooks)
- A marked block in `CLAUDE.md` and the `roady-planning` skill — see
  [Instructions](#instructions)

### Instructions

Tools and hooks make roady available; the instructions tell the agent to use
it rather than the markdown plan it would otherwise write. Setup keeps a block
between `<!-- roady:begin -->` and `<!-- roady:end -->` in the agent's
instruction file: plans live in roady, `roady next` before work, `roady
capture` / `roady plan import` for new work, never ROADMAP.md / TODO.md /
PLAN.md, and done means the check passes. Re-running setup replaces the block
in place and leaves the rest of the file alone. `opencode` and `openai` write
it to `AGENTS.md`, `gemini` to `GEMINI.md`.

For Claude Code, setup also installs `.claude/skills/roady-planning/SKILL.md`,
which Claude loads when it plans, breaks down work or decides whether a task is
done: the capture format with an example, plan import, and the finish
sequence. The file is roady's and is rewritten when it changes.

`roady setup <target> --no-instructions` skips both.

### Hooks

Agents rarely ask for their plan, and they write plans into whatever markdown
file is at hand. `roady setup claude-code` registers three hooks in the
project's `.claude/settings.json` so neither depends on the agent remembering:

| Event | Matcher | Command | Does |
|---|---|---|---|
| `SessionStart` | (all: startup, resume, clear, compact) | `roady hook session-start` | Puts the `roady next` brief into context — at the start of a session and again after compaction |
| `PostToolUse` | `ExitPlanMode` | `roady hook plan-approved` | Saves the plan you approved in plan mode to `.roady/plans/<title>.md` and imports it as tasks ([plan-import.md](plan-import.md)); tells the agent what landed and whether the roady plan needs `roady plan approve` |
| `PreToolUse` | `Write\|Edit\|MultiEdit` | `roady hook guard-write` | Refuses `ROADMAP*.md`, `TODO*.md` and `plan*.md` / `*-plan.md` files in the project, pointing the agent to `roady capture` |

The guard ignores files outside the project and under `.roady/` or `.claude/`
(Claude Code keeps its own plan files there). To keep a file it would refuse,
list it in `.roady/policy.yaml`:

```yaml
plan_files_allow:
  - ROADMAP.md        # a file name matches anywhere
  - docs/adr/**       # a whole directory
```

Every hook is silent outside a roady project and never fails the agent's
action: if roady errors, the action goes ahead without it. Re-running setup
replaces roady's own entries (commands starting `roady hook `) and leaves
every other hook as it was. Commit `.claude/settings.json` to share them.

### Usage
```
/roady-task              # Start next ready task
# Claude implements the task
/roady-review            # Check for drift
```

### Manual Configuration

**Commands:** Copy from `.claude/commands/` to `~/.claude/commands/`

**MCP:** Claude Code reads MCP servers from the project's `.mcp.json` or from
`~/.claude.json` — not from `settings.json` or `settings.local.json`. For the
project (shared with collaborators, approved once per machine):
```json
{
  "mcpServers": {
    "roady": {
      "type": "stdio",
      "command": "roady",
      "args": ["mcp"]
    }
  }
}
```
Or for every project: `claude mcp add --scope user roady -- roady mcp`.
`claude mcp list` shows whether the server is registered and approved.

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
