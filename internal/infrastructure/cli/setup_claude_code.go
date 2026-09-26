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

// claudeCodeMCPFile is where Claude Code reads project-scoped MCP servers.
// Settings files (settings.json, settings.local.json) carry no mcpServers key;
// the other location, ~/.claude.json, is Claude Code's own state file and is
// left to `claude mcp add --scope user`.
const claudeCodeMCPFile = ".mcp.json"

// roadyMCPServer is the entry roady registers, in the shape `claude mcp add`
// writes for a stdio server.
func roadyMCPServer() map[string]any {
	return map[string]any{
		"type":    "stdio",
		"command": "roady",
		"args":    []any{"mcp"},
	}
}

// mcpRegistration reports what registering changed, so setup can say it
// rather than announce success over a file it did not touch.
type mcpRegistration struct {
	Path    string
	Created bool // the file did not exist
	Updated bool // an existing roady entry differed and was replaced
	// Unchanged is true when the file already held exactly this entry.
	Unchanged bool
}

// Describe renders the outcome for the setup output.
func (r mcpRegistration) Describe() string {
	switch {
	case r.Created:
		return fmt.Sprintf("Created %s with the roady MCP server", r.Path)
	case r.Updated:
		return fmt.Sprintf("Updated the roady entry in %s", r.Path)
	case r.Unchanged:
		return fmt.Sprintf("roady MCP server already registered in %s", r.Path)
	default:
		return fmt.Sprintf("Added the roady MCP server to %s", r.Path)
	}
}

// registerClaudeCodeMCP merges the roady server into root/.mcp.json.
//
// Every other server and every other top-level key is preserved. An existing
// file that is not valid JSON is an error rather than something to overwrite:
// it belongs to the user, and replacing it would discard their servers.
func registerClaudeCodeMCP(root string) (mcpRegistration, error) {
	path := filepath.Join(root, claudeCodeMCPFile)
	reg := mcpRegistration{Path: path}

	doc := map[string]any{}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is <project root>/.mcp.json
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

	servers, ok := doc["mcpServers"].(map[string]any)
	if doc["mcpServers"] != nil && !ok {
		return reg, fmt.Errorf("%s has an mcpServers value that is not an object, so it was left untouched", path)
	}
	if servers == nil {
		servers = map[string]any{}
	}

	want := roadyMCPServer()
	if existing, found := servers["roady"]; found {
		if reflect.DeepEqual(existing, want) {
			reg.Unchanged = true
			return reg, nil
		}
		reg.Updated = true
	}
	servers["roady"] = want
	doc["mcpServers"] = servers

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return reg, fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil { // #nosec G306 -- .mcp.json is meant to be committed and shared
		return reg, fmt.Errorf("write %s: %w", path, err)
	}
	return reg, nil
}

// claudeCodeSettingsFile is the project settings file Claude Code reads hooks
// from, shared with the team when committed.
const claudeCodeSettingsFile = ".claude/settings.json"

// roadyHookPrefix marks the hook commands roady owns, so setup can replace
// its own entries without touching anyone else's.
const roadyHookPrefix = "roady hook "

// roadyHooks are the hooks setup registers, by event.
//
//   - SessionStart, every source (startup, resume, clear, compact): the task
//     brief, so the plan is in context at the start and again after
//     compaction, without the agent having to ask for it.
//   - PostToolUse on ExitPlanMode: the plan the user just approved in plan
//     mode is imported into roady.
//   - PreToolUse on file writes: ROADMAP*.md, TODO*.md and plan*.md files
//     are refused with a pointer to roady capture.
func roadyHooks() map[string][]any {
	entry := func(matcher, command string) map[string]any {
		e := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "timeout": float64(30)}}}
		if matcher != "" {
			e["matcher"] = matcher
		}
		return e
	}
	return map[string][]any{
		"SessionStart": {entry("", roadyHookPrefix+"session-start")},
		"PostToolUse":  {entry("ExitPlanMode", roadyHookPrefix+"plan-approved")},
		"PreToolUse":   {entry("Write|Edit|MultiEdit", roadyHookPrefix+"guard-write")},
	}
}

// isRoadyHookEntry reports whether a matcher entry holds only roady's hooks.
func isRoadyHookEntry(e any) bool {
	m, ok := e.(map[string]any)
	if !ok {
		return false
	}
	hooks, ok := m["hooks"].([]any)
	if !ok || len(hooks) == 0 {
		return false
	}
	for _, h := range hooks {
		hm, ok := h.(map[string]any)
		if !ok {
			return false
		}
		cmd, _ := hm["command"].(string)
		if !strings.HasPrefix(strings.TrimSpace(cmd), roadyHookPrefix) {
			return false
		}
	}
	return true
}

// registerClaudeCodeHooks merges roady's hooks into root/.claude/settings.json.
//
// Entries roady owns are replaced; every other hook, event and setting is
// preserved. Re-running changes nothing. A settings file that is not valid
// JSON is left untouched and reported.
func registerClaudeCodeHooks(root string) (mcpRegistration, error) {
	path := filepath.Join(root, filepath.FromSlash(claudeCodeSettingsFile))
	reg := mcpRegistration{Path: path}

	doc := map[string]any{}
	raw, err := os.ReadFile(path) // #nosec G304 -- path is <project root>/.claude/settings.json
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
	hooks, ok := doc["hooks"].(map[string]any)
	if doc["hooks"] != nil && !ok {
		return reg, fmt.Errorf("%s has a hooks value that is not an object, so it was left untouched", path)
	}
	if hooks == nil {
		hooks = map[string]any{}
	}

	before, _ := json.Marshal(hooks)
	hadRoady := false
	for event, want := range roadyHooks() {
		existing, _ := hooks[event].([]any)
		if hooks[event] != nil && existing == nil {
			return reg, fmt.Errorf("%s has a hooks.%s value that is not a list, so it was left untouched", path, event)
		}
		kept := make([]any, 0, len(existing)+len(want))
		for _, e := range existing {
			if isRoadyHookEntry(e) {
				hadRoady = true
				continue
			}
			kept = append(kept, e)
		}
		hooks[event] = append(kept, want...)
	}
	after, _ := json.Marshal(hooks)
	if !reg.Created && bytes.Equal(before, after) {
		reg.Unchanged = true
		return reg, nil
	}
	reg.Updated = hadRoady
	doc["hooks"] = hooks

	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return reg, fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { // #nosec G301 -- .claude is a shared project directory
		return reg, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil { // #nosec G306 -- settings.json is meant to be committed and shared
		return reg, fmt.Errorf("write %s: %w", path, err)
	}
	return reg, nil
}

// describeHooks renders a hooks registration for the setup output.
func describeHooks(r mcpRegistration) string {
	switch {
	case r.Created:
		return fmt.Sprintf("Created %s with roady's hooks (task brief at session start and after compaction, plan-mode import, plan-file guard)", r.Path)
	case r.Unchanged:
		return fmt.Sprintf("roady hooks already registered in %s", r.Path)
	case r.Updated:
		return fmt.Sprintf("Updated roady's hooks in %s", r.Path)
	default:
		return fmt.Sprintf("Added roady's hooks to %s", r.Path)
	}
}
