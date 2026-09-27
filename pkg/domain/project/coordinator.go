// Package project provides aggregate coordination for Plan and ExecutionState.
package project

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// PlanRepository defines the interface for plan persistence.
type PlanRepository interface {
	Load(ctx context.Context) (*planning.Plan, error)
	Save(ctx context.Context, plan *planning.Plan) error
}

// StateRepository defines the interface for execution state persistence.
type StateRepository interface {
	Load(ctx context.Context) (*planning.ExecutionState, error)
	Save(ctx context.Context, state *planning.ExecutionState) error
}

// EventPublisher defines the interface for publishing domain events.
type EventPublisher interface {
	PublishPlanApproved(ctx context.Context, planID, approver string) error
	PublishTaskStarted(ctx context.Context, taskID, owner string) error
	PublishTaskCompleted(ctx context.Context, taskID, evidence string) error
	PublishTaskBlocked(ctx context.Context, taskID, reason string) error
	PublishTaskUnblocked(ctx context.Context, taskID string) error
}

// Coordinator provides atomic operations across Plan and ExecutionState aggregates.
// It ensures consistency between the two aggregates and handles cross-cutting concerns.
type Coordinator struct {
	mu        sync.RWMutex
	planRepo  PlanRepository
	stateRepo StateRepository
	publisher EventPublisher

	// externalResolver answers cross-project dependency lookups. Optional:
	// a nil resolver makes external dependencies block, which is the safe
	// reading when their status cannot be confirmed.
	externalResolver ExternalStatusResolver
}

// SetExternalResolver supplies cross-project dependency resolution.
func (c *Coordinator) SetExternalResolver(r ExternalStatusResolver) {
	c.externalResolver = r
}

// NewCoordinator creates a new Coordinator.
func NewCoordinator(planRepo PlanRepository, stateRepo StateRepository, publisher EventPublisher) *Coordinator {
	return &Coordinator{
		planRepo:  planRepo,
		stateRepo: stateRepo,
		publisher: publisher,
	}
}

// ApprovePlan atomically approves the plan and initializes task states.
func (c *Coordinator) ApprovePlan(ctx context.Context, approver string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	plan, err := c.planRepo.Load(ctx)
	if err != nil {
		return err
	}
	if plan == nil {
		return ErrNoPlan
	}

	// Check if already approved
	if plan.ApprovalStatus.IsApproved() {
		return nil // Already approved, no-op
	}

	// Check transition is allowed
	if !plan.ApprovalStatus.CanTransitionTo(planning.ApprovalApproved) {
		return ErrInvalidTransition
	}

	// Update plan status
	plan.ApprovalStatus = planning.ApprovalApproved
	if err := c.planRepo.Save(ctx, plan); err != nil {
		return err
	}

	// Initialize execution state with all tasks in pending status
	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return err
	}
	if state == nil {
		state = planning.NewExecutionState(plan.ID)
	}

	// Initialize all tasks to pending
	for _, task := range plan.Tasks {
		if _, exists := state.TaskStates[task.ID]; !exists {
			state.TaskStates[task.ID] = planning.TaskResult{
				Status: planning.StatusPending,
			}
		}
	}

	if err := c.stateRepo.Save(ctx, state); err != nil {
		return err
	}

	// Publish event (fire-and-forget, errors logged internally)
	if c.publisher != nil {
		_ = c.publisher.PublishPlanApproved(ctx, plan.ID, approver)
	}

	return nil
}

// StartTask validates dependencies and starts a task.
func (c *Coordinator) StartTask(ctx context.Context, taskID, owner string) error {
	_, err := c.ClaimTask(ctx, taskID, owner, ClaimOptions{})
	return err
}

