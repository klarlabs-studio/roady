package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// A project that renamed its features after finishing work under the old
// names (nexa: 36 such tasks) must not be told to prune its history. Finished
// orphans are info and survive prune; an open orphan is still a problem.
func TestFinishedOrphansAreHistory(t *testing.T) {
	dir := claimsProject(t, "off", "task-a")
	repo := storage.NewFilesystemRepository(dir)
	plan, _ := repo.LoadPlan()
	plan.Tasks = append(plan.Tasks,
		planning.Task{ID: "old-done", Title: "Shipped", FeatureID: "renamed-away"},
		planning.Task{ID: "old-verified", Title: "Proven", FeatureID: "renamed-away"},
		planning.Task{ID: "old-open", Title: "Never started", FeatureID: "renamed-away"},
	)
	_ = repo.SavePlan(plan)
	st, _ := repo.LoadState()
	st.TaskStates["old-done"] = planning.TaskResult{Status: planning.StatusDone}
	st.TaskStates["old-verified"] = planning.TaskResult{Status: planning.StatusVerified}
	_ = repo.SaveState(st)

	audit := application.NewAuditService(repo)
	report, err := application.NewDriftService(repo, audit, nil, application.NewPolicyService(repo)).DetectDrift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sev := map[string]drift.Severity{}
	for _, is := range report.Issues {
		if is.Category == drift.CategoryOrphan {
			sev[is.ComponentID] = is.Severity
		}
	}
	if sev["old-done"] != drift.SeverityInfo || sev["old-verified"] != drift.SeverityInfo {
		t.Errorf("finished orphans should be info: %v", sev)
	}
	if sev["old-open"] != drift.SeverityMedium {
		t.Errorf("an open orphan is still medium: %v", sev)
	}

	if err := application.NewPlanService(repo, audit).PrunePlan(); err != nil {
		t.Fatal(err)
	}
	plan, _ = repo.LoadPlan()
	kept := map[string]bool{}
	for _, tk := range plan.Tasks {
		kept[tk.ID] = true
	}
	if !kept["old-done"] || !kept["old-verified"] || kept["old-open"] || !kept["task-a"] {
		t.Errorf("prune kept %v", kept)
	}
}

func TestStatusWordTitleIsRefused(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	for _, title := range []string{"done", "Done.", " in progress ", "verified", "✓ done"} {
		res := capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-x", Title: str(title), FeatureID: str("base")}}})
		if res.Applied || len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Reason, "roady task complete task-x") {
			t.Errorf("%q: %+v", title, res)
		}
	}
	res := capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{ID: "base", Requirements: []application.CaptureRequirement{{ID: "rq", Title: str("complete")}}}}})
	if res.Applied || len(res.Rejected) != 1 {
		t.Errorf("a requirement titled with a status: %+v", res)
	}
	// Words that merely contain a status are fine.
	res = capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-y", Title: str("Done screen for onboarding"), FeatureID: str("base")}}})
	if !res.Applied {
		t.Errorf("a real title was refused: %+v", res)
	}

	plan := &planning.Plan{Tasks: []planning.Task{{ID: "a", Title: "done"}, {ID: "b", Title: "Real"}, {ID: "c", Title: "Blocked"}}}
	if got := strings.Join(application.StatusTitledTasks(plan), ","); got != "a,c" {
		t.Errorf("status-titled %s", got)
	}
}

func TestImportRoadmapMarkdown(t *testing.T) {
	// nexa's memory/roadmap.md shape: frontmatter, a blockquote, bold-led
	// bullets with continuation lines, and a Done section.
	md := `---
updated: 2026-08-18
---
## Now
> **Positioning:** not a goal.

- **★ VaSt / Belegabruf auto-fill (headline bet)** — still blocked on vendor
  onboarding. The #1 activation lever.
- **ERiC validate-only → passing** — blocked on own Hersteller-ID.

## Next
- ERIC_VALIDIERE the moment the Hersteller-ID lands (PersonB, then S/G/EÜR).
- Anlage V sourcing: unblocked leftovers, see [[nexa-review]].

## Later
- Phase 5: multi-agent / life-event automation.

## Done
- Predicted refund; filing.handoff.

## Notes
- not a goal either
`
	imp, err := application.ImportRoadmapMarkdown(strings.NewReader(md))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]application.CaptureGoal{}
	for _, g := range imp.Doc.Goals {
		got[*g.Title] = g
	}
	vast, ok := got["VaSt / Belegabruf auto-fill (headline bet)"]
	if !ok || *vast.Horizon != "now" || !strings.Contains(*vast.Description, "The #1 activation lever.") {
		t.Fatalf("bold bullet with continuation: %+v", imp.Doc.Goals)
	}
	if g, ok := got["Anlage V sourcing"]; !ok || *g.Horizon != "next" || !strings.Contains(*g.Description, "nexa-review") {
		t.Errorf("colon split and wiki link: %+v", got)
	}
	if g, ok := got["Phase 5: multi-agent / life-event automation"]; !ok || *g.Horizon != "later" {
		t.Errorf("later: %+v", got)
	}
	if g, ok := got["Predicted refund; filing.handoff"]; !ok || *g.Status != "shipped" || g.Horizon != nil {
		t.Errorf("done section: %+v", got)
	}
	if len(imp.Doc.Goals) != 6 || strings.Join(imp.Skipped, ",") != "Notes" {
		t.Errorf("goals %d, skipped %v", len(imp.Doc.Goals), imp.Skipped)
	}

	// Into a project, twice: the second import changes nothing.
	repo := captureRepo()
	if res := capture(t, repo, imp.Doc); !res.Applied || len(repo.Spec.Goals) != 6 {
		t.Fatalf("import: %+v", res)
	}
	if res := capture(t, repo, imp.Doc); res.Changed() {
		t.Errorf("a second import changed %+v", res)
	}
}

