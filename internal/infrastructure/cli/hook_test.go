package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
	"github.com/spf13/cobra"
)

func TestIsPlanFile(t *testing.T) {
	root := filepath.FromSlash("/work/proj")
	cases := []struct {
		target string
		allow  []string
		want   bool
	}{
		{"ROADMAP.md", nil, true},
		{"docs/roadmap-2027.md", nil, true},
		{"TODO.md", nil, true},
		{"todo_list.md", nil, true},
		{"plan.md", nil, true},
		{"PLANS.md", nil, true},
		{"docs/implementation-plan.md", nil, true},
		{"notes/plan.v2.md", nil, true},
		{"explanation.md", nil, false},
		{"airplane.md", nil, false},
		{"planning-notes.txt", nil, false},
		{"README.md", nil, false},
		{"internal/plan.go", nil, false},
		{".roady/plans/rate-limits.md", nil, false},
		{".claude/plans/brave-otter.md", nil, false},
		{"/home/me/.claude/plans/brave-otter.md", nil, false},
		{"ROADMAP.md", []string{"ROADMAP.md"}, false},
		{"docs/adr/plan.md", []string{"docs/adr/**"}, false},
		{"docs/plan.md", []string{"docs/*.md"}, false},
		{"deep/TODO.md", []string{"TODO.md"}, false},
		{"docs/plan.md", []string{"other/**"}, true},
	}
	for _, c := range cases {
		target := c.target
		if !filepath.IsAbs(filepath.FromSlash(target)) {
			target = filepath.FromSlash(target)
		}
		_, got := isPlanFile(root, root, target, c.allow)
		if got != c.want {
			t.Errorf("isPlanFile(%q, allow=%v) = %v, want %v", c.target, c.allow, got, c.want)
		}
	}
}

