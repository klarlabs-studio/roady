package application

import (
	"context"
	"fmt"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

type PlanService struct {
	repo        domain.WorkspaceRepository
	audit       domain.AuditLogger
	reconciler  *planning.PlanReconciler
	coordinator *project.Coordinator
}

func NewPlanService(repo domain.WorkspaceRepository, audit domain.AuditLogger) *PlanService {
	return &PlanService{
		repo:        repo,
		audit:       audit,
		reconciler:  planning.NewPlanReconciler(),
		coordinator: NewProjectCoordinator(repo, audit),
	}
}

// GeneratePlan updates the Plan based on the current Spec using a default heuristic.
func (s *PlanService) GeneratePlan(ctx context.Context) (*planning.Plan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if err := s.audit.Log("plan.generate", "cli", nil); err != nil {
		return nil, fmt.Errorf("write audit log: %w", err)
	}
	spec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}

	// Default Heuristic: 1 Requirement = 1 Task
	heuristicTasks := make([]planning.Task, 0)
	for _, feat := range spec.Features {
		featSource := planning.TaskSource{Doc: feat.Source.Doc, Line: feat.Source.Line}

		if len(feat.Requirements) == 0 {
			// Fallback if no requirements
			heuristicTasks = append(heuristicTasks, planning.Task{
				ID:          fmt.Sprintf("task-%s", feat.ID),
				Title:       fmt.Sprintf("Implement %s", feat.Title),
				Description: fmt.Sprintf("Implement the feature: %s. %s", feat.Title, feat.Description),
				FeatureID:   feat.ID,
				DependsOn:   []string{},
				Origin:      planning.OriginHeuristic,
				Source:      featSource,
			})
			continue
		}

		for _, req := range feat.Requirements {
			deps := req.DependsOn
			if deps == nil {
				deps = []string{}
			}
			// Map requirement IDs to task IDs (prefix with task-)
			taskDeps := make([]string, len(deps))
			for i, d := range deps {
				taskDeps[i] = fmt.Sprintf("task-%s", d)
			}

			// Prefer the requirement's own source when present; fall
			// back to the feature heading the requirement lives under.
			source := planning.TaskSource{Doc: req.Source.Doc, Line: req.Source.Line}
			if (planning.TaskSource{}) == source {
				source = featSource
			}

			heuristicTasks = append(heuristicTasks, planning.Task{
				ID:          fmt.Sprintf("task-%s", req.ID),
				Title:       fmt.Sprintf("%s (%s)", req.Title, feat.Title),
				Description: req.Description,
				Priority:    planning.TaskPriority(req.Priority),
				Estimate:    req.Estimate,
				FeatureID:   feat.ID,
				DependsOn:   taskDeps,
				Origin:      planning.OriginHeuristic,
				Source:      source,
			})
		}
	}

	// The links here are built from feat.ID above, so there is nothing for
	// the resolver to repair or report.
	plan, _, err := s.ReconcilePlan(heuristicTasks)
	return plan, err
}

// UpdatePlan replaces the plan's tasks with a caller-supplied set, which is
// how an external agent writes a plan back.
//
// The warnings it returns name links Roady could not make sense of. They are
// not errors — the plan is written either way — but a caller that ignores
// them has written a plan that drift will report as orphaned.
func (s *PlanService) UpdatePlan(tasks []planning.Task) (*planning.Plan, []string, error) {
	plan, warnings, err := s.ReconcilePlan(tasks)
	if err != nil {
		return nil, nil, err
	}

	if err := s.audit.Log("plan.update_smart", "ai", map[string]any{
		"plan_id":    plan.ID,
		"spec_id":    plan.SpecID,
		"task_count": len(tasks),
	}); err != nil {
		return nil, nil, fmt.Errorf("write audit log: %w", err)
	}

	return plan, warnings, nil
}

// ReconcilePlan merges new tasks with the existing plan state, returning any
// feature links it could not resolve.
func (s *PlanService) ReconcilePlan(proposedTasks []planning.Task) (*planning.Plan, []string, error) {
	spec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, nil, fmt.Errorf("load spec: %w", err)
	}

	// Repair the task-to-feature links before anything is stored. Writing a
	// task whose feature_id holds a title produces a plan that drift reports
	// as orphaned the moment it is created, and the write is the last point
	// at which Roady still knows enough to fix it.
	warnings := resolveFeatureLinks(spec.Features, proposedTasks)

	existingPlan, _ := s.repo.LoadPlan()

	newPlan, err := s.reconciler.Reconcile(existingPlan, proposedTasks, planning.ReconcileOptions{
		SpecID: spec.ID,
	})
	if err != nil {
		return nil, nil, err
	}

	if err := s.repo.SavePlan(newPlan); err != nil {
		return nil, nil, fmt.Errorf("failed to save plan: %w", err)
	}

	if err := s.repo.SaveSpecLock(spec); err != nil {
		return nil, nil, fmt.Errorf("save spec lock: %w", err)
	}

	return newPlan, warnings, nil
}

