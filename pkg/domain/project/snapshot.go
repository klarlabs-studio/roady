package project

import (
	"context"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// ProjectSnapshot provides a consistent read model of plan and execution state.
type ProjectSnapshot struct {
	Plan          *planning.Plan
	State         *planning.ExecutionState
	Progress      float64
	UnlockedTasks []string
	BlockedTasks  []string
	InProgress    []string
	Completed     []string
	Verified      []string
	SnapshotTime  time.Time
}

// TaskSummary provides a summary view of a task with its status.
type TaskSummary struct {
	ID          string
	Title       string
	Description string
	Status      planning.TaskStatus
	Priority    planning.TaskPriority
	Owner       string
	DependsOn   []string
	IsBlocked   bool
	IsUnlocked  bool
}

// GetProjectSnapshot returns a consistent snapshot of the current project state.
func (c *Coordinator) GetProjectSnapshot(ctx context.Context) (*ProjectSnapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	plan, err := c.planRepo.Load(ctx)
	if err != nil {
		return nil, err
	}

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return nil, err
	}

	snapshot := &ProjectSnapshot{
		Plan:         plan,
		State:        state,
		SnapshotTime: time.Now(),
	}

	if plan == nil || state == nil {
		return snapshot, nil
	}

	// Calculate progress and categorize tasks
	totalTasks := len(plan.Tasks)
	completedCount := 0

	for _, task := range plan.Tasks {
		status := state.GetTaskStatus(task.ID)

		switch {
		case status.IsBlocked():
			snapshot.BlockedTasks = append(snapshot.BlockedTasks, task.ID)
		case status.IsInProgress():
			snapshot.InProgress = append(snapshot.InProgress, task.ID)
		case status == planning.StatusDone:
			snapshot.Completed = append(snapshot.Completed, task.ID)
			completedCount++
		case status == planning.StatusVerified:
			snapshot.Verified = append(snapshot.Verified, task.ID)
			completedCount++
		case status.IsPending():
			// Check if this task is unlocked (all deps complete)
			if isUnlocked(task, state, c.externalResolver) {
				snapshot.UnlockedTasks = append(snapshot.UnlockedTasks, task.ID)
			}
		}
	}

	// Calculate progress percentage
	if totalTasks > 0 {
		snapshot.Progress = float64(completedCount) / float64(totalTasks) * 100
	}

	return snapshot, nil
}

// GetTaskSummaries returns a summary of all tasks with their current status.
func (c *Coordinator) GetTaskSummaries(ctx context.Context) ([]TaskSummary, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	plan, err := c.planRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, ErrNoPlan
	}

	state, err := c.stateRepo.Load(ctx)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, ErrNoState
	}

	summaries := make([]TaskSummary, 0, len(plan.Tasks))

	for _, task := range plan.Tasks {
		status := state.GetTaskStatus(task.ID)
		result, _ := state.GetTaskResult(task.ID)

		summary := TaskSummary{
			ID:          task.ID,
			Title:       task.Title,
			Description: task.Description,
			Status:      status,
			Priority:    task.Priority,
			Owner:       result.Owner,
			DependsOn:   task.DependsOn,
			IsBlocked:   status.IsBlocked(),
			IsUnlocked:  status.IsPending() && isUnlocked(task, state, c.externalResolver),
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// GetReadyTasks returns tasks that are ready to be started (unlocked and pending).
func (c *Coordinator) GetReadyTasks(ctx context.Context) ([]TaskSummary, error) {
	summaries, err := c.GetTaskSummaries(ctx)
	if err != nil {
		return nil, err
	}

	var ready []TaskSummary
	for _, s := range summaries {
		if s.IsUnlocked && s.Status.IsPending() {
			ready = append(ready, s)
		}
	}

	return ready, nil
}

// GetTasksByOwner returns tasks assigned to owner, matched case-insensitively
// and ignoring surrounding whitespace. An empty owner returns unassigned tasks.
func (c *Coordinator) GetTasksByOwner(ctx context.Context, owner string) ([]TaskSummary, error) {
	summaries, err := c.GetTaskSummaries(ctx)
	if err != nil {
		return nil, err
	}

	want := normalizeOwner(owner)

	var owned []TaskSummary
	for _, s := range summaries {
		if normalizeOwner(s.Owner) == want {
			owned = append(owned, s)
		}
	}

	return owned, nil
}

func normalizeOwner(owner string) string {
	return strings.ToLower(strings.TrimSpace(owner))
}

// GetBlockedTasks returns tasks that are currently blocked.
func (c *Coordinator) GetBlockedTasks(ctx context.Context) ([]TaskSummary, error) {
	summaries, err := c.GetTaskSummaries(ctx)
	if err != nil {
		return nil, err
	}

	var blocked []TaskSummary
	for _, s := range summaries {
		if s.IsBlocked {
			blocked = append(blocked, s)
		}
	}

	return blocked, nil
}

// GetInProgressTasks returns tasks that are currently in progress.
func (c *Coordinator) GetInProgressTasks(ctx context.Context) ([]TaskSummary, error) {
	summaries, err := c.GetTaskSummaries(ctx)
	if err != nil {
		return nil, err
	}

	var inProgress []TaskSummary
	for _, s := range summaries {
		if s.Status.IsInProgress() {
			inProgress = append(inProgress, s)
		}
	}

	return inProgress, nil
}

// isUnlocked checks if a task has all its dependencies completed.
// ExternalStatusResolver reports the status of a task in another
// sub-project. Supplied by the application layer, which knows how to reach
// .roady/projects/<name>/; the domain does not.
type ExternalStatusResolver interface {
	ExternalTaskStatus(project, taskID string) (planning.TaskStatus, bool)
}

// isUnlocked reports whether every dependency is satisfied.
//
// Local and cross-project edges are separated deliberately. Passing an
// external reference like "@auth:task-signup" to the local state lookup
// returns "pending" — not because the work is pending, but because no such
// local task exists — which silently pinned the task as blocked forever with
// no way to resolve it and no explanation.
func isUnlocked(task planning.Task, state *planning.ExecutionState, resolver ExternalStatusResolver) bool {
	for _, depID := range task.LocalDependencies() {
		if !state.GetTaskStatus(depID).IsComplete() {
			return false
		}
	}

	for _, ref := range task.ExternalDependencies() {
		// Without a resolver the honest answer is "cannot confirm", and an
		// unconfirmed dependency blocks. Reporting ready would hand an agent
		// work whose prerequisite may not exist.
		if resolver == nil {
			return false
		}
		status, found := resolver.ExternalTaskStatus(ref.Project, ref.TaskID)
		if !found || !status.IsComplete() {
			return false
		}
	}

	return true
}
