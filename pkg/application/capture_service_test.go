package application_test

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func str(s string) *string { return &s }

func captureRepo() *MockRepo {
	return &MockRepo{Spec: &spec.ProductSpec{ID: "s", Title: "S", Features: []spec.Feature{{ID: "base", Title: "Base"}}}}
}

func capture(t *testing.T, repo *MockRepo, doc application.CaptureDoc) *application.CaptureResult {
	t.Helper()
	svc := application.NewCaptureService(repo, application.NewAuditService(repo))
	res, err := svc.Capture(doc, application.CaptureOptions{Actor: "agent"})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	return res
}

func wholePlan() application.CaptureDoc {
	deps := []string{"task-seq"}
	return application.CaptureDoc{
		Features: []application.CaptureFeature{{
			ID: "inv", Title: str("Invoices"),
			Requirements: []application.CaptureRequirement{
				{ID: "seq", Title: str("Gap-free numbers"), Priority: str("high"), Check: &spec.Check{Run: "go test ./inv"}},
				{ID: "pdf", Title: str("PDF output")},
			},
		}},
		Tasks: []application.CaptureTask{{ID: "task-migrate", Title: str("Migrate numbers"), Requirement: str("seq"), DependsOn: &deps}},
	}
}

func TestCaptureWholePlanInOneCall(t *testing.T) {
	repo := captureRepo()
	res := capture(t, repo, wholePlan())
	if !res.Applied || len(res.Rejected) != 0 {
		t.Fatalf("expected an applied capture, got %+v", res)
	}
	want := []string{"feature:inv", "requirement:pdf", "requirement:seq", "task:task-migrate", "task:task-pdf", "task:task-seq"}
	if strings.Join(res.Created, ",") != strings.Join(want, ",") {
		t.Fatalf("created %v, want %v", res.Created, want)
	}
	var seq, migrate planning.Task
	for _, task := range repo.Plan.Tasks {
		switch task.ID {
		case "task-seq":
			seq = task
		case "task-migrate":
			migrate = task
		}
	}
	if seq.Check == nil || seq.Check.Run != "go test ./inv" || seq.FeatureID != "inv" {
		t.Errorf("derived task did not carry the requirement's check and feature: %+v", seq)
	}
	if migrate.FeatureID != "inv" || len(migrate.DependsOn) != 1 {
		t.Errorf("explicit task not linked through its requirement: %+v", migrate)
	}
	if repo.Plan.ApprovalStatus != planning.ApprovalPending {
		t.Errorf("a new plan starts pending, got %s", repo.Plan.ApprovalStatus)
	}
}

func TestCaptureIsIdempotent(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	repo.Plan.ApprovalStatus = planning.ApprovalApproved
	plan := repo.Plan

	res := capture(t, repo, wholePlan())
	if res.Applied || res.Changed() {
		t.Fatalf("re-sending the same document must change nothing, got %+v", res)
	}
	if repo.Plan != plan || repo.Plan.ApprovalStatus != planning.ApprovalApproved {
		t.Fatal("a no-op capture rewrote the plan or reset its approval")
	}
}

// A partial capture edits only the fields it names. Dependencies set on a task
// directly survive an unrelated edit to its requirement — roady's own plan
// carries such edges.
func TestCapturePartialEditKeepsTheRest(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	deps := []string{"task-pdf"}
	capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-seq", DependsOn: &deps}}})

	res := capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{
		ID: "inv", Requirements: []application.CaptureRequirement{{ID: "seq", Description: str("per calendar year")}},
	}}})
	if strings.Join(res.Updated, ",") != "requirement:seq,task:task-seq" {
		t.Fatalf("updated %v", res.Updated)
	}
	for _, task := range repo.Plan.Tasks {
		if task.ID == "task-seq" {
			if len(task.DependsOn) != 1 || task.DependsOn[0] != "task-pdf" {
				t.Errorf("dependency set on the task was wiped: %v", task.DependsOn)
			}
			if task.Description != "per calendar year" || task.Priority != "high" {
				t.Errorf("edit not applied or other fields lost: %+v", task)
			}
		}
	}
}

// All or nothing: one bad item means nothing is written.
func TestCaptureRejectsAtomically(t *testing.T) {
	repo := captureRepo()
	missing := []string{"task-nope"}
	res := capture(t, repo, application.CaptureDoc{
		Features: []application.CaptureFeature{{ID: "f", Title: str("F")}},
		Tasks:    []application.CaptureTask{{ID: "t", Title: str("T"), FeatureID: str("f"), DependsOn: &missing}},
	})
	if res.Applied || len(res.Rejected) == 0 {
		t.Fatalf("expected a rejection, got %+v", res)
	}
	if repo.Plan != nil || len(repo.Spec.Features) != 1 {
		t.Fatal("a rejected capture wrote something")
	}
}