func (s *PlanService) GetPlan() (*planning.Plan, error) {

	return s.repo.LoadPlan()

}

func (s *PlanService) GetState() (*planning.ExecutionState, error) {

	return s.repo.LoadState()

}

func (s *PlanService) GetUsage() (*domain.UsageStats, error) {
	return s.repo.LoadUsage()
}

func (s *PlanService) ApprovePlan() error {
	return s.ApprovePlanWithActor("cli")
}

// ApprovePlanWithActor atomically approves the plan and initializes task states.
func (s *PlanService) ApprovePlanWithActor(actor string) error {
	ctx := context.Background()
	if err := s.coordinator.ApprovePlan(ctx, actor); err != nil {
		if err == project.ErrNoPlan {
			return fmt.Errorf("no plan found to approve")
		}
		return err
	}
	return nil
}

func (s *PlanService) PrunePlan() error {
	spec, err := s.repo.LoadSpec()
	if err != nil {
		return err
	}
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return err
	}

	validTaskIDs := make(map[string]bool)
	validFeatureIDs := make(map[string]bool)
	for _, f := range spec.Features {
		validFeatureIDs[f.ID] = true
		for _, r := range f.Requirements {
			validTaskIDs[fmt.Sprintf("task-%s", r.ID)] = true
		}
	}

	before := len(plan.Tasks)
	plan.Tasks = s.reconciler.FilterValidTasks(plan.Tasks, validTaskIDs, validFeatureIDs)
	plan.UpdatedAt = time.Now()

	if err := s.audit.Log("plan.prune", "cli", map[string]any{
		"plan_id":        plan.ID,
		"spec_id":        plan.SpecID,
		"tasks_pruned":   before - len(plan.Tasks),
		"tasks_retained": len(plan.Tasks),
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}

	if err := s.repo.SavePlan(plan); err != nil {
		return err
	}

	// Prune used to stop here, leaving state.json describing tasks the plan
	// no longer contains. The two files then disagreed about what the
	// project even consists of, and nothing reconciled them — Roady's own
	// repository carried 113 such entries. Execution state for a task that
	// does not exist is not history worth keeping; the history lives in
	// events.jsonl, which is untouched.
	return s.pruneOrphanedState(plan)
}

// pruneOrphanedState drops execution state for tasks no longer in the plan.
// It writes nothing when there is nothing to drop, so a no-op prune does not
// bump the state version and provoke a spurious optimistic-locking conflict
// for a concurrent writer.
func (s *PlanService) pruneOrphanedState(plan *planning.Plan) error {
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		// No state yet is not an error: a plan can be pruned before any
		// task has been started.
		return nil //nolint:nilerr // absent state has nothing to prune
	}

	live := make(map[string]bool, len(plan.Tasks))
	for _, t := range plan.Tasks {
		live[t.ID] = true
	}

	removed := 0
	for id := range state.TaskStates {
		if !live[id] {
			delete(state.TaskStates, id)
			removed++
		}
	}

	if removed == 0 {
		return nil
	}

	return s.repo.SaveState(state)
}

func (s *PlanService) RejectPlan() error {
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return err
	}
	if plan == nil {
		return fmt.Errorf("no plan found to reject")
	}

	plan.ApprovalStatus = planning.ApprovalRejected
	plan.UpdatedAt = time.Now()
	if err := s.audit.Log("plan.reject", "cli", map[string]any{
		"plan_id": plan.ID,
		"spec_id": plan.SpecID,
	}); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return s.repo.SavePlan(plan)
}

// GetProjectSnapshot returns a consistent view of plan and execution state.
func (s *PlanService) GetProjectSnapshot(ctx context.Context) (*project.ProjectSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.coordinator.GetProjectSnapshot(ctx)
}

// GetTaskSummaries returns summaries of all tasks with their current status.
func (s *PlanService) GetTaskSummaries(ctx context.Context) ([]project.TaskSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.coordinator.GetTaskSummaries(ctx)
}

// GetReadyTasks returns tasks that are ready to be started (unlocked and pending).
func (s *PlanService) GetReadyTasks(ctx context.Context) ([]project.TaskSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.coordinator.GetReadyTasks(ctx)
}

// GetBlockedTasks returns tasks that are currently blocked.
func (s *PlanService) GetBlockedTasks(ctx context.Context) ([]project.TaskSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.coordinator.GetBlockedTasks(ctx)
}

// GetInProgressTasks returns tasks that are currently in progress.
func (s *PlanService) GetInProgressTasks(ctx context.Context) ([]project.TaskSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.coordinator.GetInProgressTasks(ctx)
}

// GetTasksByOwner returns tasks assigned to owner. An empty owner returns
// unassigned tasks.
func (s *PlanService) GetTasksByOwner(ctx context.Context, owner string) ([]project.TaskSummary, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return s.coordinator.GetTasksByOwner(ctx, owner)
}

// GetCoordinator returns the underlying project coordinator for advanced operations.
func (s *PlanService) GetCoordinator() *project.Coordinator {
	return s.coordinator
}
