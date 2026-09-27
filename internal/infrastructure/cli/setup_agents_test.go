package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return doc
}

// Every agent's setup writes its files once and changes nothing the second
// time.
func TestAgentSetupsAreIdempotent(t *testing.T) {
	for _, a := range agentSetups() {
		t.Run(a.name, func(t *testing.T) {
			dir := t.TempDir()
			first, err := a.configure(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(first) == 0 {
				t.Fatal("setup reported nothing")
			}
			second, err := a.configure(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, line := range second {
				if strings.HasPrefix(line, "Created") || strings.HasPrefix(line, "Updated") {
					t.Errorf("second run changed something: %s", line)
				}
			}
		})
	}
}

func TestCodexSetup(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".codex", "config.toml")
	_ = os.MkdirAll(filepath.Dir(cfg), 0o755)
	own := "model = \"gpt-5\"\n\n[mcp_servers.roady]\ncommand = \"old-roady\"\n\n[mcp_servers.other]\ncommand = \"other\"\n"
	_ = os.WriteFile(cfg, []byte(own), 0o644)
	hooks := filepath.Join(dir, ".codex", "hooks.json")
	_ = os.WriteFile(hooks, []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"./check.sh"}]}]}}`), 0o644)

	if _, err := configureCodex(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cfg)
	want := "model = \"gpt-5\"\n\n[mcp_servers.roady]\ncommand = \"roady\"\nargs = [\"mcp\"]\n\n[mcp_servers.other]\ncommand = \"other\"\n"
	if string(got) != want {
		t.Errorf("config.toml:\n%s\nwant:\n%s", got, want)
	}
	doc := readJSONFile(t, hooks)
	pre := doc["hooks"].(map[string]any)["PreToolUse"].([]any)
	if len(pre) != 2 || !strings.Contains(mustJSON(pre[0]), "./check.sh") || !strings.Contains(mustJSON(pre[1]), "guard-write --agent codex") {
		t.Errorf("PreToolUse = %s", mustJSON(pre))
	}
	ss := mustJSON(doc["hooks"].(map[string]any)["SessionStart"])
	if !strings.Contains(ss, "compact") || !strings.Contains(ss, "session-start --agent codex") {
		t.Errorf("SessionStart = %s", ss)
	}
}

func TestGeminiSetupKeepsSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gemini", "settings.json")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, []byte(`{"theme":"dark","mcpServers":{"gh":{"command":"gh-mcp"}}}`), 0o644)
	if _, err := configureGemini(dir); err != nil {
		t.Fatal(err)
	}
	doc := readJSONFile(t, path)
	servers := doc["mcpServers"].(map[string]any)
	if doc["theme"] != "dark" || servers["gh"] == nil || servers["roady"] == nil {
		t.Errorf("settings = %s", mustJSON(doc))
	}
	after := mustJSON(doc["hooks"].(map[string]any)["AfterTool"])
	if !strings.Contains(after, "exit_plan_mode") || !strings.Contains(after, "plan-approved --agent gemini") {
		t.Errorf("AfterTool = %s", after)
	}
}

func TestCursorAndCopilotSetup(t *testing.T) {
	dir := t.TempDir()
	if _, err := configureCursor(dir); err != nil {
		t.Fatal(err)
	}
	hooks := readJSONFile(t, filepath.Join(dir, ".cursor", "hooks.json"))
	if hooks["version"] != float64(1) || !strings.Contains(mustJSON(hooks["hooks"]), "guard-write --agent cursor") {
		t.Errorf("cursor hooks = %s", mustJSON(hooks))
	}
	if _, err := configureCopilot(dir); err != nil {
		t.Fatal(err)
	}
	cp := readJSONFile(t, filepath.Join(dir, ".github", "hooks", "roady.json"))
	if !strings.Contains(mustJSON(cp), `|| true`) {
		t.Errorf("copilot hooks fail closed, so they must tolerate a missing roady: %s", mustJSON(cp))
	}
	if readJSONFile(t, filepath.Join(dir, ".vscode", "mcp.json"))["servers"] == nil {
		t.Error("VS Code MCP entry missing")
	}
}

func TestOpenCodeSetup(t *testing.T) {
	dir := t.TempDir()
	if _, err := configureOpenCode(dir); err != nil {
		t.Fatal(err)
	}
	cfg := readJSONFile(t, filepath.Join(dir, "opencode.json"))
	roady := cfg["mcp"].(map[string]any)["roady"].(map[string]any)
	if roady["type"] != "local" || mustJSON(roady["command"]) != `["roady","mcp"]` {
		t.Errorf("opencode mcp = %s", mustJSON(roady))
	}
	plugin, err := os.ReadFile(filepath.Join(dir, ".opencode", "plugins", "roady.js"))
	if err != nil || !strings.Contains(string(plugin), "tool.execute.before") {
		t.Errorf("plugin: %v", err)
	}

	// JSONC may carry comments; it is left alone.
	other := t.TempDir()
	_ = os.WriteFile(filepath.Join(other, "opencode.jsonc"), []byte("// mine\n{}\n"), 0o644)
	lines, err := configureOpenCode(other)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(other, "opencode.jsonc")); string(b) != "// mine\n{}\n" {
		t.Error("opencode.jsonc was modified")
	}
	if !strings.Contains(strings.Join(lines, "\n"), "left alone") {
		t.Errorf("setup should say what to add: %v", lines)
	}
}

func TestFindAgentSetupAliases(t *testing.T) {
	if a, ok := findAgentSetup("openai"); !ok || a.name != "codex" {
		t.Error("openai should set up Codex")
	}
	if _, ok := findAgentSetup("nope"); ok {
		t.Error("unknown agent")
	}
}
