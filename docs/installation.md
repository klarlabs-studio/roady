# Installation

## Quick Install

### Homebrew (macOS/Linux)

```bash
brew install felixgeelhaar/tap/roady
```

### Go Install

```bash
go install github.com/felixgeelhaar/roady@latest
```

### Download Binary

Download the latest release from [GitHub Releases](https://github.com/felixgeelhaar/roady/releases/latest):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/felixgeelhaar/roady/releases/latest/download/roady-darwin-arm64 -o /usr/local/bin/roady

# macOS (Intel)
curl -L https://github.com/felixgeelhaar/roady/releases/latest/download/roady-darwin-amd64 -o /usr/local/bin/roady

# Linux
curl -L https://github.com/felixgeelhaar/roady/releases/latest/download/roady-linux-amd64 -o /usr/local/bin/roady

chmod +x /usr/local/bin/roady
```

## AI Coding Tool Setup

### One-Command Setup

Run in your project:

```bash
roady setup claude-code   # or: codex, gemini, cursor, opencode, copilot, kiro
roady setup all           # every agent at once
roady setup claude-desktop  # prints the Claude Desktop config to add
```

Each target registers the MCP server in the agent's project config, adds the
roady instruction block and planning skill, and installs the hooks the agent
supports. See [mcp-integration.md](mcp-integration.md) for what each agent
gets.

## Manual MCP Configuration

## Verify Installation

```bash
# Check version
roady --version

# Run setup wizard
roady config-wizard

# Check health
roady doctor
```

## Model configuration — there is none

Roady calls no language model and needs no API key. Earlier versions embedded
provider clients (Ollama, OpenAI, Anthropic, Gemini) and the `ROADY_AI_PROVIDER`
and `*_API_KEY` variables configured them; both were removed in v0.15.0.

The operations that need a model — decomposing a spec, reviewing it, suggesting
priorities, explaining drift, judging semantic drift — now return a prompt for
you to run. You already have a model, a key, a budget, and more context about
the task than Roady can reconstruct from files, so Roady assembles the question
and leaves the inference to you.

```bash
roady plan generate --ai        # prints the decomposition prompt
roady spec review               # prints a review prompt
roady drift semantic            # prints the semantic-drift question
```

Write the answer back with the tool named in the request — `roady_plan_update`,
`roady spec add`, `roady_drift_record_semantic`. See `docs/prompts.md`.

## Shell Completion

```bash
# Bash
roady completion bash > /etc/bash_completion.d/roady

# Zsh
roady completion zsh > "${fpath[1]}/_roady"

# Fish
roady completion fish > ~/.config/fish/completions/roady.fish
```

## Next Steps

1. Initialize a project: `roady init my-project`
2. Add a feature: `roady spec add "Feature Name" "Description"`
3. Generate plan: `roady plan generate --ai`
4. Approve and execute: `roady plan approve`
