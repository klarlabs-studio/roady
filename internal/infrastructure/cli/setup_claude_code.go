package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
