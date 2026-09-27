package application_test

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
)

// The done-when of task-append-only-edits: a task's splits and edits read
// back as history from the event log.
func TestTaskHistoryShowsSplitsAndEdits(t *testing.T) {
	dir := claimsProject(t, "1h", "task-a", "task-b")
	svc, repo := agent(dir)
	capSvc := application.NewCaptureService(repo, application.NewAuditService(repo))
	apply := func(doc application.CaptureDoc, note string) {
		t.Helper()
		res, err := capSvc.Capture(doc, application.CaptureOptions{Actor: "felix", Note: note})
		if err != nil || len(res.Rejected) > 0 {
			t.Fatalf("capture: %v %+v", err, res)
		}
	}
	plan, _ := repo.LoadPlan()
	doc, err := application.EditTaskDoc(plan, "task-a", application.EditTask{Estimate: strPtrT("2d")})
	if err != nil {
		t.Fatal(err)
	}
	apply(doc, "")
	plan, _ = repo.LoadPlan()
	doc, ids, err := application.SplitTaskDoc(plan, "task-a", []string{"Part one", "Part two"}, false)
	if err != nil {
		t.Fatal(err)
	}
	apply(doc, "Split task-a into 2 parts")
	if err := svc.StartTask(t.Context(), ids[0], "codex"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BlockWithReason(ids[0], "spec-conflict", "R1 vs R2", "codex"); err != nil {
		t.Fatal(err)
	}

	hist, err := svc.TaskHistory("task-a")
	if err != nil {
		t.Fatal(err)
	}
	text := application.RenderHistory("task-a", hist)
	for _, want := range []string{`edited: estimate (none) → "2d"`, "depends on (none) → [" + ids[0] + ", " + ids[1] + "] (Split task-a into 2 parts)"} {
		if !strings.Contains(text, want) {
			t.Errorf("history lacks %q:\n%s", want, text)
		}
	}
	hist, _ = svc.TaskHistory(ids[0])
	text = application.RenderHistory(ids[0], hist)
	for _, want := range []string{`created: "Part one", feature f (Split task-a into 2 parts)`, "codex        start", "block (spec-conflict): R1 vs R2"} {
		if !strings.Contains(text, want) {
			t.Errorf("history lacks %q:\n%s", want, text)
		}
	}
	if err := svc.TransitionTask(ids[1], "start", "codex", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransitionTask(ids[1], "complete", "codex", "abc123"); err != nil {
		t.Fatal(err)
	}
	h2, _ := svc.TaskHistory(ids[1])
	if t2 := application.RenderHistory(ids[1], h2); strings.Count(t2, "complete") != 1 || !strings.Contains(t2, "complete: abc123") {
		t.Errorf("a completion is shown once, with its evidence:\n%s", t2)
	}
	if strings.Count(text, "start") != 1 {
		t.Errorf("a start is shown once:\n%s", text)
	}
	if hist, _ := svc.TaskHistory("task-none"); len(hist) != 0 || !strings.Contains(application.RenderHistory("task-none", hist), "No recorded history") {
		t.Error("an unknown task has no history")
	}
}

func strPtrT(s string) *string { return &s }
