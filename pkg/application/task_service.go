package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	checkRunner CheckRunner
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

	if event == "start" {
		// Claims that ran out no longer count against WIP or block a start.
		_, _ = s.ReleaseExpiredClaims(actor)
	}
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
		replaced, err := s.claimTask(taskID, actor)
		if err != nil {
			return s.mapCoordinatorError(err, event)
		}
		if replaced != nil {
			s.logClaimExpired(taskID, *replaced, actor)
		}
		meta := map[string]interface{}{
			"task_id": taskID,
			"event":   event,
			"status":  string(planning.StatusInProgress),
		}
		if st, err := s.repo.LoadState(); err == nil && st != nil {
			if l := st.TaskStates[taskID].Lease; l != nil {
				meta["lease_expires_at"] = l.ExpiresAt.Format(time.RFC3339)
			}
		}
		return s.audit.Log("task.transition", actor, meta)

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
		return s.BlockWithReason(taskID, "", evidence, actor)

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
		if err := s.ensureEvidence(taskID); err != nil {
			return err
		}
		check, err := s.ensureCheckPassed(ctx, taskID, actor)
		if err != nil {
			return err
		}
		if err := s.coordinator.VerifyTask(ctx, taskID, actor); err != nil {
			return s.mapCoordinatorError(err, event)
		}
		meta := map[string]interface{}{
			"task_id":  taskID,
			"event":    event,
			"status":   string(planning.StatusVerified),
			"verifier": actor,
		}
		if check != nil {
			meta["check_kind"] = check.Kind
			meta["check_commit"] = check.Commit
			meta["check_command"] = check.Command
		}
		return s.audit.Log("task.transition", actor, meta)

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

	var claimed *project.ClaimedError
	if errors.As(err, &claimed) {
		return fmt.Errorf("cannot %s task %s: it is claimed by %s until %s. Pick another task, or wait for the claim to lapse",
			event, claimed.TaskID, claimed.Holder, claimed.ExpiresAt.Local().Format("2006-01-02 15:04"))
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

// StartTask starts a task using the coordinator with proper dependency validation.
func (s *TaskService) StartTask(ctx context.Context, taskID, owner string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// Claims that ran out no longer count against WIP or block a start.
	_, _ = s.ReleaseExpiredClaims(owner)
	if s.policy != nil {
		if err := s.policy.ValidateTransitionForOwner(taskID, "start", owner); err != nil {
			return err
		}
	}
	replaced, err := s.claimTask(taskID, owner)
	if err != nil {
		return s.mapCoordinatorError(err, "start")
	}
	if replaced != nil {
		s.logClaimExpired(taskID, *replaced, owner)
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
	if err := s.ensureEvidence(taskID); err != nil {
		return err
	}
	if _, err := s.ensureCheckPassed(ctx, taskID, verifier); err != nil {
		return err
	}
	err := s.coordinator.VerifyTask(ctx, taskID, verifier)
	if err != nil {
		return s.mapCoordinatorError(err, "verify")
	}
	return nil
}

// GetCoordinator returns the underlying project coordinator for advanced operations.
func (s *TaskService) GetCoordinator() *project.Coordinator {
	return s.coordinator
}

// BlockWithReason blocks a task and records why. kind spec-conflict or
// cannot-complete is the honest exit from work that cannot be done as
// specified: it needs a detail saying what is wrong, and it is raised as
// drift until a person resolves it.
func (s *TaskService) BlockWithReason(taskID, kind, detail, actor string) error {
	k, err := planning.ParseBlockKind(kind)
	if err != nil {
		return err
	}
	detail = strings.TrimSpace(detail)
	if k.NeedsDecision() && detail == "" {
		return fmt.Errorf("say what is wrong: a %s block needs a detail a person can act on", k)
	}
	why := planning.Block{Kind: k, Detail: detail, By: actor, At: time.Now()}
	if err := s.coordinator.BlockTaskWith(context.Background(), taskID, why); err != nil {
		return s.mapCoordinatorError(err, "block")
	}
	meta := map[string]interface{}{
		"task_id": taskID,
		"event":   "block",
		"status":  string(planning.StatusBlocked),
		"reason":  detail,
	}
	if k != "" {
		meta["kind"] = string(k)
	}
	return s.audit.Log("task.transition", actor, meta)
}
