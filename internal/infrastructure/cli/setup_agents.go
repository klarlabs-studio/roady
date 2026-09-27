package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// Project setup for agents other than Claude Code. Each writes, where the
// agent supports it, the same four things `setup claude-code` does:
//
//   - the roady MCP server, in the agent's project config
//   - the instruction block, in the file the agent reads every session
//   - the roady-planning skill, in .agents/skills (read by Codex, Gemini CLI,
//     Cursor, OpenCode and Copilot) or the agent's own skills directory
//   - hooks: the brief at session start, the plan-file guard, and plan
//     import where the agent has a plan-approval event
//
// Every file is merged, never overwritten wholesale unless roady owns it, and
// re-running changes nothing.

// agentSetup is one agent's project setup.
type agentSetup struct {
	name         string // setup target
	label        string
	instructions string // instruction file
	skillDir     string // where the skill goes
	configure    func(root string) ([]string, error)
	notes        []string
}

func agentSetups() []agentSetup {
	return []agentSetup{
		{
			name: "codex", label: "OpenAI Codex", instructions: "AGENTS.md", skillDir: ".agents/skills",
			configure: configureCodex,
			notes: []string{
				"Codex loads .codex/ only in a trusted project, and asks you to review new hooks once (/hooks).",
				"Codex has no plan-approval hook: the instructions tell it to record an approved plan with roady capture.",
			},
		},
		{
			name: "gemini", label: "Gemini CLI", instructions: "GEMINI.md", skillDir: ".agents/skills",
			configure: configureGemini,
			notes: []string{
				"Gemini CLI starts project MCP servers only in a trusted folder (`gemini trust`).",
				"Gemini CLI has no post-compaction event; the brief is injected at session start.",
			},
		},
		{
			name: "cursor", label: "Cursor", instructions: "AGENTS.md", skillDir: ".agents/skills",
			configure: configureCursor,
			notes: []string{
				"The Cursor CLI (`agent`) has been reported to ignore the project .cursor/mcp.json; if roady's tools are missing there, add the same entry to ~/.cursor/mcp.json.",
				"Cursor has no plan-approval hook: import a saved plan with `roady plan import .cursor/plans/<plan>.md`.",
			},
		},
		{
			name: "opencode", label: "OpenCode", instructions: "AGENTS.md", skillDir: ".agents/skills",
			configure: configureOpenCode,
		},
		{
			name: "copilot", label: "GitHub Copilot", instructions: "AGENTS.md", skillDir: ".agents/skills",
			configure: configureCopilot,
		},
		{
			name: "kiro", label: "Kiro", instructions: "AGENTS.md", skillDir: ".kiro/skills",
			configure: configureKiro,
			notes: []string{
				"Kiro's hooks live in custom agent definitions, so setup does not install them. Import a spec's tasks with `roady plan import .kiro/specs/<name>/tasks.md`.",
			},
		},
	}
}

func findAgentSetup(name string) (agentSetup, bool) {
	if name == "openai" {
		name = "codex"
	}
	for _, a := range agentSetups() {
		if a.name == name {
			return a, true
		}
	}
	return agentSetup{}, false
}

// runAgentSetup performs one agent's setup in the current project.
func runAgentSetup(a agentSetup) error {
	fmt.Printf("🚀 Setting up Roady for %s...\n", a.label)
	root, err := getProjectRoot()
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	lines, err := a.configure(root)
	if err != nil {
		return err
	}
	for _, l := range lines {
		fmt.Printf("  ✓ %s\n", l)
	}
	if !setupNoInstructions {
		if err := installInstructions(root, a.instructions); err != nil {
			return err
		}
		skill, err := writeRoadySkillIn(root, a.skillDir)
		if err != nil {
			return err
		}
		fmt.Printf("  ✓ %s\n", describeSkill(skill))
	}
	if len(a.notes) > 0 {
		fmt.Println("\nNotes:")
		for _, n := range a.notes {
			fmt.Printf("  - %s\n", n)
		}
	}
	fmt.Printf("\n✅ %s setup complete. Commit the files above to share them.\n", a.label)
	return nil
}

// hookCommand is the command an agent's hook runs.
func hookCommand(event, agent string) string {
	return fmt.Sprintf("roady hook %s --agent %s", event, agent)
}

// ---- Codex ----------------------------------------------------------------

