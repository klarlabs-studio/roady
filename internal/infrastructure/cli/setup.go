package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup [target]",
	Short: "Set Roady up for an AI coding agent (claude-code, codex, gemini, cursor, opencode, copilot, kiro)",
	Long: `Configure Roady for an AI coding agent in this project.

Project targets write, where the agent supports it: the roady MCP server in
the agent's project config, a marked instruction block (plans live in roady;
never ROADMAP.md/TODO.md/plan.md), the roady-planning skill, and hooks that
put the current task brief in context at session start, refuse plan files,
and import a plan the user approved in plan mode.

  claude-code    .mcp.json, .claude/settings.json hooks, CLAUDE.md, .claude/skills
  codex          .codex/config.toml, .codex/hooks.json, AGENTS.md, .agents/skills
  gemini         .gemini/settings.json (MCP + hooks), GEMINI.md, .agents/skills
  cursor         .cursor/mcp.json, .cursor/hooks.json, AGENTS.md, .agents/skills
  opencode       opencode.json, .opencode/plugins/roady.js, AGENTS.md, .agents/skills
  copilot        .vscode/mcp.json, .github/hooks/roady.json, AGENTS.md, .agents/skills
  kiro           .kiro/settings/mcp.json, AGENTS.md, .kiro/skills
  all            every target above

  claude-desktop prints the Claude Desktop MCP config to add
  global         installs the Claude Code slash commands in ~/.claude/commands

"openai" is accepted for codex. Every file is merged, not overwritten: other
servers, hooks and settings are kept, and re-running changes nothing.
--no-instructions skips the instruction block and the skill.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		target := "claude-code"
		if len(args) > 0 {
			target = strings.ToLower(args[0])
		}

		switch target {
		case "claude-code":
			return setupClaudeCode()
		case "claude-desktop":
			return setupClaudeDesktop()
		case "global":
			return setupGlobal()
		case "all":
			if err := setupClaudeCode(); err != nil {
				return err
			}
			for _, a := range agentSetups() {
				fmt.Println()
				if err := runAgentSetup(a); err != nil {
					return err
				}
			}
			return nil
		}
		if a, ok := findAgentSetup(target); ok {
			return runAgentSetup(a)
		}
		return fmt.Errorf("unknown target: %s (supported: claude-code, codex, gemini, cursor, opencode, copilot, kiro, all, claude-desktop, global)", target)
	},
}

func setupClaudeCode() error {
	fmt.Println("🚀 Setting up Roady for Claude Code...")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("create .claude/commands: %w", err)
	}

	if _, err := exec.LookPath("roady"); err != nil {
		fmt.Println("Warning: roady is not on PATH - Claude Code will not be able to start the MCP server")
	}

	roadyCommands := map[string]string{
		"roady-task.md":   roadyTaskCommand,
		"roady-status.md": roadyStatusCommand,
		"roady-review.md": roadyReviewCommand,
	}

	for name, content := range roadyCommands {
		path := filepath.Join(commandsDir, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("write command %s: %w", name, err)
			}
			fmt.Printf("  ✓ Created %s\n", path)
		} else {
			fmt.Printf("  ✓ %s already exists\n", name)
		}
	}

	// MCP servers are read from the project's .mcp.json or from
	// ~/.claude.json, never from a settings file. This used to write
	// ~/.claude/settings.local.json, which Claude Code does not read for
	// mcpServers, and skipped silently when that file existed — so the
	// server was never registered and the command still reported success.
	root, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	reg, err := registerClaudeCodeMCP(root)
	if err != nil {
		return err
	}
	fmt.Printf("  ✓ %s\n", reg.Describe())
	hooks, err := registerClaudeCodeHooks(root)
	if err != nil {
		return err
	}
	fmt.Printf("  ✓ %s\n", describeHooks(hooks))
	if !setupNoInstructions {
		if err := installInstructions(root, "CLAUDE.md"); err != nil {
			return err
		}
		skill, err := writeRoadySkill(root)
		if err != nil {
			return err
		}
		fmt.Printf("  ✓ %s\n", describeSkill(skill))
	}

	fmt.Println("\n✅ Claude Code setup complete!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Start Claude Code in this project and approve the roady server when asked")
	fmt.Println("     (project servers from .mcp.json need a one-time approval; `claude mcp list` shows the status)")
	fmt.Println("  2. Commit .mcp.json and .claude/settings.json so collaborators get the same server and hooks")
	fmt.Println("  3. Run /roady-task to start a task")
	fmt.Println("\nTo register roady for every project instead: claude mcp add --scope user roady -- roady mcp")

	return nil
}

func setupClaudeDesktop() error {
	fmt.Println("🚀 Setting up Roady for Claude Desktop...")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")

	roadyConfig := `,
    "mcpServers": {
      "roady": {
        "command": "roady",
        "args": ["mcp"],
        "env": {}
      }
    }`

	fmt.Printf("  📝 Edit %s and add:\n", configPath)
	fmt.Println(roadyConfig)

	return nil
}

func setupGlobal() error {
	fmt.Println("🚀 Setting up Roady globally...")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	claudeDir := filepath.Join(homeDir, ".claude")
	commandsDir := filepath.Join(claudeDir, "commands")

	if err := os.MkdirAll(commandsDir, 0755); err != nil {
		return fmt.Errorf("create .claude/commands: %w", err)
	}

	roadyCommands := map[string]string{
		"roady-task.md":   roadyTaskCommand,
		"roady-status.md": roadyStatusCommand,
		"roady-review.md": roadyReviewCommand,
	}

	for name, content := range roadyCommands {
		path := filepath.Join(commandsDir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write command %s: %w", name, err)
		}
		fmt.Printf("  ✓ Installed %s\n", path)
	}

	fmt.Println("\n✅ Global setup complete!")
	return nil
}

// setupNoInstructions leaves CLAUDE.md / AGENTS.md / GEMINI.md and the skill
// alone, for projects that manage their agent instructions themselves.
var setupNoInstructions bool

func init() {
	setupCmd.Flags().BoolVar(&setupNoInstructions, "no-instructions", false, "Do not write the roady block into CLAUDE.md / AGENTS.md / GEMINI.md or install the planning skill")
	RootCmd.AddCommand(setupCmd)
}

const (
	roadyTaskCommand = `# Start Next Ready Task

Start the next task that is ready to begin (unlocked and pending).

## Usage
/roady-task

## What it does
1. Runs roady task ready to find the next pending task
2. Starts the task with roady task start <task-id>
3. Reports the task details
`

	roadyStatusCommand = `# Full Project Status

Get a comprehensive overview of the project status.

## Usage
/roady-status

## What it does
1. Runs roady status for task overview
2. Checks for drift with roady drift detect
3. Runs roady next for the current task and what done means
`

	roadyReviewCommand = `# Check for Drift

Detect any discrepancies between the current implementation and the plan.

## Usage
/roady-review

## What it does
1. Runs roady drift detect to find implementation gaps
2. Runs roady drift explain for a prompt to explain what was found
3. Provides explanation of any drift found

## When to use
- Before starting new work
- After completing significant features
- During code review
`
)
