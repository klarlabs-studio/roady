package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/storage"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// runRoady executes the root command with args and returns what it printed.
// Flags are package state in cobra, so each call resets the ones these tests
// touch before running.
func runRoady(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	editDryRun, editJSON, captureDryRun, captureJSON, captureFile = false, false, false, false, ""
	planImportDryRun, planImportJSON, planImportParallel, planImportIncludeDone = false, false, false, false
	planImportFeature, planImportFormat = "", "auto"
	addID, addReq, addFeature, addDesc, addPriority, addEstimate, addGoal = "", "", "", "", "", "", ""
	addAfter, addBefore, addCheckRun, addCheckManual = nil, nil, "", ""
	splitSequential = false
	taskAcceptAllDone, taskAcceptReason = false, ""
	doctorFix = false
	taskListStatus, taskListLimit, taskQueryJSON = "all", 50, false
	goalImportSections, goalImportSectionGoals = nil, nil
	gitSuggestLimit, gitSuggestJSON = 50, false
	moveReq, moveFeature = "", ""
	goalID, goalDesc, goalHorizon, goalStatus, goalMilestone, goalTitle = "", "", "", "", "", ""
	goalFeatures, goalListJSON, taskHistoryJSON, statsJSON = nil, false, false, false
	goalRenderOut, goalRenderCheck, goalRenderForce = "", false, false
	decideID, decideChoice, decideContext, decideConsequences, decideSupersedes = "", "", "", "", ""
	decideGoals, decideFeatures, decideReqs, decideList = nil, nil, nil, false
	for _, c := range []*cobra.Command{goalAddCmd, goalEditCmd, goalListCmd, goalRenderCmd, goalImportCmd, decideCmd} {
		c.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	}
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

// The roadmap from the terminal: add goals at each horizon, link a feature,
// move a goal, ship one, and read it back.
func TestGoalCommands(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "roadmap"); err != nil {
		t.Fatalf("init: %v", err)
	}
	doc := "features:\n  - id: sync\n    title: Sync\n"
	if _, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatalf("capture: %v", err)
	}
	for _, args := range [][]string{
		{"goal", "add", "Offline mode", "--horizon", "next", "--feature", "sync"},
		{"goal", "add", "Plugin marketplace", "--status", "idea", "--horizon", "later"},
		{"goal", "add", "Jira parity", "--status", "out-of-scope"},
		{"goal", "edit", "goal-offline-mode", "--horizon", "now", "--milestone", "v2"},
	} {
		if out, err := runRoady(t, "", args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	out, err := runRoady(t, "", "goal", "list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Now\n  goal-offline-mode  Offline mode  (v2", "features: sync", "Later\n  goal-plugin-marketplace", "Out of scope\n  goal-jira-parity"} {
		if !strings.Contains(out, want) {
			t.Errorf("goal list lacks %q:\n%s", want, out)
		}
	}
	if out, err = runRoady(t, "", "goal", "list", "--json"); err != nil || !strings.Contains(out, `"name": "Now"`) {
		t.Errorf("goal list --json: %v\n%s", err, out)
	}
	if _, err = runRoady(t, "", "goal", "edit", "goal-nope", "--horizon", "now"); err == nil {
		t.Error("editing a missing goal should fail")
	}
}

// ROADMAP.md from the goals: written, round-tripped by --check, a hand edit
// reported by drift and refused by render until --force.
func TestGoalRender(t *testing.T) {
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "roadmap"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := runRoady(t, "", "goal", "add", "Offline mode", "--horizon", "now", "--description", "Work without a network."); err != nil {
		t.Fatal(err)
	}
	if _, err := runRoady(t, "", "goal", "render", "--check"); err == nil {
		t.Error("--check must fail before the roadmap is rendered")
	}
	out, err := runRoady(t, "", "goal", "render")
	if err != nil || !strings.Contains(out, "Wrote") {
		t.Fatalf("render: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "goal", "render", "--check"); err != nil {
		t.Fatalf("--check after render: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "goal", "render", "--out", "-"); err != nil || !strings.Contains(out, "### Offline mode") {
		t.Errorf("--out -: %v\n%s", err, out)
	}

	path := filepath.Join(dir, "ROADMAP.md")
	raw, _ := os.ReadFile(path)
	_ = os.WriteFile(path, []byte(strings.Replace(string(raw), "Work without a network.", "Work offline, always.", 1)), 0o644)
	if out, _ = runRoady(t, "", "drift", "detect"); !strings.Contains(out, "edited by hand") {
		t.Errorf("drift did not report the hand edit:\n%s", out)
	}
	if _, err = runRoady(t, "", "goal", "render"); err == nil || !strings.Contains(err.Error(), "edited by hand") {
		t.Errorf("render must refuse to drop a hand edit: %v", err)
	}
	if _, err = runRoady(t, "", "goal", "render", "--force"); err != nil {
		t.Errorf("render --force: %v", err)
	}
	if out, _ = runRoady(t, "", "drift", "detect"); strings.Contains(out, "ROADMAP") {
		t.Errorf("a fresh render still reads as drift:\n%s", out)
	}
}

// Starting a task claims it; renew keeps the claim, and someone else is
// refused while it holds.
func TestTaskClaimFromTheTerminal(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "claims"); err != nil {
		t.Fatal(err)
	}
	doc := "features:\n  - id: f\n    title: F\n    requirements:\n      - id: r\n        title: R\n"
	if _, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatal(err)
	}
	if _, err := runRoady(t, "", "plan", "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err := runRoady(t, "", "task", "renew", "task-r"); err == nil {
		t.Error("renewing a task that is not started should fail")
	}
	if _, err := runRoady(t, "", "task", "start", "task-r"); err != nil {
		t.Fatal(err)
	}
	out, err := runRoady(t, "", "task", "renew", "task-r")
	if err != nil || !strings.Contains(out, "renewed until") {
		t.Fatalf("renew: %v\n%s", err, out)
	}
	if out, _ = runRoady(t, "", "next"); !strings.Contains(out, "Claim: yours until") {
		t.Errorf("next does not show the claim:\n%s", out)
	}
	t.Setenv("ROADY_USER", "someone-else")
	if _, err = runRoady(t, "", "task", "start", "task-r"); err == nil || !strings.Contains(err.Error(), "claimed by tester") {
		t.Errorf("a second agent took the task: %v", err)
	}
}

// The done-when of task-drift-reruns-checks: break the code behind a
// verified task and drift --checks reports it.
func TestDriftChecksReportsARegression(t *testing.T) {
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "regress"); err != nil {
		t.Fatal(err)
	}
	doc := "features:\n  - id: f\n    title: F\n    requirements:\n      - id: r\n        title: R\n        check:\n          run: test -f built.txt\n"
	if _, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "built.txt"), []byte("ok"), 0o644)
	for _, args := range [][]string{
		{"plan", "approve"}, {"task", "start", "task-r"}, {"task", "complete", "task-r", "--evidence", "abc123"}, {"task", "verify", "task-r", "--override", "test fixture"},
	} {
		if out, err := runRoady(t, "", args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	driftChecks = false
	if out, err := runRoady(t, "", "drift", "detect", "--checks"); err != nil || strings.Contains(out, "REGRESSION") {
		t.Fatalf("a passing verified task reported drift: %v\n%s", err, out)
	}

	_ = os.Remove(filepath.Join(dir, "built.txt"))
	driftChecks = false
	out, err := runRoady(t, "", "drift", "detect", "--checks")
	if err == nil || !strings.Contains(out, "REGRESSION") || !strings.Contains(out, "task-r is verified, but its acceptance check fails now") {
		t.Errorf("the regression was not reported: %v\n%s", err, out)
	}
	driftChecks = false
	if out, _ = runRoady(t, "", "drift", "detect"); strings.Contains(out, "REGRESSION") {
		t.Errorf("without --checks nothing is re-run:\n%s", out)
	}
}

// The honest exit from the terminal: block with a reason, see it in status
// and drift, then resolve it.
func TestBlockWithASpecConflict(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "conflict"); err != nil {
		t.Fatal(err)
	}
	doc := "features:\n  - id: f\n    title: F\n    requirements:\n      - id: r\n        title: R\n"
	if _, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"plan", "approve"}, {"task", "start", "task-r"},
		{"task", "block", "task-r", "--reason", "spec-conflict", "-e", "R wants two things at once"}} {
		if out, err := runRoady(t, "", args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if out, _ := runRoady(t, "", "status"); !strings.Contains(out, "Needs a decision (1)") || !strings.Contains(out, "task-r [spec-conflict] R wants two things at once") {
		t.Errorf("status:\n%s", out)
	}
	if out, _ := runRoady(t, "", "drift", "detect"); !strings.Contains(out, "CONFLICT") {
		t.Errorf("drift:\n%s", out)
	}
	if _, err := runRoady(t, "", "task", "unblock", "task-r"); err != nil {
		t.Fatal(err)
	}
	if out, _ := runRoady(t, "", "status"); strings.Contains(out, "Needs a decision") {
		t.Errorf("resolved, yet still listed:\n%s", out)
	}
}

// The done-when of task-unplanned-tasks: `roady add` with no --req succeeds,
// drift classifies it as unplanned (informational, not failing), and prune
// keeps it.
func TestUnplannedTask(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "inbox"); err != nil {
		t.Fatal(err)
	}
	doc := "goals:\n  - id: goal-hygiene\n    title: Hygiene\n    horizon: now\nfeatures:\n  - id: f\n    title: F\n    requirements:\n      - id: r\n        title: R\n"
	if _, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatal(err)
	}
	if out, err := runRoady(t, "", "add", "Fix the typo in the README"); err != nil || !strings.Contains(out, "task-fix-the-typo-in-the-readme") {
		t.Fatalf("add without --req: %v\n%s", err, out)
	}
	if out, err := runRoady(t, "", "add", "Bump dependencies", "--goal", "goal-hygiene"); err != nil {
		t.Fatalf("add --goal: %v\n%s", err, out)
	}

	driftChecks = false
	out, err := runRoady(t, "", "drift", "detect")
	if err != nil {
		t.Errorf("unplanned work must not fail drift: %v\n%s", err, out)
	}
	if !strings.Contains(out, "(plan/UNPLANNED)") || !strings.Contains(out, "unplanned work in the inbox") ||
		!strings.Contains(out, "unplanned work in goal goal-hygiene") || strings.Contains(out, "ORPHAN") {
		t.Errorf("drift:\n%s", out)
	}
	if out, _ = runRoady(t, "", "goal", "list"); !strings.Contains(out, "goal-hygiene  Hygiene  (0/1 tasks done)") {
		t.Errorf("the goal does not count its unplanned task:\n%s", out)
	}
	if _, err := runRoady(t, "", "plan", "prune"); err != nil {
		t.Fatal(err)
	}
	if out, _ = runRoady(t, "", "status"); !strings.Contains(out, "Fix the typo in the README") {
		t.Errorf("prune dropped unplanned work:\n%s", out)
	}
}

