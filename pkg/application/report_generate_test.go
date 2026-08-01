package application

import (
	"context"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// newSeededRepo builds a real filesystem repository with a plan, state, and
// event log. Exercising Generate/BuildTrail against the real repository is
// what makes these tests worth having: the interesting failures live in how
// the services compose the repo, not in the pure helpers.
func newSeededRepo(t *testing.T) *storage.FilesystemRepository {
	t.Helper()

	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "spec-1", Title: "Checkout", Version: "0.1.0",
		Features: []spec.Feature{{
			ID: "f1", Title: "Signup",
			Requirements: []spec.Requirement{{ID: "r1", Title: "Email signup"}},
		}},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID: "plan-1", SpecID: "spec-1", ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "Wire signup", FeatureID: "f1", Priority: planning.PriorityHigh,
				Origin: planning.OriginHeuristic, Source: planning.TaskSource{Doc: "docs/spec.md", Line: 12}},
			{ID: "task-2", Title: "Refunds", FeatureID: "f1"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{
		Status: planning.StatusDone, Owner: "alice", Evidence: []string{"commit abc123"},
	}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	audit := NewAuditService(repo)
	for _, e := range []struct {
		action, actor string
		meta          map[string]any
	}{
		{"task.started", "alice", map[string]any{"task_id": "task-1", "session_id": "s1", "agent": "claude-code"}},
		{"task.completed", "system", map[string]any{"task_id": "task-1", "evidence": "commit abc123", "session_id": "s1", "agent": "claude-code"}},
		{"task.assign", "cli", map[string]any{"task_id": "task-2", "assignee": "bob"}},
	} {
		if err := audit.Log(e.action, e.actor, e.meta); err != nil {
			t.Fatalf("log %s: %v", e.action, err)
		}
	}

	return repo
}

func newReportServices(t *testing.T, repo domain.WorkspaceRepository) *ReportService {
	t.Helper()
	audit := NewAuditService(repo)
	plan := NewPlanService(repo, audit)
	policy := NewPolicyService(repo)
	drift := NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	return NewReportService(plan, nil, drift, nil, nil)
}

func TestReportServiceGenerate(t *testing.T) {
	repo := newSeededRepo(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	rep, err := newReportServices(t, repo).Generate(context.Background(), ReportOptions{
		Project: "checkout",
		Now:     now,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if rep.Project != "checkout" || !rep.GeneratedAt.Equal(now) {
		t.Errorf("header wrong: %s / %v", rep.Project, rep.GeneratedAt)
	}
	if rep.Progress.Total != 2 || rep.Progress.Done != 1 {
		t.Errorf("progress = %+v, want 2 total / 1 done", rep.Progress)
	}
	if rep.Progress.Percent != 50 {
		t.Errorf("percent = %v, want 50", rep.Progress.Percent)
	}

	// alice owns the done task; task-2 is unassigned in state.json.
	var owners []string
	for _, a := range rep.Assignments {
		if a.Unassigned {
			owners = append(owners, "(unassigned)")
			continue
		}
		owners = append(owners, a.Owner)
	}
	if len(owners) == 0 {
		t.Error("expected assignments to be grouped")
	}

	// A nil forecast/debt service must omit sections, not fail the report.
	if rep.Forecast != nil {
		t.Error("forecast should be omitted when the service is absent")
	}
}

func TestReportServiceGenerateWithSince(t *testing.T) {
	repo := newSeededRepo(t)
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	rep, err := newReportServices(t, repo).Generate(context.Background(), ReportOptions{
		Project: "checkout", Since: since, MaxChanges: 2,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if rep.Since == nil || !rep.Since.Equal(since) {
		t.Errorf("Since = %v, want %v", rep.Since, since)
	}
	if len(rep.Changes) > 2 {
		t.Errorf("MaxChanges not honoured: got %d", len(rep.Changes))
	}
	// task.assign is bookkeeping and must not appear in a stakeholder report.
	for _, c := range rep.Changes {
		if c.Action == "task.assign" {
			t.Error("assignment noise leaked into the change log")
		}
	}
}

func TestAuditTrailServiceBuildTrail(t *testing.T) {
	repo := newSeededRepo(t)
	audit := NewAuditService(repo)
	plan := NewPlanService(repo, audit)

	svc := NewAuditTrailService(nil, audit, plan, repo)

	trail, err := svc.BuildTrail(context.Background(), TrailQuery{
		TaskID: "task-1",
		Now:    time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("BuildTrail: %v", err)
	}

	if trail.Subject.Kind != "task" || trail.Subject.ID != "task-1" {
		t.Errorf("subject = %+v", trail.Subject)
	}
	if len(trail.Entries) != 2 {
		t.Fatalf("expected the 2 task-1 events, got %d", len(trail.Entries))
	}
	if !trail.Integrity.CheckedChain || !trail.Integrity.Verified {
		t.Errorf("a freshly written log should verify: %+v", trail.Integrity)
	}

	if trail.Task == nil {
		t.Fatal("task facts missing")
	}
	if trail.Task.Status != "done" || trail.Task.Owner != "alice" {
		t.Errorf("task facts = %+v", trail.Task)
	}
	if trail.Task.SourceDoc != "docs/spec.md" || trail.Task.SourceLine != 12 {
		t.Errorf("spec citation lost: %+v", trail.Task)
	}
	if !trail.HasEvidence() {
		t.Error("evidence should be present")
	}
	if len(trail.Findings()) != 0 {
		t.Errorf("a complete, evidenced, verified task should have no findings: %v", trail.Findings())
	}
}

func TestAuditTrailServiceFiltersByAgent(t *testing.T) {
	repo := newSeededRepo(t)
	audit := NewAuditService(repo)
	svc := NewAuditTrailService(nil, audit, NewPlanService(repo, audit), repo)

	trail, err := svc.BuildTrail(context.Background(), TrailQuery{Agent: "claude-code"})
	if err != nil {
		t.Fatalf("BuildTrail: %v", err)
	}

	if trail.Subject.Kind != "agent" {
		t.Errorf("subject kind = %q, want agent", trail.Subject.Kind)
	}
	for _, e := range trail.Entries {
		if e.Agent != "claude-code" {
			t.Errorf("entry from %q leaked into a claude-code trail", e.Agent)
		}
	}
	if len(trail.Entries) != 2 {
		t.Errorf("expected 2 claude-code entries, got %d", len(trail.Entries))
	}

	// The unattributed task.assign event must not be counted as this agent's.
	if trail.Unattributed() != 0 {
		t.Errorf("agent-filtered trail should contain no unattributed entries")
	}
}

func TestAuditTrailServiceUnknownTask(t *testing.T) {
	repo := newSeededRepo(t)
	audit := NewAuditService(repo)
	svc := NewAuditTrailService(nil, audit, NewPlanService(repo, audit), repo)

	trail, err := svc.BuildTrail(context.Background(), TrailQuery{TaskID: "nope"})
	if err != nil {
		t.Fatalf("BuildTrail: %v", err)
	}

	if len(trail.Entries) != 0 || trail.Task != nil {
		t.Errorf("expected an empty trail for an unknown task")
	}
	findings := trail.Findings()
	if len(findings) == 0 {
		t.Error("an empty trail should say so as a finding")
	}
}