// A ROADMAP.md roady rendered reads back into the same goals.
func TestImportRoadmapRoundTrip(t *testing.T) {
	sp := &spec.ProductSpec{Goals: []spec.Goal{
		{ID: "goal-offline", Title: "Offline mode", Horizon: spec.HorizonNow, Milestone: "v2.0", Description: "Works on a plane."},
		{ID: "goal-maybe", Title: "Plugins", Horizon: spec.HorizonLater, Status: spec.GoalIdea},
		{ID: "goal-old", Title: "First release", Status: spec.GoalShipped, Milestone: "v0.1.0"},
		{ID: "goal-no", Title: "Jira parity", Status: spec.GoalOutOfScope, Description: "Not our job."},
	}, Features: []spec.Feature{{ID: "sync", Title: "Sync", Goal: "goal-offline"}}}
	imp, err := application.ImportRoadmapMarkdown(strings.NewReader(application.RenderRoadmapMarkdown(sp)))
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, g := range imp.Doc.Goals {
		line := g.ID
		for _, p := range []*string{g.Horizon, g.Status, g.Milestone, g.Description} {
			if p != nil {
				line += "|" + *p
			}
		}
		lines = append(lines, line)
	}
	want := "goal-offline-mode|now|v2.0|Works on a plane.;goal-plugins|later|idea;goal-first-release|shipped|v0.1.0;goal-jira-parity|out_of_scope|Not our job."
	if strings.Join(lines, ";") != want {
		t.Errorf("round trip:\n got %s\nwant %s", strings.Join(lines, ";"), want)
	}
}

func TestNotesCapturePrompt(t *testing.T) {
	dir := claimsProject(t, "off", "task-a")
	repo := storage.NewFilesystemRepository(dir)
	pol, _ := repo.LoadPolicy()
	pol.AllowAI = true
	_ = repo.SavePolicy(pol)
	sp, _ := repo.LoadSpec()
	sp.Goals = []spec.Goal{{ID: "goal-offline", Title: "Offline mode", Horizon: spec.HorizonNow}}
	_ = repo.SaveSpec(sp)

	notes := filepath.Join(dir, "memory")
	_ = os.MkdirAll(notes, 0o755)
	_ = os.WriteFile(filepath.Join(notes, "decisions.md"), []byte("- 2026-06-09: Tax numbers are deterministic Go, never LLM.\n"), 0o644)
	_ = os.WriteFile(filepath.Join(notes, "open-threads.md"), []byte("- ERiC validate: blocked on Hersteller-ID\n"), 0o644)
	_ = os.WriteFile(filepath.Join(notes, "big.md"), []byte(strings.Repeat("x", 50<<10)), 0o644)
	_ = os.WriteFile(filepath.Join(notes, "skip.txt"), []byte("not markdown"), 0o644)

	files, err := application.ReadNotes(dir, []string{"memory"})
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Path)
	}
	if strings.Join(names, ",") != "memory/big.md,memory/decisions.md,memory/open-threads.md" || !files[0].Truncated || len(files[0].Content) != 40<<10 {
		t.Fatalf("notes %v (truncated %v, %d bytes)", names, files[0].Truncated, len(files[0].Content))
	}

	req, err := application.NewPromptService(repo).NotesToCapture(t.Context(), files)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"=== memory/decisions.md ===", "deterministic Go, never LLM", "=== memory/big.md (truncated) ===", "goal goal-offline: Offline mode", "never a status word"} {
		if !strings.Contains(req.Prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if req.WriteBack != "roady_capture" || !strings.Contains(req.ExpectedFormat, `"decisions"`) {
		t.Errorf("request %+v", req)
	}

	if _, err := application.ReadNotes(dir, []string{"nope.md"}); err == nil {
		t.Error("a missing file should fail")
	}
	pol.AllowAI = false
	_ = repo.SavePolicy(pol)
	if _, err := application.NewPromptService(repo).NotesToCapture(t.Context(), files); err == nil {
		t.Error("allow_ai: false should refuse")
	}
}
