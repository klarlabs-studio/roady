# AI Tool Integration Guide

Roady provides a unified planning layer for any AI coding tool via the Model Context Protocol (MCP). This guide covers Claude Code, OpenAI Codex, Gemini CLI, Cursor, OpenCode, GitHub Copilot, Kiro and Claude Desktop.

## Why Use Roady as Your Planning Layer?

| Built-in Tasks | Roady |
|----------------|-------|
| Lost on context reset | Survives resets (`.roady/` on disk) |
| No spec tracking | Spec → Plan → Execution pipeline |
| No drift detection | Intent/Plan/Code/Policy drift detection |
| No cross-session memory | Durable, git-versioned state |
| No proof of done | Acceptance checks, evidence-gated verify, audit log |
| Tool-specific | Works with any MCP-compatible AI |

## One-Command Setup

```bash
roady setup claude-code   # or: codex, gemini, cursor, opencode, copilot, kiro
roady setup all           # every agent above, in one project
```

Each project target writes what that agent supports, merged into existing
files (other servers, hooks and settings are kept; re-running changes
nothing):

| Agent | MCP server | Instructions | Skill | Brief at session start | Brief after compaction | Plan-file guard | Plan-mode import |
|---|---|---|---|---|---|---|---|
| Claude Code | `.mcp.json` | `CLAUDE.md` | `.claude/skills` | ✓ | ✓ | ✓ | ✓ (ExitPlanMode) |
| Codex | `.codex/config.toml` | `AGENTS.md` | `.agents/skills` | ✓ | ✓ | ✓ (apply_patch) | — no plan hook |
| Gemini CLI | `.gemini/settings.json` | `GEMINI.md` | `.agents/skills` | ✓ | — no event | ✓ | ✓ (exit_plan_mode) |
| Cursor | `.cursor/mcp.json` | `AGENTS.md` | `.agents/skills` | ✓ | — | ✓ | — no plan hook |
| OpenCode | `opencode.json` | `AGENTS.md` | `.agents/skills` | — | ✓ (plugin) | ✓ (plugin) | — |
| GitHub Copilot | `.vscode/mcp.json` | `AGENTS.md` | `.agents/skills` | ✓ | — | ✓ | — |
| Kiro | `.kiro/settings/mcp.json` | `AGENTS.md` | `.kiro/skills` | — | — | — | `roady plan import .kiro/specs/<name>/tasks.md` |

Where an agent has no plan hook, the instruction block and the skill tell it
to put an approved plan into roady (`roady plan import` or `roady capture`)
before starting work — and the guard still stops it writing the plan into a
markdown file instead.

The hooks all run `roady hook <event> --agent <agent>`, which reads that
agent's payload and answers in its format. They are silent outside a roady
project and never block the agent because roady failed. See
[Other agents](#other-agents) for each agent's files and caveats.

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

## Other agents

### OpenAI Codex (`roady setup codex`, also `openai`)

- `.codex/config.toml`: `[mcp_servers.roady]` (`command = "roady"`,
  `args = ["mcp"]`). Only that table is touched; the rest of the file is
  kept byte for byte.
- `.codex/hooks.json`: `SessionStart` (matcher `startup|resume|clear|compact`)
  injects the brief; `PreToolUse` on `apply_patch|Edit|Write` reads the file
  names out of the patch and refuses plan files.
- Codex loads `.codex/` only in a trusted project, and asks you to review new
  hooks once (`/hooks`). It has no plan-approval hook: the instructions tell
  it to capture an approved plan.

### Gemini CLI (`roady setup gemini`)

- `.gemini/settings.json`: `mcpServers.roady` and `hooks` — `SessionStart`
  (brief), `BeforeTool` on `write_file|replace` (guard, answered with
  `decision: deny`), `AfterTool` on `exit_plan_mode` (imports the plan file
  it names, unless the result says the user rejected it).
- Project MCP servers start only in a trusted folder (`gemini trust`). Gemini
  has no post-compaction event, so the brief arrives at session start only.
- Instructions go to `GEMINI.md`; Gemini does not read `AGENTS.md` unless
  `context.fileName` says so.

### Cursor (`roady setup cursor`)

- `.cursor/mcp.json` and `.cursor/hooks.json` (`sessionStart` for the brief,
  `preToolUse` for the guard — without a matcher; the handler lets every
  tool but a file write through).
- The Cursor CLI (`agent`) has been reported to ignore the project
  `.cursor/mcp.json`; if roady's tools are missing there, add the same entry
  to `~/.cursor/mcp.json`. No plan hook: import a saved plan with
  `roady plan import .cursor/plans/<plan>.md`.

### OpenCode (`roady setup opencode`)

- `opencode.json`: `mcp.roady` (`type: local`, `command: ["roady", "mcp"]`).
  An `opencode.jsonc` is left alone (it may hold comments) and setup prints
  the entry to add.
- OpenCode has no shell hooks, so setup writes `.opencode/plugins/roady.js`,
  which calls `roady hook` from `tool.execute.before` (guard) and
  `experimental.session.compacting` (brief). It stays out of the way if
  roady is missing.

### GitHub Copilot (`roady setup copilot`)

- `.vscode/mcp.json` (`servers.roady`) for VS Code agent mode. The Copilot
  CLI reads `.mcp.json`, which `roady setup claude-code` writes.
- `.github/hooks/roady.json`: `sessionStart` and `preToolUse`. Copilot's
  command hooks fail closed, so each command ends in `|| true`: a missing
  roady never blocks a tool call.

### Kiro (`roady setup kiro`)

- `.kiro/settings/mcp.json` and the skill in `.kiro/skills`. Kiro reads
  `AGENTS.md` always. Its hooks live in custom agent definitions, so setup
  does not install them. Kiro plans as specs: `roady plan import
  .kiro/specs/<name>/tasks.md`.

## MCP Tools Reference

One tool per CLI noun, with the CLI verbs as its `action` — every project
command has an MCP equivalent:

| Tool | Actions |
|------|---------|
| `roady_next` | The current task (or the next to start): why, done-when, dependencies |
| `roady_capture` | Record features, requirements and tasks in one write |
| `roady_task` | `start` `complete` `block` `unblock` `stop` `reopen` `verify` `check` `dispatch` `list` |
| `roady_plan` | `get` `generate` `import` `prioritize` `decompose` `approve`* `reject`* `prune`* |
| `roady_spec` | `get` `add` `explain` `review` `validate` `analyze`* `import`* `lock`* |
| `roady_drift` | `detect` `explain` `semantic` `record` `accept`* |
| `roady_state` | `get` `rebuild`* |
| `roady_goal` | `list` `add` `edit` — the roadmap |
| `roady_audit` | `verify` `trail` |
| `roady_policy` / `roady_git` | `check` / `sync` |
| `roady_status` / `roady_query` / `roady_init` | Status, project context for a question, a new project |

\* A decision: it runs only after the user confirms it in their client (MCP
elicitation). Declined, cancelled, or a client that cannot ask means nothing
changes and the agent is told the CLI command to hand to a person. See
[mcp-guide.md](mcp-guide.md).

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
3. **Prove it done**: `roady task check <task-id>` before calling a task complete
4. **Disable built-in tasks**: Set `CLAUDE_CODE_ENABLE_TASKS=false` to prevent conflicts