// ClaimOptions shapes the claim starting a task takes.
type ClaimOptions struct {
	Session string
	// TTL is the lease length; zero takes no lease.
	TTL time.Duration
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// ClaimedError is returned when a task is in progress under someone else's
// live claim.
type ClaimedError struct {
	TaskID    string
	Holder    string
	ExpiresAt time.Time
}

func (e *ClaimedError) Error() string {
	return fmt.Sprintf("task %s is claimed by %s until %s", e.TaskID, e.Holder, e.ExpiresAt.Format(time.RFC3339))
}

// ClaimTask starts a task for owner and puts a lease on it. A task still
// in progress under a lease that has run out is taken over; the lease it
// replaced is returned so the takeover can be recorded.
func (c *Coordinator) ClaimTask(ctx context.Context, taskID, owner string, opt ClaimOptions) (*planning.Lease, error) {
	if owner == "" {
		return nil, ErrOwnerRequired
	}
	now := time.Now
	if opt.Now != nil {
		now = opt.Now
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	plan, err := c.planRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrNoPlan
	}
	if !plan.ApprovalStatus.IsApproved() {
		return nil, ErrPlanNotApproved
	}

	// Find task in plan
	task := findTask(plan, taskID)
	if task == nil {
		return nil, ErrTaskNotFound
	}

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, ErrNoState
	}

	// Check current status allows starting. A claim that has run out no
	// longer holds the task: it can be taken over.
	currentStatus := state.GetTaskStatus(taskID)
	var replaced *planning.Lease
	if result, _ := state.GetTaskResult(taskID); currentStatus == planning.StatusInProgress && result.Lease != nil {
		if !result.Lease.Expired(now()) {
			if !result.Lease.HeldBy(owner, opt.Session) {
				return nil, &ClaimedError{TaskID: taskID, Holder: result.Lease.Holder, ExpiresAt: result.Lease.ExpiresAt}
			}
		} else {
			replaced = result.Lease
			currentStatus = planning.StatusPending
		}
	}
	if !currentStatus.CanTransitionWith("start") {
		return nil, &TransitionError{
			TaskID:     taskID,
			FromStatus: string(currentStatus),
			ToStatus:   string(planning.StatusInProgress),
			Event:      "start",
		}
	}

	// Validate dependencies
	for _, depID := range task.DependsOn {
		depStatus := state.GetTaskStatus(depID)
		if !depStatus.IsComplete() {
			return nil, &DependencyError{
				TaskID:       taskID,
				DependencyID: depID,
				Status:       string(depStatus),
			}
		}
	}

	// Update state
	state.SetTaskStatus(taskID, planning.StatusInProgress)
	state.SetTaskOwner(taskID, owner)
	state.StartTask(taskID)
	state.Claim(taskID, owner, opt.Session, opt.TTL, now())

	if err := c.stateRepo.Save(ctx, state); err != nil {
		return nil, err
	}

	// Publish event (fire-and-forget, errors logged internally)
	if c.publisher != nil {
		_ = c.publisher.PublishTaskStarted(ctx, taskID, owner)
	}

	return replaced, nil
}

// CompleteTask completes a task and returns newly unlocked task IDs.
func (c *Coordinator) CompleteTask(ctx context.Context, taskID, evidence string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	plan, err := c.planRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrNoPlan
	}

	task := findTask(plan, taskID)
	if task == nil {
		return nil, ErrTaskNotFound
	}

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, ErrNoState
	}

	// Check current status allows completion
	currentStatus := state.GetTaskStatus(taskID)
	if !currentStatus.CanTransitionWith("complete") {
		return nil, &TransitionError{
			TaskID:     taskID,
			FromStatus: string(currentStatus),
			ToStatus:   string(planning.StatusDone),
			Event:      "complete",
		}
	}

	// Update state
	state.SetTaskStatus(taskID, planning.StatusDone)
	state.CompleteTask(taskID)
	if evidence != "" {
		state.AddEvidence(taskID, evidence)
	}

	if err := c.stateRepo.Save(ctx, state); err != nil {
		return nil, err
	}

	// Publish event (fire-and-forget, errors logged internally)
	if c.publisher != nil {
		_ = c.publisher.PublishTaskCompleted(ctx, taskID, evidence)
	}

	// Find newly unlocked tasks
	unlocked := c.findUnlockedTasks(plan, state)

	return unlocked, nil
}