func TestApprovedPlanText(t *testing.T) {
	in := hookInput{ToolInput: map[string]any{"plan": "# Plan\n\n1. a\n"}}
	if got := approvedPlanText(in); !strings.Contains(got, "1. a") {
		t.Errorf("plan text from tool_input.plan: %q", got)
	}
	f := filepath.Join(t.TempDir(), "otter.md")
	if err := os.WriteFile(f, []byte("# From file\n\n- step\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	in = hookInput{ToolInput: map[string]any{}, ToolResponse: map[string]any{"filePath": f}}
	if got := approvedPlanText(in); !strings.Contains(got, "From file") {
		t.Errorf("plan text from a named plan file: %q", got)
	}
	if got := approvedPlanText(hookInput{}); got != "" {
		t.Errorf("empty payload: %q", got)
	}
}

func readSettings(t *testing.T, dir string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]any{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestRegisterClaudeCodeHooks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
  "permissions": {"allow": ["Bash(go test:*)"]},
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./lint.sh"}]}],
    "SessionStart": [{"hooks": [{"type": "command", "command": "roady hook session-start --old"}]}]
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := registerClaudeCodeHooks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reg.Updated {
		t.Errorf("an old roady entry was replaced: want Updated, got %+v", reg)
	}
	doc := readSettings(t, dir)
	if doc["permissions"] == nil {
		t.Error("other settings must be preserved")
	}
	hooks := doc["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	if len(pre) != 2 || !strings.Contains(mustJSON(pre[0]), "./lint.sh") {
		t.Errorf("the user's own PreToolUse hook must be kept, first: %s", mustJSON(pre))
	}
	ss := hooks["SessionStart"].([]any)
	if len(ss) != 1 || strings.Contains(mustJSON(ss), "--old") {
		t.Errorf("roady's old SessionStart entry must be replaced, not duplicated: %s", mustJSON(ss))
	}
	if !strings.Contains(mustJSON(hooks["PostToolUse"]), `"matcher":"ExitPlanMode"`) {
		t.Errorf("plan-mode import hook missing: %s", mustJSON(hooks["PostToolUse"]))
	}

	again, err := registerClaudeCodeHooks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Unchanged {
		t.Errorf("re-running setup should change nothing, got %+v", again)
	}

	bad := t.TempDir()
	_ = os.MkdirAll(filepath.Join(bad, ".claude"), 0o755)
	_ = os.WriteFile(filepath.Join(bad, ".claude", "settings.json"), []byte("{not json"), 0o644)
	if _, err := registerClaudeCodeHooks(bad); err == nil {
		t.Error("invalid settings.json must be left alone and reported")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func runHook(t *testing.T, cmd *cobra.Command, payload any) string {
	t.Helper()
	raw, _ := json.Marshal(payload)
	var out bytes.Buffer
	cmd.SetIn(bytes.NewReader(raw))
	cmd.SetOut(&out)
	defer func() { cmd.SetIn(nil); cmd.SetOut(nil) }()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("%s: %v", cmd.Name(), err)
	}
	return out.String()
}

// The done-when of the hooks task: in a fresh project an approved plan-mode
// plan lands in roady with no manual step, the brief reaches the session, and
// creating ROADMAP.md is redirected.
func TestClaudeCodeHooksEndToEnd(t *testing.T) {
	dir, cleanup := withTempDir(t)
	defer cleanup()
	repo := storage.NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	sp := &spec.ProductSpec{ID: "demo", Title: "demo", Version: "0.1.0"}
	_ = repo.SaveSpec(sp)
	_ = repo.SaveSpecLock(sp)
	_ = repo.SaveState(planning.NewExecutionState("demo"))

	// Outside a roady project every hook is silent.
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	if out := runHook(t, hookSessionStartCmd, map[string]any{"cwd": t.TempDir()}); out != "" {
		t.Errorf("a session in another directory must get nothing: %q", out)
	}
	if out := runHook(t, hookGuardWriteCmd, map[string]any{"cwd": t.TempDir(), "tool_input": map[string]any{"file_path": "ROADMAP.md"}}); out != "" {
		t.Errorf("the guard only applies inside a roady project: %q", out)
	}

	plan := "# Plan: Rate limits\n\n## Context\n\nNo limits today.\n\n## Steps\n\n1. Add a token bucket\n2. Wire the middleware\n"
	out := runHook(t, hookPlanApprovedCmd, map[string]any{
		"cwd": dir, "hook_event_name": "PostToolUse", "tool_name": "ExitPlanMode",
		"tool_input": map[string]any{"plan": plan},
	})
	var resp struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("plan-approved output is not hook JSON: %q", out)
	}
	ctx := resp.HookSpecificOutput.AdditionalContext
	if resp.HookSpecificOutput.HookEventName != "PostToolUse" || !strings.Contains(ctx, "2 task(s) created") {
		t.Fatalf("context = %q", ctx)
	}
	if _, err := os.Stat(filepath.Join(dir, ".roady", "plans", "rate-limits.md")); err != nil {
		t.Errorf("the approved plan should be kept under .roady/plans: %v", err)
	}
	p, err := repo.LoadPlan()
	if err != nil || len(p.Tasks) != 2 {
		t.Fatalf("plan tasks: %v %+v", err, p)
	}
	if p.Tasks[0].Source.Doc != ".roady/plans/rate-limits.md" {
		t.Errorf("source = %+v", p.Tasks[0].Source)
	}

	// Approving the same plan again changes nothing.
	out = runHook(t, hookPlanApprovedCmd, map[string]any{"cwd": dir, "tool_input": map[string]any{"plan": plan}})
	if !strings.Contains(out, "already recorded") {
		t.Errorf("re-import: %q", out)
	}

	out = runHook(t, hookSessionStartCmd, map[string]any{"cwd": dir, "source": "compact"})
	if !strings.Contains(out, "task-rate-limits-add-a-token-bucket") {
		t.Errorf("session brief should name the next task: %q", out)
	}

	out = runHook(t, hookGuardWriteCmd, map[string]any{
		"cwd": dir, "tool_name": "Write", "tool_input": map[string]any{"file_path": filepath.Join(dir, "ROADMAP.md")},
	})
	if !strings.Contains(out, `"permissionDecision":"deny"`) || !strings.Contains(out, "roady capture") {
		t.Errorf("ROADMAP.md must be redirected: %q", out)
	}
	out = runHook(t, hookGuardWriteCmd, map[string]any{
		"cwd": dir, "tool_name": "Write", "tool_input": map[string]any{"file_path": filepath.Join(dir, "main.go")},
	})
	if out != "" {
		t.Errorf("ordinary files pass silently: %q", out)
	}
}
