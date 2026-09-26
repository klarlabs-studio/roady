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
	Short: "Setup Roady for various platforms (claude-code, claude-desktop, opencode, openai, gemini)",
	Long: `Configure Roady for use with AI coding tools.

Supported targets:
  claude-code    - Configure Claude Code: MCP, hooks, CLAUDE.md block, planning skill
  claude-desktop - Configure Claude Desktop with Roady MCP server
  opencode       - Configure OpenCode with Roady MCP server (and AGENTS.md)
  openai         - Setup for OpenAI Codex (via MCP, and AGENTS.md)
  gemini         - Setup for Google Gemini (via MCP bridge, and GEMINI.md)
  global         - Install commands globally and setup MCP

Examples:
  roady setup claude-code
  roady setup opencode
  roady setup openai
  roady setup claude-desktop
  roady setup global

Project targets write a marked roady block into the agent's instruction file
(plans live in roady; never create ROADMAP.md/TODO.md/plan.md). Re-running
updates the block in place; --no-instructions skips it.`,
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
		case "opencode":
			return setupOpenCode()
		case "openai":
			return setupOpenAI()
		case "gemini":
			return setupGemini()
		case "global":
			return setupGlobal()
		default:
			return fmt.Errorf("unknown target: %s (supported: claude-code, claude-desktop, opencode, openai, gemini, global)", target)
		}
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

func setupOpenCode() error {
	fmt.Println("🚀 Setting up Roady for OpenCode...")
	if err := projectInstructions("AGENTS.md"); err != nil {
		return err
	}

	fmt.Println("\n📝 Add this to your OpenCode config (~/.opencode/config.json):")
	fmt.Println()
	fmt.Println(`{
  "mcpServers": {
    "roady": {
      "command": "roady",
      "args": ["mcp"]
    }
  }
}`)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".opencode", "config.json")
	if _, err := os.Stat(filepath.Dir(configPath)); os.IsNotExist(err) {
		fmt.Printf("\n  ✓ Config directory will be created at first launch\n")
	} else {
		fmt.Printf("\n  📁 Config path: %s\n", configPath)
	}

	fmt.Println("\n✅ OpenCode setup ready!")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Restart OpenCode")
	fmt.Println("  2. Roady MCP tools will be available")

	return nil
}

func setupOpenAI() error {
	fmt.Println("🚀 Setting up Roady for OpenAI Codex...")
	if err := projectInstructions("AGENTS.md"); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("📝 In your Codex agent code, use the MCP server:")
	fmt.Println()
	fmt.Println(`from agents import Agent
from openai import OpenAI

client = OpenAI()

# Start Roady MCP server as subprocess
import subprocess
roady_process = subprocess.Popen(
    ["roady", "mcp", "--transport", "stdio"],
    stdout=subprocess.PIPE,
    stdin=subprocess.PIPE,
)

# Use with Codex agent
agent = Agent(
    name="Developer",
    mcp_servers=[roady_process],  # Roady MCP
)`)
	fmt.Println("\nOr use with OpenAI SDK directly:")
	fmt.Println()
	fmt.Println(`from openai.mcp import MCPServer

server = MCPServer(command="roady", args=["mcp"])`)

	fmt.Println("\n✅ OpenAI Codex setup ready!")
	fmt.Println("\nNote: OpenAI Codex MCP support requires the latest OpenAI SDK.")

	return nil
}

func setupGemini() error {
	fmt.Println("🚀 Setting up Roady for Google Gemini...")
	if err := projectInstructions("GEMINI.md"); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("📝 Add Roady to Gemini MCP configuration:")
	fmt.Println()
	fmt.Print(`Via Google AI Studio or gcloud:

{
  "mcpServers": {
    "roady": {
      "command": "roady",
      "args": ["mcp"]
    }
  }
}

Note: Gemini MCP support varies by platform.
  - Google AI Studio: Use MCP servers extension
  - Vertex AI: Configure via Agent Builder
`)

	fmt.Println()
	fmt.Println("✅ Gemini setup ready!")

	return nil
}

// setupNoInstructions leaves CLAUDE.md / AGENTS.md / GEMINI.md and the skill
// alone, for projects that manage their agent instructions themselves.
var setupNoInstructions bool

// projectInstructions writes the roady block into the project's instruction
// files, unless --no-instructions.
func projectInstructions(files ...string) error {
	if setupNoInstructions {
		return nil
	}
	root, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	return installInstructions(root, files...)
}

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
3. Shows AI usage with roady usage
`

	roadyReviewCommand = `# Check for Drift

Detect any discrepancies between the current implementation and the plan.

## Usage
/roady-review

## What it does
1. Runs roady drift detect to find implementation gaps
2. Runs roady debt summary for planning debt overview
3. Provides explanation of any drift found

## When to use
- Before starting new work
- After completing significant features
- During code review
`
)