// The done-when of task-decisions: `roady decide` records one and `roady
// next` shows it for the active task.
func TestDecideAndNext(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "decisions"); err != nil {
		t.Fatal(err)
	}
	doc := "features:\n  - id: auth\n    title: Auth\n    requirements:\n      - id: jwt\n        title: JWT sessions\n"
	if _, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatal(err)
	}
	if _, err := runRoady(t, "", "plan", "approve"); err != nil {
		t.Fatal(err)
	}
	out, err := runRoady(t, "", "decide", "Session storage", "--choice", "Signed cookies, no server sessions",
		"--context", "Stateless deploys", "--req", "jwt")
	if err != nil || !strings.Contains(out, "decision-session-storage") {
		t.Fatalf("decide: %v\n%s", err, out)
	}
	if _, err := runRoady(t, "", "task", "start", "task-jwt"); err != nil {
		t.Fatal(err)
	}
	if out, _ = runRoady(t, "", "next"); !strings.Contains(out, "Decided: Session storage — Signed cookies, no server sessions (decision-session-storage)") {
		t.Errorf("next does not show the decision:\n%s", out)
	}
	if out, _ = runRoady(t, "", "decide", "--list"); !strings.Contains(out, "applies to requirement jwt") {
		t.Errorf("decide --list:\n%s", out)
	}
}

