package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

type TaskService struct {
	repo        domain.WorkspaceRepository
	audit       domain.AuditLogger
	policy      *PolicyService
	coordinator *project.Coordinator
}

func NewTaskService(repo domain.WorkspaceRepository, audit domain.AuditLogger, policy *PolicyService) *TaskService {
	return &TaskService{
		repo:        repo,
		audit:       audit,
		policy:      policy,
		coordinator: NewProjectCoordinator(repo, audit),
	}
}

func (s *TaskService) TransitionTask(taskID string, event string, actor string, evidence string) error {
	ctx := context.Background()

	// Validate policy first if policy service is available. The actor is the
	// owner a "start" would assign, so pass it through for per-owner limits.
	if s.policy != nil {
		if err := s.policy.ValidateTransitionForOwner(taskID, event, actor); err != nil {
			return err
		}
	}

	// Use coordinator for supported operations
	switch event {
	case "start":
		err := s.coordinator.StartTask(ctx, taskID, actor, "")
		if err != nil {
			return s.mapCoordinatorError(err, event)
		}
		return s.audit.Log("task.transition", actor, map[string]interface{}{
			"task_id": taskID,
			"event":   event,
			"status":  string(planning.StatusInProgress),
		})

	case "complete":
		unlocked, err := s.coordinator.CompleteTask(ctx, taskID, evidence)
		if err != nil {
			return s.mapCoordinatorError(err, event)
		}
		return s.audit.Log("task.transition", actor, map[string]interface{}{
			"task_id":  taskID,
			"event":    event,
			"status":   string(planning.StatusDone),
			"evidence": evidence,
			"unlocked": unlocked,
		})

	case "block":
		err := s.coordinator.BlockTask(ctx, taskID, evidence)
		if err != nil {
			return s.mapCoordinatorError(err, event)
		}
		return s.audit.Log("task.transition", actor, map[string]interface{}{
			"task_id": taskID,
			"event":   event,
			"status":  string(planning.StatusBlocked),
			"reason":  evidence,
		})

	case "unblock":
		err := s.coordinator.UnblockTask(ctx, taskID)
		if err != nil {
			return s.mapCoordinatorError(err, event)
		}
		return s.audit.Log("task.transition", actor, map[string]interface{}{
			"task_id": taskID,
			"event":   event,
			"status":  string(planning.StatusPending),
		})

	case "verify":
		err := s.coordinator.VerifyTask(ctx, taskID, actor)
		if err != nil {
			return s.mapCoordinatorError(err, event)
		}
		return s.audit.Log("task.transition", actor, map[string]interface{}{
			"task_id":  taskID,
			"event":    event,
			"status":   string(planning.StatusVerified),
			"verifier": actor,
		})

	default:
		// Fallback to FSM for unsupported events
		return s.transitionWithFSM(taskID, event, actor, evidence)
	}
}

// mapCoordinatorError converts coordinator errors to user-friendly messages.
func (s *TaskService) mapCoordinatorError(err error, event string) error {
	if errors.Is(err, project.ErrNoPlan) {
		return fmt.Errorf("no plan found")
	}
	if errors.Is(err, project.ErrPlanNotApproved) {
		return fmt.Errorf("cannot %s task: the plan is not approved. Please approve the plan using 'roady plan approve' before starting work", event)
	}
	if errors.Is(err, project.ErrTaskNotFound) {
		return fmt.Errorf("task not found in plan")
	}
	if errors.Is(err, project.ErrNoState) {
		return fmt.Errorf("no execution state found")
	}
	if errors.Is(err, project.ErrOwnerRequired) {
		return fmt.Errorf("owner/actor required for this operation")
	}

	var depErr *project.DependencyError
	if errors.As(err, &depErr) {
		return fmt.Errorf("cannot start task %s: dependency %s is not complete (status: %s)", depErr.TaskID, depErr.DependencyID, depErr.Status)
	}

	var transErr *project.TransitionError
	if errors.As(err, &transErr) {
		return fmt.Errorf("cannot %s task %s: invalid transition from %s", transErr.Event, transErr.TaskID, transErr.FromStatus)
	}

	return err
}

