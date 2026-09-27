package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readMCPFile(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("read .mcp.json: %v", err)
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode .mcp.json: %v", err)
	}
	return doc
}

// setup used to write ~/.claude/settings.local.json, which Claude Code never
// reads for MCP servers, so the server was never registered.
func TestRegisterClaudeCodeMCPCreatesProjectFile(t *testing.T) {
	dir := t.TempDir()
	reg, err := registerClaudeCodeMCP(dir)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !reg.Created {
		t.Errorf("expected Created, got %+v", reg)
	}
	servers := readMCPFile(t, dir)["mcpServers"].(map[string]any)
	roady := servers["roady"].(map[string]any)
	if roady["command"] != "roady" || roady["type"] != "stdio" {
		t.Errorf("unexpected roady entry: %v", roady)
	}
}

// The old code skipped silently whenever the file existed. An existing file
// must be merged: other servers and keys stay, roady is added.
func TestRegisterClaudeCodeMCPMergesExistingServers(t *testing.T) {
	dir := t.TempDir()
	existing := `{"mcpServers":{"other":{"type":"http","url":"https://example.com/mcp"}},"note":"keep me"}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err := registerClaudeCodeMCP(dir)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if reg.Created || reg.Updated || reg.Unchanged {
		t.Errorf("expected a plain add, got %+v", reg)
	}
	doc := readMCPFile(t, dir)
	servers := doc["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Error("existing server was dropped")
	}
	if _, ok := servers["roady"]; !ok {
		t.Error("roady was not added")
	}
	if doc["note"] != "keep me" {
		t.Error("unrelated top-level key was dropped")
	}
}

func TestRegisterClaudeCodeMCPIsIdempotentAndRepairs(t *testing.T) {
	dir := t.TempDir()
	if _, err := registerClaudeCodeMCP(dir); err != nil {
		t.Fatal(err)
	}
	reg, err := registerClaudeCodeMCP(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Unchanged {
		t.Errorf("second run should change nothing, got %+v", reg)
	}

	stale := `{"mcpServers":{"roady":{"command":"/old/path/roady","args":["mcp"]}}}`
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	reg, err = registerClaudeCodeMCP(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Updated {
		t.Errorf("a differing roady entry should be replaced, got %+v", reg)
	}
}

func TestRegisterClaudeCodeMCPRefusesToOverwriteInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	broken := `{"mcpServers": {"mine": `
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := registerClaudeCodeMCP(dir); err == nil || !strings.Contains(err.Error(), "left untouched") {
		t.Fatalf("expected a refusal, got %v", err)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != broken {
		t.Error("an invalid file was overwritten")
	}
}
