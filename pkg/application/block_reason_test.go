package application_test

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// The honest exit: an agent that cannot do a task as specified blocks it
// with a reason, and the project says a person has to decide — in drift, in
// the brief — until someone does.
func TestSpecConflictRaisesDriftUntilResolved(t *testing.T) {
	dir := claimsProject(t, "1h", "task-a", "task-b")
	svc, repo := agent(dir)
	if err := svc.StartTask(t.Context(), "task-a", "codex"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BlockWithReason("task-a", "spec-conflict", "", "codex"); err == nil || !strings.Contains(err.Error(), "say what is wrong") {
		t.Errorf("a spec-conflict needs a detail: %v", err)
	}
	if err := svc.BlockWithReason("task-a", "whatever", "x", "codex"); err == nil {
		t.Error("an unknown reason must be refused")
	}
	if err := svc.BlockWithReason("task-a", "spec-conflict", "R2 requires sync writes, R5 forbids them", "codex"); err != nil {
		t.Fatal(err)
	}

	driftSvc := application.NewDriftService(repo, application.NewAuditService(repo), nil, application.NewPolicyService(repo))
	report, err := driftSvc.DetectDrift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var conflict *drift.Issue
	for i := range report.Issues {
		if report.Issues[i].Category == drift.CategoryConflict {
			conflict = &report.Issues[i]
		}
	}
	if conflict == nil || conflict.Type != drift.DriftTypeSpec || conflict.Severity != drift.SeverityHigh ||
		!strings.Contains(conflict.Message, "R2 requires sync writes") || !strings.Contains(conflict.Message, "codex") {
		t.Fatalf("conflict drift %+v in %+v", conflict, report.Issues)
	}

	brief, _ := svc.Brief(t.Context(), "someone")
	if len(brief.NeedsDecision) != 1 || !strings.Contains(brief.Render(), "wait on a person's decision") {
		t.Errorf("brief does not surface it: %+v\n%s", brief.NeedsDecision, brief.Render())
	}
	if nd := application.RenderNeedsDecisions(brief.NeedsDecision); !strings.Contains(nd, "task-a [spec-conflict] R2 requires") {
		t.Errorf("render: %s", nd)
	}

	// cannot-complete is a plan issue; an ordinary block is not drift.
	if err := svc.StartTask(t.Context(), "task-b", "codex"); err != nil {
		t.Fatal(err)
	}
	if err := svc.BlockWithReason("task-b", "cannot-complete", "needs production credentials", "codex"); err != nil {
		t.Fatal(err)
	}
	report, _ = driftSvc.DetectDrift(t.Context())
	kinds := map[drift.DriftType]int{}
	for _, i := range report.Issues {
		if i.Category == drift.CategoryConflict {
			kinds[i.Type]++
		}
	}
	if kinds[drift.DriftTypeSpec] != 1 || kinds[drift.DriftTypePlan] != 1 {
		t.Errorf("conflict issues by type: %v", kinds)
	}

	// A person resolves it: unblocking clears the drift.
	if err := svc.TransitionTask("task-a", "unblock", "felix", ""); err != nil {
		t.Fatal(err)
	}
	st, _ := repo.LoadState()
	if st.TaskStates["task-a"].Block != nil || st.GetTaskStatus("task-a") != planning.StatusPending {
		t.Errorf("after unblock: %+v", st.TaskStates["task-a"])
	}
	if len(application.NeedsDecisions(st)) != 1 {
		t.Error("only task-b should still need a decision")
	}
	events, _ := repo.LoadEvents()
	found := false
	for _, e := range events {
		found = found || (e.Metadata["kind"] == "spec-conflict" && e.Metadata["task_id"] == "task-a")
	}
	if !found {
		t.Error("the spec-conflict block is not in the audit trail")
	}
}
