package application

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// pruneRepo captures what prune writes back.
type pruneRepo struct {
	domainWorkspaceRepo
	spec      *spec.ProductSpec
	plan      *planning.Plan
	state     *planning.ExecutionState
	savedPlan *planning.Plan
	saved     *planning.ExecutionState
}

func (r *pruneRepo) LoadSpec() (*spec.ProductSpec, error)         { return r.spec, nil }
func (r *pruneRepo) LoadPlan() (*planning.Plan, error)            { return r.plan, nil }
func (r *pruneRepo) LoadState() (*planning.ExecutionState, error) { return r.state, nil }
func (r *pruneRepo) SavePlan(p *planning.Plan) error              { r.savedPlan = p; return nil }
func (r *pruneRepo) SaveState(s *planning.ExecutionState) error   { r.saved = s; return nil }
func (r *pruneRepo) RecordEvent(domain.Event) error               { return nil }
func (r *pruneRepo) LoadEvents() ([]domain.Event, error)          { return nil, nil }

func newPruneRepo() *pruneRepo {
	state := planning.NewExecutionState("plan-1")
	// One task the spec still wants, two it no longer does.
	state.TaskStates["task-keep"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "alice"}
	state.TaskStates["task-drop-a"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["task-drop-b"] = planning.TaskResult{Status: planning.StatusDone}

	return &pruneRepo{
		spec: &spec.ProductSpec{
			ID: "spec-1",
			Features: []spec.Feature{{
				ID: "f1", Title: "Kept",
				Requirements: []spec.Requirement{{ID: "keep", Title: "Kept requirement"}},
			}},
		},
		plan: &planning.Plan{
			ID: "plan-1", SpecID: "spec-1",
			Tasks: []planning.Task{
				{ID: "task-keep", Title: "Kept", FeatureID: "f1"},
				{ID: "task-drop-a", Title: "Dropped A", FeatureID: "gone"},
				{ID: "task-drop-b", Title: "Dropped B", FeatureID: "gone"},
			},
		},
		state: state,
	}
}

// TestPrunePlanRemovesOrphanedState is the defect this covers: prune filtered
// plan.json and left state.json untouched, so every pruned task kept a state
// entry describing a task that no longer existed. Roady's own repository
// accumulated 113 of them.
func TestPrunePlanRemovesOrphanedState(t *testing.T) {
	repo := newPruneRepo()
	svc := NewPlanService(repo, NewAuditService(repo))

	if err := svc.PrunePlan(); err != nil {
		t.Fatalf("PrunePlan: %v", err)
	}

	if repo.savedPlan == nil {
		t.Fatal("plan was not saved")
	}
	if len(repo.savedPlan.Tasks) != 1 || repo.savedPlan.Tasks[0].ID != "task-keep" {
		t.Fatalf("expected only task-keep in the plan, got %+v", repo.savedPlan.Tasks)
	}

	if repo.saved == nil {
		t.Fatal("state was not saved; orphaned entries would survive the prune")
	}
	if len(repo.saved.TaskStates) != 1 {
		t.Errorf("expected 1 state entry, got %d: %v", len(repo.saved.TaskStates), repo.saved.TaskStates)
	}
	if _, ok := repo.saved.TaskStates["task-keep"]; !ok {
		t.Error("the surviving task lost its state")
	}
	for _, gone := range []string{"task-drop-a", "task-drop-b"} {
		if _, ok := repo.saved.TaskStates[gone]; ok {
			t.Errorf("%s was pruned from the plan but kept its state entry", gone)
		}
	}
}

// TestPrunePlanPreservesSurvivingState guards the obvious way to get this
// wrong: clearing state wholesale instead of only the orphans.
func TestPrunePlanPreservesSurvivingState(t *testing.T) {
	repo := newPruneRepo()
	svc := NewPlanService(repo, NewAuditService(repo))

	if err := svc.PrunePlan(); err != nil {
		t.Fatalf("PrunePlan: %v", err)
	}

	kept := repo.saved.TaskStates["task-keep"]
	if kept.Status != planning.StatusInProgress {
		t.Errorf("status = %q, want in_progress", kept.Status)
	}
	if kept.Owner != "alice" {
		t.Errorf("owner = %q, want alice", kept.Owner)
	}
}

// TestPrunePlanWithNothingToPruneLeavesStateAlone avoids writing state — and
// bumping its version — when the prune changed nothing.
func TestPrunePlanWithNothingToPruneLeavesStateAlone(t *testing.T) {
	repo := newPruneRepo()
	repo.plan.Tasks = []planning.Task{{ID: "task-keep", Title: "Kept", FeatureID: "f1"}}
	repo.state.TaskStates = map[string]planning.TaskResult{
		"task-keep": {Status: planning.StatusInProgress},
	}

	svc := NewPlanService(repo, NewAuditService(repo))
	if err := svc.PrunePlan(); err != nil {
		t.Fatalf("PrunePlan: %v", err)
	}

	if repo.saved != nil {
		t.Error("state was rewritten despite nothing being pruned")
	}
}