// doctor judges the audit log as `roady audit verify` does: an entry this
// build cannot check is history, an edited one is a problem.
func TestDoctorAuditJudgement(t *testing.T) {
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "doctor"); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(dir, ".roady", "events.jsonl")
	f, _ := os.OpenFile(events, os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString(`{"id":"legacy-1","action":"old.entry","actor":"someone","timestamp":"2025-01-01T00:00:00Z"}` + "\n")
	_ = f.Close()
	if out, err := runRoady(t, "", "doctor"); err != nil || !strings.Contains(out, "cannot be checked by this build") {
		t.Errorf("an unhashed legacy entry is not a failure: %v\n%s", err, out)
	}

	raw, _ := os.ReadFile(events)
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	lines[0] = strings.Replace(lines[0], `"actor":"`, `"actor":"x`, 1)
	_ = os.WriteFile(events, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
	if out, err := runRoady(t, "", "doctor"); err == nil || !strings.Contains(out, "may mean the log was altered") {
		t.Errorf("an edited entry must fail doctor: %v\n%s", err, out)
	}
}

// History from the terminal, and the log that carries it still verifies:
// nested change records must hash the same after a reload.
func TestTaskHistoryFromTheTerminal(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "history"); err != nil {
		t.Fatal(err)
	}
	doc := "features:\n  - id: f\n    title: F\n    requirements:\n      - id: r\n        title: R\n"
	for _, step := range []struct {
		stdin string
		args  []string
	}{
		{doc, []string{"capture"}},
		{"", []string{"edit", "task-r", "--estimate", "4h"}},
		{"", []string{"split", "task-r", "One", "Two"}},
	} {
		if out, err := runRoady(t, step.stdin, step.args...); err != nil {
			t.Fatalf("%v: %v\n%s", step.args, err, out)
		}
	}
	out, err := runRoady(t, "", "task", "history", "task-r")
	if err != nil || !strings.Contains(out, `edited: estimate (none) → "4h"`) || !strings.Contains(out, "Split task-r into 2 parts") {
		t.Errorf("history: %v\n%s", err, out)
	}
	if out, err := runRoady(t, "", "audit", "verify"); err != nil || strings.Contains(out, "altered") {
		t.Errorf("the log with change records does not verify: %v\n%s", err, out)
	}
}

func TestStatsCommand(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "stats"); err != nil {
		t.Fatal(err)
	}
	out, err := runRoady(t, "", "stats")
	if err != nil || !strings.Contains(out, "Plans captured automatically") || !strings.Contains(out, "Sessions resumed the right task") {
		t.Errorf("stats: %v\n%s", err, out)
	}
	if out, err = runRoady(t, "", "stats", "--json"); err != nil || !strings.Contains(out, `"verified_with_passing_check"`) {
		t.Errorf("stats --json: %v\n%s", err, out)
	}
}