// transitionWithFSM handles transitions not supported by coordinator (for backward compatibility).
func (s *TaskService) transitionWithFSM(taskID string, event string, actor string, evidence string) error {
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return err
	}
	if plan == nil {
		return fmt.Errorf("no plan found")
	}

	found := false
	for _, t := range plan.Tasks {
		if t.ID == taskID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("task not found in plan: %s", taskID)
	}

	state, err := s.repo.LoadState()
	if err != nil {
		return err
	}

	currentStatus := planning.StatusPending
	if result, ok := state.TaskStates[taskID]; ok {
		currentStatus = result.Status
	}

	guard := func(tid string, ev string) bool {
		if ev == "start" {
			p, err := s.repo.LoadPlan()
			if err != nil || p == nil || !p.ApprovalStatus.IsApproved() {
				return false
			}
		}
		if s.policy != nil {
			return s.policy.ValidateTransition(tid, ev) == nil
		}
		return true
	}

	fsm, err := planning.NewTaskStateMachine(string(currentStatus), taskID, guard)
	if err != nil {
		return err
	}

	if err := fsm.Transition(event); err != nil {
		return err
	}

	newState := fsm.Current()
	result := state.TaskStates[taskID]
	result.Status = planning.TaskStatus(newState)

	if event == "start" {
		result.Owner = actor
	}
	if evidence != "" {
		result.Evidence = append(result.Evidence, evidence)
	}

	state.TaskStates[taskID] = result
	state.UpdatedAt = time.Now()

	if err := s.repo.SaveState(state); err != nil {
		return err
	}

	return s.audit.Log("task.transition", actor, map[string]interface{}{
		"task_id":  taskID,
		"event":    event,
		"status":   newState,
		"evidence": evidence,
	})
}

func (s *TaskService) LinkTask(taskID string, provider string, ref planning.ExternalRef) error {
	state, err := s.repo.LoadState()
	if err != nil {
		return err
	}

	result := state.TaskStates[taskID]
	if result.ExternalRefs == nil {
		result.ExternalRefs = make(map[string]planning.ExternalRef)
	}

	result.ExternalRefs[provider] = ref
	state.TaskStates[taskID] = result
	state.UpdatedAt = time.Now()

	if err := s.repo.SaveState(state); err != nil {
		return err
	}

	return s.audit.Log("task.link", "plugin", map[string]interface{}{
		"task_id":  taskID,
		"provider": provider,
		"ref":      ref.Identifier,
	})
}

// StartTask starts a task using the coordinator with proper dependency validation.
func (s *TaskService) StartTask(ctx context.Context, taskID, owner, rateID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.policy != nil {
		if err := s.policy.ValidateTransitionForOwner(taskID, "start", owner); err != nil {
			return err
		}
	}
	err := s.coordinator.StartTask(ctx, taskID, owner, rateID)
	if err != nil {
		return s.mapCoordinatorError(err, "start")
	}
	return nil
}

// CompleteTask completes a task and returns newly unlocked task IDs.
func (s *TaskService) CompleteTask(ctx context.Context, taskID, evidence string) ([]string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.policy != nil {
		if err := s.policy.ValidateTransition(taskID, "complete"); err != nil {
			return nil, err
		}
	}
	unlocked, err := s.coordinator.CompleteTask(ctx, taskID, evidence)
	if err != nil {
		return nil, s.mapCoordinatorError(err, "complete")
	}
	return unlocked, nil
}

// BlockTask blocks a task with a reason.
func (s *TaskService) BlockTask(ctx context.Context, taskID, reason string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	err := s.coordinator.BlockTask(ctx, taskID, reason)
	if err != nil {
		return s.mapCoordinatorError(err, "block")
	}
	return nil
}

// UnblockTask unblocks a previously blocked task.
func (s *TaskService) UnblockTask(ctx context.Context, taskID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	err := s.coordinator.UnblockTask(ctx, taskID)
	if err != nil {
		return s.mapCoordinatorError(err, "unblock")
	}
	return nil
}

// ReopenTask transitions a Done or Verified task back to Pending so it can
// be re-planned and started again.
func (s *TaskService) ReopenTask(ctx context.Context, taskID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.coordinator.ReopenTask(ctx, taskID); err != nil {
		return s.mapCoordinatorError(err, "reopen")
	}
	return nil
}

// VerifyTask marks a completed task as verified.
func (s *TaskService) VerifyTask(ctx context.Context, taskID, verifier string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	err := s.coordinator.VerifyTask(ctx, taskID, verifier)
	if err != nil {
		return s.mapCoordinatorError(err, "verify")
	}
	return nil
}

// AssignTask sets the owner on a task without requiring a status transition.
func (s *TaskService) AssignTask(_ context.Context, taskID, assignee string) error {
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return err
	}
	if plan == nil {
		return fmt.Errorf("no plan found")
	}

	found := false
	for _, t := range plan.Tasks {
		if t.ID == taskID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("task not found in plan: %s", taskID)
	}

	state, err := s.repo.LoadState()
	if err != nil {
		return err
	}

	state.SetTaskOwner(taskID, assignee)

	if err := s.repo.SaveState(state); err != nil {
		return err
	}

	return s.audit.Log("task.assign", assignee, map[string]interface{}{
		"task_id":  taskID,
		"assignee": assignee,
	})
}

// GetCoordinator returns the underlying project coordinator for advanced operations.
func (s *TaskService) GetCoordinator() *project.Coordinator {
	return s.coordinator
}
