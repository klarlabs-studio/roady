package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/storage"
)

// runRoady executes the root command with args and returns what it printed.
// Flags are package state in cobra, so each call resets the ones these tests
// touch before running.
func runRoady(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	editDryRun, editJSON, captureDryRun, captureJSON, captureFile = false, false, false, false, ""
	planImportDryRun, planImportJSON, planImportParallel, planImportIncludeDone = false, false, false, false
	planImportFeature, planImportFormat = "", "auto"
	addID, addReq, addFeature, addDesc, addPriority, addEstimate = "", "", "", "", "", ""
	addAfter, addBefore, addCheckRun, addCheckManual = nil, nil, "", ""
	splitSequential = false
	moveReq, moveFeature = "", ""
	var err error
	out := captureStdout(t, func() {
		RootCmd.SetIn(strings.NewReader(stdin))
		RootCmd.SetOut(os.Stdout)
		RootCmd.SetArgs(args)
		err = RootCmd.Execute()
		RootCmd.SetIn(nil)
		RootCmd.SetOut(nil)
	})
	return out, err
}

// The loop a person drives from the terminal: capture, add/edit/split/move,
// import a plan, start, check, verify — each through the real commands.
func TestAgentLoopThroughTheCLI(t *testing.T) {
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")

	if _, err := runRoady(t, "", "init", "loop"); err != nil {
		t.Fatalf("init: %v", err)
	}

	doc := `features:
  - id: pdf
    title: PDF export
    requirements:
      - id: pdf-gen
        title: Generate a PDF
        priority: high
        check:
          run: "true"
`
	out, err := runRoady(t, doc, "capture", "--dry-run")
	if err != nil || !strings.Contains(out, "Dry run") {
		t.Fatalf("capture --dry-run: %v\n%s", err, out)
	}
	out, err = runRoady(t, doc, "capture", "--json")
	if err != nil || !strings.Contains(out, `"task:task-pdf-gen"`) {
		t.Fatalf("capture: %v\n%s", err, out)
	}

	if out, err = runRoady(t, "", "add", "Handle an empty file", "--after", "task-pdf-gen", "-p", "high"); err != nil || !strings.Contains(out, "task-handle-an-empty-file") {
		t.Fatalf("add: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "edit", "task-handle-an-empty-file", "--estimate", "2h", "--json"); err != nil || !strings.Contains(out, `"task:task-handle-an-empty-file"`) {
		t.Fatalf("edit: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "split", "task-handle-an-empty-file", "Detect it", "Say so", "--sequential"); err != nil || !strings.Contains(out, "Split task-handle-an-empty-file into 2 parts") {
		t.Fatalf("split: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "move", "task-handle-an-empty-file", "--feature", "pdf", "--dry-run"); err != nil {
		t.Fatalf("move: %v\n%s", err, out)
	}

	plan := filepath.Join(dir, "plan.md")
	_ = os.WriteFile(plan, []byte("# Plan: Fonts\n\n## Steps\n\n1. Embed fonts\n2. Subset them\n"), 0o600)
	if out, err = runRoady(t, "", "plan", "import", plan, "--feature", "pdf", "--dry-run", "--json"); err != nil || !strings.Contains(out, `"format": "markdown"`) {
		t.Fatalf("plan import --dry-run: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "plan", "import", plan, "--feature", "pdf", "--parallel"); err != nil || !strings.Contains(out, "2 step(s)") {
		t.Fatalf("plan import: %v\n%s", err, out)
	}

	if _, err = runRoady(t, "", "plan", "approve"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, err = runRoady(t, "", "task", "start", "task-pdf-gen"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if out, err = runRoady(t, "", "task", "check", "task-pdf-gen"); err != nil || !strings.Contains(out, "PASSED") {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "next"); err != nil || !strings.Contains(out, "task-pdf-gen") {
		t.Fatalf("next: %v\n%s", err, out)
	}

	repo := storage.NewFilesystemRepository(dir)
	p, err := repo.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, task := range p.Tasks {
		ids[task.ID] = true
	}
	for _, want := range []string{"task-pdf-gen", "task-handle-an-empty-file", "task-handle-an-empty-file-detect-it", "task-fonts-embed-fonts"} {
		if !ids[want] {
			t.Errorf("plan lacks %s: %v", want, ids)
		}
	}

	if out, err = runRoady(t, "", "state", "rebuild"); err != nil {
		t.Fatalf("state rebuild: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "audit", "trail", "task-pdf-gen"); err != nil || out == "" {
		t.Fatalf("audit trail: %v\n%s", err, out)
	}
}

// setup all writes every agent's files; running it twice changes nothing.
func TestSetupAllTargets(t *testing.T) {
	dir, cleanup := withTempDir(t)
	defer cleanup()
	t.Setenv("HOME", t.TempDir())

	if _, err := runRoady(t, "", "setup", "all"); err != nil {
		t.Fatalf("setup all: %v", err)
	}
	for _, f := range []string{".mcp.json", ".claude/settings.json", "CLAUDE.md", ".codex/config.toml", ".gemini/settings.json",
		".cursor/hooks.json", "opencode.json", ".github/hooks/roady.json", ".kiro/settings/mcp.json", "AGENTS.md", "GEMINI.md",
		".agents/skills/roady-planning/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("setup all did not write %s", f)
		}
	}
	before, _ := os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	out, err := runRoady(t, "", "setup", "all")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	if string(before) != string(after) || strings.Contains(out, "Created ") {
		t.Errorf("second setup changed files:\n%s", out)
	}
	var mcp map[string]any
	raw, _ := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if json.Unmarshal(raw, &mcp) != nil || mcp["mcpServers"] == nil {
		t.Errorf(".mcp.json = %s", raw)
	}

	if _, err := runRoady(t, "", "setup", "openai", "--no-instructions"); err != nil {
		t.Errorf("setup openai (Codex alias): %v", err)
	}
	setupNoInstructions = false
	if _, err := runRoady(t, "", "setup", "nope"); err == nil {
		t.Error("an unknown target must be an error")
	}
}