func configureCodex(root string) ([]string, error) {
	var out []string
	reg, err := upsertTOMLTable(filepath.Join(root, ".codex", "config.toml"), "[mcp_servers.roady]",
		"command = \"roady\"\nargs = [\"mcp\"]\n")
	if err != nil {
		return nil, err
	}
	out = append(out, describeFile(reg, "the roady MCP server"))

	hookEntry := func(matcher, command string) map[string]any {
		e := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
		if matcher != "" {
			e["matcher"] = matcher
		}
		return e
	}
	reg, err = mergeJSONFile(filepath.Join(root, ".codex", "hooks.json"), func(doc map[string]any) error {
		return mergeHookEvents(doc, "hooks", map[string][]any{
			"SessionStart": {hookEntry("startup|resume|clear|compact", hookCommand("session-start", agentCodex))},
			"PreToolUse":   {hookEntry("apply_patch|Edit|Write", hookCommand("guard-write", agentCodex))},
		})
	})
	if err != nil {
		return nil, err
	}
	return append(out, describeFile(reg, "roady's hooks (brief at session start and after compaction, plan-file guard)")), nil
}

// ---- Gemini CLI -----------------------------------------------------------

func configureGemini(root string) ([]string, error) {
	hookEntry := func(matcher, command string) map[string]any {
		e := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "name": "roady"}}}
		if matcher != "" {
			e["matcher"] = matcher
		}
		return e
	}
	reg, err := mergeJSONFile(filepath.Join(root, ".gemini", "settings.json"), func(doc map[string]any) error {
		if err := setJSONKey(doc, "mcpServers", "roady", map[string]any{"command": "roady", "args": []any{"mcp"}}); err != nil {
			return err
		}
		return mergeHookEvents(doc, "hooks", map[string][]any{
			"SessionStart": {hookEntry("", hookCommand("session-start", agentGemini))},
			"BeforeTool":   {hookEntry("write_file|replace", hookCommand("guard-write", agentGemini))},
			"AfterTool":    {hookEntry("exit_plan_mode", hookCommand("plan-approved", agentGemini))},
		})
	})
	if err != nil {
		return nil, err
	}
	return []string{describeFile(reg, "the roady MCP server and hooks (brief at session start, plan-mode import, plan-file guard)")}, nil
}

// ---- Cursor ---------------------------------------------------------------

func configureCursor(root string) ([]string, error) {
	var out []string
	reg, err := mergeJSONFile(filepath.Join(root, ".cursor", "mcp.json"), func(doc map[string]any) error {
		return setJSONKey(doc, "mcpServers", "roady", map[string]any{"type": "stdio", "command": "roady", "args": []any{"mcp"}})
	})
	if err != nil {
		return nil, err
	}
	out = append(out, describeFile(reg, "the roady MCP server"))

	reg, err = mergeJSONFile(filepath.Join(root, ".cursor", "hooks.json"), func(doc map[string]any) error {
		if _, ok := doc["version"]; !ok {
			doc["version"] = float64(1)
		}
		return mergeHookEvents(doc, "hooks", map[string][]any{
			"sessionStart": {map[string]any{"command": hookCommand("session-start", agentCursor)}},
			// No matcher: the handler lets every tool but a file write through,
			// which holds whatever Cursor names its write tools.
			"preToolUse": {map[string]any{"command": hookCommand("guard-write", agentCursor)}},
		})
	})
	if err != nil {
		return nil, err
	}
	return append(out, describeFile(reg, "roady's hooks (brief at session start, plan-file guard)")), nil
}

// ---- OpenCode -------------------------------------------------------------

func configureOpenCode(root string) ([]string, error) {
	var out []string
	_, jsoncErr := os.Stat(filepath.Join(root, "opencode.jsonc"))
	_, jsonErr := os.Stat(filepath.Join(root, "opencode.json"))
	if jsoncErr == nil && os.IsNotExist(jsonErr) {
		// JSONC may hold comments a JSON round trip would drop.
		out = append(out, "opencode.jsonc was left alone (it may hold comments); add to it:\n"+
			`      "mcp": {"roady": {"type": "local", "command": ["roady", "mcp"], "enabled": true}}`)
	} else {
		reg, err := mergeJSONFile(filepath.Join(root, "opencode.json"), func(doc map[string]any) error {
			if _, ok := doc["$schema"]; !ok {
				doc["$schema"] = "https://opencode.ai/config.json"
			}
			return setJSONKey(doc, "mcp", "roady", map[string]any{"type": "local", "command": []any{"roady", "mcp"}, "enabled": true})
		})
		if err != nil {
			return nil, err
		}
		out = append(out, describeFile(reg, "the roady MCP server"))
	}
	reg, err := writeManagedFile(filepath.Join(root, ".opencode", "plugins", "roady.js"), openCodePlugin)
	if err != nil {
		return nil, err
	}
	return append(out, describeFile(reg, "roady's plugin (brief after compaction, plan-file guard)")), nil
}

