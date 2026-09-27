package application_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// guardRepo: requirement r with a check, its task in the given status, the
// spec locked as it stands.
func guardRepo(status planning.TaskStatus) *MockRepo {
	sp := &spec.ProductSpec{ID: "s", Title: "S", Features: []spec.Feature{{ID: "f", Title: "F",
		Requirements: []spec.Requirement{{ID: "r", Title: "R", Check: &spec.Check{Run: "go test ./strict"}}}}}}
	repo := &MockRepo{Spec: sp}
	_ = repo.SaveSpecLock(sp)
	repo.Plan = &planning.Plan{ID: "p", SpecID: "s", ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{{ID: "task-r", Title: "R (F)", FeatureID: "f", Check: &planning.Check{Run: "go test ./strict"}}}}
	repo.State = &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{"task-r": {Status: status}}}
	return repo
}

func loosen() application.CaptureDoc {
	return application.CaptureDoc{Features: []application.CaptureFeature{{ID: "f",
		Requirements: []application.CaptureRequirement{{ID: "r", Check: &spec.Check{Run: "true"}}}}}}
}

func TestCaptureRefusesLooseningStartedWork(t *testing.T) {
	for _, st := range []planning.TaskStatus{planning.StatusInProgress, planning.StatusDone, planning.StatusVerified} {
		repo := guardRepo(st)
		res := capture(t, repo, loosen())
		if res.Applied || len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Reason, "task-r") {
			t.Fatalf("%s: expected the check change refused, got %+v", st, res)
		}
		if repo.Spec.Features[0].Requirements[0].Check.Run != "go test ./strict" {
			t.Fatalf("%s: a refused capture changed the spec", st)
		}
	}
}

func TestCheckChangesFreeBeforeWorkStartsAndWhenTightening(t *testing.T) {
	repo := guardRepo(planning.StatusPending)
	if res := capture(t, repo, loosen()); !res.Applied {
		t.Fatalf("changing the check of work not yet started is planning, got %+v", res.Rejected)
	}

	repo = guardRepo(planning.StatusDone)
	repo.Spec.Features[0].Requirements = append(repo.Spec.Features[0].Requirements, spec.Requirement{ID: "r2", Title: "R2"})
	_ = repo.SaveSpecLock(repo.Spec)
	repo.Plan.Tasks = append(repo.Plan.Tasks, planning.Task{ID: "task-r2", Title: "R2 (F)", FeatureID: "f"})
	repo.State.TaskStates["task-r2"] = planning.TaskResult{Status: planning.StatusDone}
	res := capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{ID: "f",
		Requirements: []application.CaptureRequirement{{ID: "r2", Check: &spec.Check{Run: "go test ./new"}}}}}})
	if !res.Applied {
		t.Fatalf("adding a check tightens and is always allowed, got %+v", res.Rejected)
	}
}

// Allowed on the record: done work reopens and the change is logged.
func TestAllowedCheckChangeReopensDoneWork(t *testing.T) {
	repo := guardRepo(planning.StatusDone)
	svc := application.NewCaptureService(repo, application.NewAuditService(repo))
	res, err := svc.Capture(loosen(), application.CaptureOptions{Actor: "felix", AllowCheckChange: true})
	if err != nil || !res.Applied {
		t.Fatalf("expected the allowed change applied, got %+v %v", res, err)
	}
	if got := repo.State.TaskStates["task-r"].Status; got != planning.StatusPending {
		t.Fatalf("done work whose check changed must reopen, got %s", got)
	}
}

// The plan path: removing a task's check through plan_update.
func TestPlanUpdateRefusesRemovingStartedCheck(t *testing.T) {
	repo := guardRepo(planning.StatusInProgress)
	svc := application.NewPlanService(repo, application.NewAuditService(repo))
	_, _, err := svc.UpdatePlan([]planning.Task{{ID: "task-r", Title: "R (F)", FeatureID: "f"}})
	var guarded *application.CheckChangeError
	if !errors.As(err, &guarded) {
		t.Fatalf("expected a refusal, got %v", err)
	}
	svc.AllowCheckChanges(true)
	if _, _, err := svc.UpdatePlan([]planning.Task{{ID: "task-r", Title: "R (F)", FeatureID: "f"}}); err != nil {
		t.Fatalf("allowed: %v", err)
	}
}

// The lock path: a check loosened by hand in spec.yaml must not become the
// baseline through `spec lock` or `drift accept` — that is how it would stop
// being reported as drift.
func TestLockingCannotBlessALoosenedCheck(t *testing.T) {
	handEdit := func() *MockRepo {
		repo := guardRepo(planning.StatusVerified)
		edited := *repo.Spec
		edited.Features = []spec.Feature{{ID: "f", Title: "F", Requirements: []spec.Requirement{{ID: "r", Title: "R", Check: &spec.Check{Run: "true"}}}}}
		repo.Spec = &edited
		return repo
	}
	var guarded *application.CheckChangeError

	repo := handEdit()
	if _, err := application.NewSpecService(repo).WriteLock(); !errors.As(err, &guarded) {
		t.Fatalf("spec lock: expected a refusal, got %v", err)
	}
	if _, err := application.NewSpecService(repo).AddFeature("Other", ""); !errors.As(err, &guarded) {
		t.Fatalf("spec add re-locks the spec and must refuse too, got %v", err)
	}
	drift := application.NewDriftService(repo, application.NewAuditService(repo), nil, nil)
	if err := drift.AcceptDrift(); !errors.As(err, &guarded) {
		t.Fatalf("drift accept: expected a refusal, got %v", err)
	}
	if repo.SpecLock.Features[0].Requirements[0].Check.Run != "go test ./strict" {
		t.Fatal("the lock was rewritten despite the refusal")
	}

	if err := drift.AcceptDriftWith(true, "felix"); err != nil {
		t.Fatalf("allowed accept: %v", err)
	}
	if got := repo.State.TaskStates["task-r"].Status; got != planning.StatusPending {
		t.Fatalf("verified work whose check was loosened must reopen, got %s", got)
	}
}

// spec lock compared only IDs and counts, so after a description edit it
// answered "already in sync" while drift, which compares hashes, kept
// reporting the change.
func TestSpecLockNoticesContentChanges(t *testing.T) {
	repo := guardRepo(planning.StatusPending)
	edited := *repo.Spec
	edited.Features = []spec.Feature{{ID: "f", Title: "F", Requirements: []spec.Requirement{{ID: "r", Title: "R",
		Description: "now per calendar year", Check: &spec.Check{Run: "go test ./strict"}}}}}
	repo.Spec = &edited
	res, err := application.NewSpecService(repo).WriteLock()
	if err != nil {
		t.Fatal(err)
	}
	if !res.LockUpdated || repo.SpecLock.Hash() != repo.Spec.Hash() {
		t.Fatalf("a content change must re-capture the lock, got %+v", res)
	}
}
