package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func withHookAgent(t *testing.T, agent string) {
	t.Helper()
	old := hookAgent
	hookAgent = agent
	t.Cleanup(func() { hookAgent = old })
}

func initHookProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := storage.NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	sp := &spec.ProductSpec{ID: "demo", Title: "demo", Version: "0.1.0"}
	_ = repo.SaveSpec(sp)
	_ = repo.SaveSpecLock(sp)
	_ = repo.SaveState(planning.NewExecutionState("demo"))
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("GEMINI_PROJECT_DIR", "")
	t.Setenv("CURSOR_PROJECT_DIR", "")
	return dir
}

func TestHookFilePathsAcrossAgents(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: src/main.go\n@@\n-a\n+b\n*** Add File: ROADMAP.md\n+# Roadmap\n*** End Patch\n"
	cases := []struct {
		name string
		in   hookInput
		want []string
	}{
		{"claude", hookInput{ToolInput: map[string]any{"file_path": "/p/TODO.md"}}, []string{"/p/TODO.md"}},
		{"codex apply_patch", hookInput{ToolInput: map[string]any{"command": patch}}, []string{"src/main.go", "ROADMAP.md"}},
		{"opencode", hookInput{ToolInput: map[string]any{"filePath": "plan.md"}}, []string{"plan.md"}},
		{"kiro", hookInput{ToolInput: map[string]any{"path": "plan.md"}}, []string{"plan.md"}},
	}
	for _, c := range cases {
		got := hookFilePaths(c.in)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestReadHookInputNormalisesAgents(t *testing.T) {
	copilot := `{"cwd":"/p","toolName":"create","toolArgs":"{\"path\":\"TODO.md\"}"}`
	in := readHookInput(strings.NewReader(copilot))
	if in.ToolName != "create" || in.ToolInput["path"] != "TODO.md" {
		t.Errorf("copilot: %+v", in)
	}
	cursor := `{"hook_event_name":"preToolUse","workspace_roots":["/w"],"tool_name":"Write","tool_input":{"file_path":"x.md"}}`
	if in := readHookInput(strings.NewReader(cursor)); in.Cwd != "/w" {
		t.Errorf("cursor: cwd from workspace_roots, got %+v", in)
	}
}

func TestIsWriteTool(t *testing.T) {
	for _, name := range []string{"", "Write", "Edit", "MultiEdit", "apply_patch", "write_file", "replace", "fs_write", "edit", "create", "write"} {
		if !isWriteTool(name) {
			t.Errorf("%q should be guarded", name)
		}
	}
	for _, name := range []string{"Read", "read_file", "Shell", "Bash", "grep", "mcp__roady__roady_capture"} {
		if isWriteTool(name) {
			t.Errorf("%q must not be guarded: reading ROADMAP.md is fine", name)
		}
	}
}

func TestGuardWriteDialects(t *testing.T) {
	dir := initHookProject(t)
	payload := map[string]any{"cwd": dir, "tool_name": "Write", "tool_input": map[string]any{"file_path": filepath.Join(dir, "ROADMAP.md")}}

	cases := map[string]func(t *testing.T, out string){
		agentClaude: func(t *testing.T, out string) {
			if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, `"hookSpecificOutput"`) {
				t.Errorf("%s", out)
			}
		},
		agentCodex: func(t *testing.T, out string) {
			if !strings.Contains(out, `"hookEventName":"PreToolUse"`) || !strings.Contains(out, `"permissionDecision":"deny"`) {
				t.Errorf("%s", out)
			}
		},
		agentGemini: func(t *testing.T, out string) {
			var m map[string]any
			_ = json.Unmarshal([]byte(out), &m)
			if m["decision"] != "deny" || !strings.Contains(m["reason"].(string), "roady capture") {
				t.Errorf("%s", out)
			}
		},
		agentCursor: func(t *testing.T, out string) {
			var m map[string]any
			_ = json.Unmarshal([]byte(out), &m)
			if m["permission"] != "deny" || m["agent_message"] == nil {
				t.Errorf("%s", out)
			}
		},
		agentCopilot: func(t *testing.T, out string) {
			var m map[string]any
			_ = json.Unmarshal([]byte(out), &m)
			if m["permissionDecision"] != "deny" {
				t.Errorf("%s", out)
			}
		},
	}
	for agent, check := range cases {
		t.Run(agent, func(t *testing.T) {
			withHookAgent(t, agent)
			check(t, runHook(t, hookGuardWriteCmd, payload))
		})
	}

	for _, agent := range []string{agentKiro, agentOpenCode} {
		t.Run(agent, func(t *testing.T) {
			withHookAgent(t, agent)
			code := 0
			old := hookExit
			hookExit = func(c int) { code = c }
			defer func() { hookExit = old }()
			var stderr strings.Builder
			hookGuardWriteCmd.SetErr(&stderr)
			defer hookGuardWriteCmd.SetErr(nil)
			runHook(t, hookGuardWriteCmd, map[string]any{"cwd": dir, "tool_name": "fs_write", "tool_input": map[string]any{"path": "TODO.md"}})
			if code != 2 || !strings.Contains(stderr.String(), "roady capture") {
				t.Errorf("exit %d, stderr %q", code, stderr.String())
			}
		})
	}

	// Reading a plan file is not writing one.
	withHookAgent(t, agentCursor)
	if out := runHook(t, hookGuardWriteCmd, map[string]any{"cwd": dir, "tool_name": "Read", "tool_input": map[string]any{"file_path": "ROADMAP.md"}}); out != "" {
		t.Errorf("read was guarded: %s", out)
	}
	// Codex's apply_patch is guarded by the files inside the patch.
	withHookAgent(t, agentCodex)
	patch := "*** Begin Patch\n*** Add File: TODO.md\n+- x\n*** End Patch\n"
	if out := runHook(t, hookGuardWriteCmd, map[string]any{"cwd": dir, "tool_name": "apply_patch", "tool_input": map[string]any{"command": patch}}); !strings.Contains(out, "deny") {
		t.Errorf("apply_patch creating TODO.md: %q", out)
	}
}

func TestSessionStartDialects(t *testing.T) {
	dir := initHookProject(t)
	cases := map[string]string{
		agentClaude:  "Roady",
		agentCodex:   `"hookSpecificOutput":{"additionalContext":"Roady`,
		agentGemini:  `"hookEventName":"SessionStart"`,
		agentCursor:  `"additional_context":"Roady`,
		agentCopilot: `"additionalContext":"Roady`,
		agentKiro:    "Roady",
	}
	for agent, want := range cases {
		t.Run(agent, func(t *testing.T) {
			withHookAgent(t, agent)
			out := runHook(t, hookSessionStartCmd, map[string]any{"cwd": dir})
			if agent == agentCodex {
				var m map[string]map[string]string
				if err := json.Unmarshal([]byte(out), &m); err != nil || !strings.HasPrefix(m["hookSpecificOutput"]["additionalContext"], "Roady") {
					t.Errorf("%s: %q", agent, out)
				}
				return
			}
			if !strings.Contains(out, want) {
				t.Errorf("%s: %q lacks %q", agent, out, want)
			}
		})
	}
}

// Gemini CLI's exit_plan_mode names a plan file and runs AfterTool whether
// or not the user approved.
func TestGeminiPlanApproved(t *testing.T) {
	dir := initHookProject(t)
	withHookAgent(t, agentGemini)
	planFile := filepath.Join(t.TempDir(), "plan.md")
	_ = os.WriteFile(planFile, []byte("# Plan: Offline mode\n\n## Steps\n\n1. Queue writes\n2. Replay on reconnect\n"), 0o600)

	out := runHook(t, hookPlanApprovedCmd, map[string]any{
		"cwd": dir, "tool_name": "exit_plan_mode", "tool_input": map[string]any{"plan_path": planFile},
		"tool_response": map[string]any{"llmContent": "The user rejected the plan: use SQLite"},
	})
	if out != "" {
		t.Errorf("a rejected plan must not be imported: %q", out)
	}

	out = runHook(t, hookPlanApprovedCmd, map[string]any{
		"cwd": dir, "tool_name": "exit_plan_mode", "tool_input": map[string]any{"plan_path": planFile},
		"tool_response": map[string]any{"llmContent": "Plan approved. Switching to auto-edit mode."},
	})
	if !strings.Contains(out, `"hookEventName":"AfterTool"`) || !strings.Contains(out, "2 task(s) created") {
		t.Errorf("approved plan: %q", out)
	}
}