// openCodePlugin bridges OpenCode's plugin events to `roady hook`. OpenCode
// has no shell hooks, only JS plugins; this one shells out so the logic stays
// in roady.
const openCodePlugin = `// Managed by ` + "`roady setup opencode`" + `; rewritten on the next run.
// Bridges OpenCode plugin events to ` + "`roady hook`" + `: the plan-file guard before
// a write, and the current task brief when a session is compacted.
export const Roady = async ({ $, directory }) => {
  const hook = async (name, payload) => {
    try {
      const res = await $` + "`roady hook ${name} --agent opencode < ${new Response(JSON.stringify(payload))}`" + `
        .cwd(directory).quiet().nothrow()
      return { code: res.exitCode, out: res.stdout.toString(), err: res.stderr.toString() }
    } catch {
      return { code: 0, out: "", err: "" } // roady missing or failing: stay out of the way
    }
  }
  return {
    "tool.execute.before": async (input, output) => {
      const r = await hook("guard-write", { cwd: directory, tool_name: input.tool, tool_input: output.args })
      if (r.code === 2) throw new Error(r.err.trim())
    },
    "experimental.session.compacting": async (_input, output) => {
      const r = await hook("session-start", { cwd: directory, source: "compact" })
      if (r.out.trim()) output.context.push(r.out)
    },
  }
}
`

// ---- GitHub Copilot -------------------------------------------------------

func configureCopilot(root string) ([]string, error) {
	var out []string
	reg, err := mergeJSONFile(filepath.Join(root, ".vscode", "mcp.json"), func(doc map[string]any) error {
		return setJSONKey(doc, "servers", "roady", map[string]any{"type": "stdio", "command": "roady", "args": []any{"mcp"}})
	})
	if err != nil {
		return nil, err
	}
	out = append(out, describeFile(reg, "the roady MCP server (VS Code agent mode)"))

	// Copilot command hooks fail closed, so each command tolerates a missing
	// or failing roady rather than blocking every tool call.
	entry := func(event string) map[string]any {
		return map[string]any{"type": "command", "bash": hookCommand(event, agentCopilot) + " 2>/dev/null || true", "timeoutSec": float64(30)}
	}
	doc := map[string]any{"version": float64(1), "hooks": map[string]any{
		"sessionStart": []any{entry("session-start")},
		"preToolUse":   []any{entry("guard-write")},
	}}
	raw, _ := json.MarshalIndent(doc, "", "  ")
	reg, err = writeManagedFile(filepath.Join(root, ".github", "hooks", "roady.json"), string(raw)+"\n")
	if err != nil {
		return nil, err
	}
	out = append(out, describeFile(reg, "roady's hooks (brief at session start, plan-file guard)"))
	out = append(out, "Copilot CLI reads MCP servers from .mcp.json or ~/.copilot/mcp-config.json; `roady setup claude-code` writes .mcp.json")
	return out, nil
}

// ---- Kiro -----------------------------------------------------------------

func configureKiro(root string) ([]string, error) {
	reg, err := mergeJSONFile(filepath.Join(root, ".kiro", "settings", "mcp.json"), func(doc map[string]any) error {
		return setJSONKey(doc, "mcpServers", "roady", map[string]any{"command": "roady", "args": []any{"mcp"}, "disabled": false})
	})
	if err != nil {
		return nil, err
	}
	return []string{describeFile(reg, "the roady MCP server")}, nil
}

// ---- file helpers -----------------------------------------------------------

func describeFile(r mcpRegistration, what string) string {
	switch {
	case r.Created:
		return fmt.Sprintf("Created %s with %s", r.Path, what)
	case r.Unchanged:
		return fmt.Sprintf("%s already current (%s)", r.Path, what)
	default:
		return fmt.Sprintf("Updated %s: %s", r.Path, what)
	}
}

// mergeJSONFile applies mutate to the JSON object in path and writes it back
// only when something changed. A file that is not a JSON object is reported
// and left alone: it belongs to the user.
func mergeJSONFile(path string, mutate func(doc map[string]any) error) (mcpRegistration, error) {
	reg := mcpRegistration{Path: path}
	doc := map[string]any{}
	raw, err := os.ReadFile(path) // #nosec G304 -- an agent config file under the project root
	switch {
	case os.IsNotExist(err):
		reg.Created = true
	case err != nil:
		return reg, fmt.Errorf("read %s: %w", path, err)
	case len(bytes.TrimSpace(raw)) > 0:
		if err := json.Unmarshal(raw, &doc); err != nil {
			return reg, fmt.Errorf("%s is not valid JSON, so it was left untouched: %w", path, err)
		}
	}
	before, _ := json.Marshal(doc)
	if err := mutate(doc); err != nil {
		return reg, fmt.Errorf("%s: %w", path, err)
	}
	after, _ := json.Marshal(doc)
	if !reg.Created && bytes.Equal(before, after) {
		reg.Unchanged = true
		return reg, nil
	}
	reg.Updated = !reg.Created
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return reg, err
	}
	return reg, writeProjectFile(path, append(out, '\n'))
}