// BlockTask blocks a task with a reason.
func (c *Coordinator) BlockTask(ctx context.Context, taskID, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return err
	}
	if state == nil {
		return ErrNoState
	}

	currentStatus := state.GetTaskStatus(taskID)
	if !currentStatus.CanTransitionWith("block") {
		return &TransitionError{
			TaskID:     taskID,
			FromStatus: string(currentStatus),
			ToStatus:   string(planning.StatusBlocked),
			Event:      "block",
		}
	}

	state.SetTaskStatus(taskID, planning.StatusBlocked)
	if err := c.stateRepo.Save(ctx, state); err != nil {
		return err
	}

	// Publish event (fire-and-forget, errors logged internally)
	if c.publisher != nil {
		_ = c.publisher.PublishTaskBlocked(ctx, taskID, reason)
	}

	return nil
}

// UnblockTask unblocks a previously blocked task.
func (c *Coordinator) UnblockTask(ctx context.Context, taskID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return err
	}
	if state == nil {
		return ErrNoState
	}

	currentStatus := state.GetTaskStatus(taskID)
	if !currentStatus.CanTransitionWith("unblock") {
		return &TransitionError{
			TaskID:     taskID,
			FromStatus: string(currentStatus),
			ToStatus:   string(planning.StatusPending),
			Event:      "unblock",
		}
	}

	state.SetTaskStatus(taskID, planning.StatusPending)
	if err := c.stateRepo.Save(ctx, state); err != nil {
		return err
	}

	// Publish event (fire-and-forget, errors logged internally)
	if c.publisher != nil {
		_ = c.publisher.PublishTaskUnblocked(ctx, taskID)
	}

	return nil
}

// ReopenTask transitions a Done or Verified task back to Pending so it can
// be picked up and started again. Useful when a completion was premature or
// new evidence shows the task is not actually finished.
func (c *Coordinator) ReopenTask(ctx context.Context, taskID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return err
	}
	if state == nil {
		return ErrNoState
	}

	currentStatus := state.GetTaskStatus(taskID)
	if !currentStatus.CanTransitionWith("reopen") {
		return &TransitionError{
			TaskID:     taskID,
			FromStatus: string(currentStatus),
			ToStatus:   string(planning.StatusPending),
			Event:      "reopen",
		}
	}

	state.SetTaskStatus(taskID, planning.StatusPending)
	if err := c.stateRepo.Save(ctx, state); err != nil {
		return err
	}

	// Fire-and-forget; reuse the unblock event since both land in Pending and
	// signal "this task is back in flight".
	if c.publisher != nil {
		_ = c.publisher.PublishTaskUnblocked(ctx, taskID)
	}

	return nil
}

// VerifyTask marks a completed task as verified.
func (c *Coordinator) VerifyTask(ctx context.Context, taskID, verifier string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return err
	}
	if state == nil {
		return ErrNoState
	}

	currentStatus := state.GetTaskStatus(taskID)
	if !currentStatus.CanTransitionWith("verify") {
		return &TransitionError{
			TaskID:     taskID,
			FromStatus: string(currentStatus),
			ToStatus:   string(planning.StatusVerified),
			Event:      "verify",
		}
	}

	state.SetTaskStatus(taskID, planning.StatusVerified)
	return c.stateRepo.Save(ctx, state)
}

// GetPlan returns the current plan.
func (c *Coordinator) GetPlan(ctx context.Context) (*planning.Plan, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.planRepo.Load(ctx)
}

// GetState returns the current execution state.
func (c *Coordinator) GetState(ctx context.Context) (*planning.ExecutionState, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stateRepo.Load(ctx)
}

// findUnlockedTasks returns task IDs that can now be started.
func (c *Coordinator) findUnlockedTasks(plan *planning.Plan, state *planning.ExecutionState) []string {
	var unlocked []string

	for _, task := range plan.Tasks {
		// Skip tasks that are not pending
		if state.GetTaskStatus(task.ID) != planning.StatusPending {
			continue
		}

		// Check if all dependencies are complete
		allDepsComplete := true
		for _, depID := range task.DependsOn {
			if !state.GetTaskStatus(depID).IsComplete() {
				allDepsComplete = false
				break
			}
		}

		if allDepsComplete {
			unlocked = append(unlocked, task.ID)
		}
	}

	return unlocked
}

// findTask looks up a task in the plan by ID.
func findTask(plan *planning.Plan, taskID string) *planning.Task {
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID {
			return &plan.Tasks[i]
		}
	}
	return nil
}