func TestCaptureRejections(t *testing.T) {
	cycleA, cycleB := []string{"b"}, []string{"a"}
	tests := []struct {
		name   string
		doc    application.CaptureDoc
		reason string
	}{
		{"cycle", application.CaptureDoc{Tasks: []application.CaptureTask{
			{ID: "a", Title: str("A"), FeatureID: str("base"), DependsOn: &cycleA},
			{ID: "b", Title: str("B"), FeatureID: str("base"), DependsOn: &cycleB},
		}}, "cycle"},
		{"unknown goal", application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "a", Title: str("A"), Goal: str("goal-missing")}}}, "goal \"goal-missing\" does not exist"},
		{"unknown feature", application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "a", Title: str("A"), FeatureID: str("zzz")}}}, "does not exist"},
		{"untitled", application.CaptureDoc{Features: []application.CaptureFeature{{ID: "new"}}}, "needs a title"},
		{"bad priority", application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "a", Title: str("A"), FeatureID: str("base"), Priority: str("urgent")}}}, "priority"},
		{"bad check", application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "a", Title: str("A"), FeatureID: str("base"), Check: &planning.Check{Run: "x", Manual: "y"}}}}, "either run or manual"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := capture(t, captureRepo(), tc.doc)
			if res.Applied || len(res.Rejected) == 0 || !strings.Contains(res.Rejected[0].Reason, tc.reason) {
				t.Fatalf("expected a rejection mentioning %q, got %+v", tc.reason, res.Rejected)
			}
		})
	}
}

func TestCaptureRequirementIDsAreUniqueAcrossFeatures(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	res := capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{
		ID: "base", Requirements: []application.CaptureRequirement{{ID: "seq", Title: str("dup")}},
	}}})
	if res.Applied || len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Reason, `"inv"`) {
		t.Fatalf("expected the collision rejected, got %+v", res.Rejected)
	}
}

// Per-change approval: work added within approved intent keeps the approval;
// a change to the intent itself needs re-approval.
func TestCaptureApprovalFollowsScope(t *testing.T) {
	approved := func() *MockRepo {
		repo := captureRepo()
		capture(t, repo, wholePlan())
		repo.Plan.ApprovalStatus = planning.ApprovalApproved
		return repo
	}

	repo := approved()
	deps := []string{"task-pdf"}
	res := capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{
		{ID: "task-pdf-a4", Title: str("A4 layout"), Requirement: str("pdf"), DependsOn: &deps},
		{ID: "task-pdf", Title: str("PDF output, split")},
	}})
	if res.PlanApproval != string(planning.ApprovalApproved) {
		t.Fatalf("adding and editing tasks must keep the approval, got %q (%s)", res.PlanApproval, res.ApprovalReason)
	}
	if n := len(repo.Plan.Tasks); n != 4 {
		t.Fatalf("expected the new task alongside the others, got %d tasks", n)
	}

	repo = approved()
	res = capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{
		ID: "inv", Requirements: []application.CaptureRequirement{{ID: "seq", Check: &spec.Check{Run: "true"}}},
	}}})
	if res.PlanApproval != string(planning.ApprovalPending) || !strings.Contains(res.ApprovalReason, "requirement:seq") {
		t.Fatalf("changing a requirement must need re-approval naming it, got %q (%s)", res.PlanApproval, res.ApprovalReason)
	}

	repo = approved()
	repo.Policy = &domain.PolicyConfig{PlanApproval: "every_change"}
	res = capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-pdf", Title: str("x")}}})
	if res.PlanApproval != string(planning.ApprovalPending) {
		t.Fatalf("every_change must reset on any change, got %q", res.PlanApproval)
	}
}

// The plan update path follows the same rule: the reconciler returned every
// rewritten plan as pending, so attaching a check to a task reset approval.
func TestPlanUpdateKeepsApprovalWhenIntentUnchanged(t *testing.T) {
	sp := &spec.ProductSpec{ID: "s", Title: "S", Features: []spec.Feature{{ID: "f", Title: "F",
		Requirements: []spec.Requirement{{ID: "r", Title: "R"}}}}}
	repo := &MockRepo{Spec: sp}
	_ = repo.SaveSpecLock(sp)
	repo.Plan = &planning.Plan{ID: "p", SpecID: "s", ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{{ID: "task-r", Title: "R", FeatureID: "f"}}}
	svc := application.NewPlanService(repo, application.NewAuditService(repo))

	plan, _, err := svc.UpdatePlan([]planning.Task{{ID: "task-r", Title: "R", FeatureID: "f", Check: &planning.Check{Run: "go test ./..."}}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ApprovalStatus != planning.ApprovalApproved {
		t.Fatalf("a task-only update must keep the approval, got %s", plan.ApprovalStatus)
	}

	// A spec that no longer matches its lock is a change of intent.
	repo.Spec.Features[0].Requirements = append(repo.Spec.Features[0].Requirements, spec.Requirement{ID: "r2", Title: "R2"})
	repo.Plan.ApprovalStatus = planning.ApprovalApproved
	plan, _, err = svc.UpdatePlan([]planning.Task{{ID: "task-r", Title: "R", FeatureID: "f"}, {ID: "task-r2", Title: "R2", FeatureID: "f"}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ApprovalStatus != planning.ApprovalPending {
		t.Fatalf("changed intent must need re-approval, got %s", plan.ApprovalStatus)
	}
}

func TestCaptureDryRunWritesNothing(t *testing.T) {
	repo := captureRepo()
	svc := application.NewCaptureService(repo, application.NewAuditService(repo))
	res, err := svc.Capture(wholePlan(), application.CaptureOptions{Actor: "agent", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied || !res.Changed() || !res.DryRun {
		t.Fatalf("expected a reported but unapplied change, got %+v", res)
	}
	if repo.Plan != nil || len(repo.Spec.Features) != 1 {
		t.Fatal("a dry run wrote something")
	}
}