// setJSONKey sets doc[section][name] = value, keeping the section's other
// entries.
func setJSONKey(doc map[string]any, section, name string, value any) error {
	sec, ok := doc[section].(map[string]any)
	if doc[section] != nil && !ok {
		return fmt.Errorf("%s is not an object, so it was left untouched", section)
	}
	if sec == nil {
		sec = map[string]any{}
	}
	if !reflect.DeepEqual(sec[name], value) {
		sec[name] = value
	}
	doc[section] = sec
	return nil
}

// mergeHookEvents replaces roady's entries under doc[key][event] with want,
// keeping every other hook.
func mergeHookEvents(doc map[string]any, key string, want map[string][]any) error {
	hooks, ok := doc[key].(map[string]any)
	if doc[key] != nil && !ok {
		return fmt.Errorf("%s is not an object, so it was left untouched", key)
	}
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, entries := range want {
		existing, ok := hooks[event].([]any)
		if hooks[event] != nil && !ok {
			return fmt.Errorf("%s.%s is not a list, so it was left untouched", key, event)
		}
		kept := make([]any, 0, len(existing)+len(entries))
		for _, e := range existing {
			if !mentionsRoadyHook(e) {
				kept = append(kept, e)
			}
		}
		hooks[event] = append(kept, entries...)
	}
	doc[key] = hooks
	return nil
}

// mentionsRoadyHook reports whether a hook entry runs `roady hook`, in
// whatever field the agent keeps the command.
func mentionsRoadyHook(v any) bool {
	switch x := v.(type) {
	case string:
		return strings.HasPrefix(strings.TrimSpace(x), roadyHookPrefix)
	case map[string]any:
		for _, val := range x {
			if mentionsRoadyHook(val) {
				return true
			}
		}
	case []any:
		for _, val := range x {
			if mentionsRoadyHook(val) {
				return true
			}
		}
	}
	return false
}

// upsertTOMLTable sets one [table] in a TOML file: replaced when the header
// is there, appended otherwise. The rest of the file is kept byte for byte.
// Line-based on purpose: roady's table is plain keys, and a TOML library
// would reformat the user's file.
func upsertTOMLTable(path, header, body string) (mcpRegistration, error) {
	reg := mcpRegistration{Path: path}
	raw, err := os.ReadFile(path) // #nosec G304 -- an agent config file under the project root
	switch {
	case os.IsNotExist(err):
		reg.Created = true
	case err != nil:
		return reg, fmt.Errorf("read %s: %w", path, err)
	}
	block := header + "\n" + body
	lines := strings.SplitAfter(string(raw), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == header {
			start = i
			break
		}
	}
	var out string
	if start < 0 {
		existing := strings.TrimRight(string(raw), "\n")
		if existing != "" {
			existing += "\n\n"
		}
		out = existing + block
	} else {
		end := len(lines)
		for i := start + 1; i < len(lines); i++ {
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
				end = i
				break
			}
		}
		current := strings.TrimRight(strings.Join(lines[start:end], ""), "\n") + "\n"
		if current == block {
			reg.Unchanged = true
			return reg, nil
		}
		rest := strings.Join(lines[end:], "")
		if rest != "" {
			rest = "\n" + rest
		}
		out = strings.Join(lines[:start], "") + block + rest
		reg.Updated = true
	}
	return reg, writeProjectFile(path, []byte(out))
}

// writeManagedFile writes a file roady owns entirely.
func writeManagedFile(path, content string) (mcpRegistration, error) {
	reg := mcpRegistration{Path: path}
	raw, err := os.ReadFile(path) // #nosec G304 -- a roady-owned file under the project root
	switch {
	case os.IsNotExist(err):
		reg.Created = true
	case err != nil:
		return reg, fmt.Errorf("read %s: %w", path, err)
	case string(raw) == content:
		reg.Unchanged = true
		return reg, nil
	default:
		reg.Updated = true
	}
	return reg, writeProjectFile(path, []byte(content))
}

func writeProjectFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- shared project directory
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 -- project config, committed and shared
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeRoadySkillIn installs the planning skill under root/dir.
func writeRoadySkillIn(root, dir string) (mcpRegistration, error) {
	return writeManagedFile(filepath.Join(root, filepath.FromSlash(dir), "roady-planning", "SKILL.md"), roadySkill)
}
