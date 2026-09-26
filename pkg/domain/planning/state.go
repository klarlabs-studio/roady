package planning

import (
	"fmt"
	"time"
)

// ExecutionState represents the current reality of the project execution.
type ExecutionState struct {
	ProjectID  string                `json:"project_id"`
	Version    int                   `json:"version"`
	TaskStates map[string]TaskResult `json:"task_states"` // TaskID -> Result
	UpdatedAt  time.Time             `json:"updated_at"`
}

// ConflictError is returned when a save fails due to a version mismatch.
type ConflictError struct {
	Expected int
	Actual   int
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict: expected version %d but found %d; reload and retry", e.Expected, e.Actual)
}

// TaskResult captures the progress of a single task.
type TaskResult struct {
	Status       TaskStatus             `json:"status"`
	Path         string                 `json:"path"`
	Owner        string                 `json:"owner,omitempty"`    // Who is currently working on this?
	Evidence     []string               `json:"evidence,omitempty"` // List of evidence (commit hashes, links, etc.)
	ExternalRefs map[string]ExternalRef `json:"external_refs,omitempty"`

	// Time tracking fields
	StartedAt      *time.Time `json:"started_at,omitempty"`   // When task moved to in_progress
	CompletedAt    *time.Time `json:"completed_at,omitempty"` // When task moved to done/verified
	ElapsedMinutes int        `json:"elapsed_minutes"`        // Total elapsed time in minutes
	RateID         string     `json:"rate_id,omitempty"`      // Rate used for billing

	// Checks records every run of the task's acceptance check, newest last.
	Checks []CheckResult `json:"checks,omitempty"`
}

// ExternalRef links a Roady task to an external system (Linear, Jira, etc.)
type ExternalRef struct {
	ID           string    `json:"id"`         // Internal ID (e.g. "e84910...")
	Identifier   string    `json:"identifier"` // Human readable ID (e.g. "LIN-123")
	URL          string    `json:"url"`
	LastSyncedAt time.Time `json:"last_synced_at"`
}

func NewExecutionState(projectID string) *ExecutionState {
	return &ExecutionState{
		ProjectID:  projectID,
		TaskStates: make(map[string]TaskResult),
		UpdatedAt:  time.Now(),
	}
}

// GetTaskStatus returns the current status of a task, defaulting to Pending if not found.
func (s *ExecutionState) GetTaskStatus(taskID string) TaskStatus {
	if result, ok := s.TaskStates[taskID]; ok {
		return result.Status
	}
	return StatusPending
}

// GetTaskResult returns the full task result for a task ID.
func (s *ExecutionState) GetTaskResult(taskID string) (TaskResult, bool) {
	result, ok := s.TaskStates[taskID]
	return result, ok
}

// SetTaskStatus updates the status for a task and records the update time.
func (s *ExecutionState) SetTaskStatus(taskID string, status TaskStatus) {
	result := s.TaskStates[taskID]
	result.Status = status
	s.TaskStates[taskID] = result
	s.UpdatedAt = time.Now()
}

// SetTaskOwner sets the owner for a task.
func (s *ExecutionState) SetTaskOwner(taskID string, owner string) {
	result := s.TaskStates[taskID]
	if result.Status == "" {
		result.Status = StatusPending
	}
	result.Owner = owner
	s.TaskStates[taskID] = result
	s.UpdatedAt = time.Now()
}

// AddEvidence appends evidence to a task's result.
func (s *ExecutionState) AddEvidence(taskID string, evidence string) {
	result := s.TaskStates[taskID]
	if result.Status == "" {
		result.Status = StatusPending
	}
	result.Evidence = append(result.Evidence, evidence)
	s.TaskStates[taskID] = result
	s.UpdatedAt = time.Now()
}

// SetExternalRef sets an external reference for a task.
func (s *ExecutionState) SetExternalRef(taskID string, provider string, ref ExternalRef) {
	result := s.TaskStates[taskID]
	if result.Status == "" {
		result.Status = StatusPending
	}
	if result.ExternalRefs == nil {
		result.ExternalRefs = make(map[string]ExternalRef)
	}
	result.ExternalRefs[provider] = ref
	s.TaskStates[taskID] = result
	s.UpdatedAt = time.Now()
}

// CountByStatus returns the count of tasks with the given status.
func (s *ExecutionState) CountByStatus(status TaskStatus) int {
	count := 0
	for _, result := range s.TaskStates {
		if result.Status == status {
			count++
		}
	}
	return count
}

// GetTasksByStatus returns all task IDs with the given status.
func (s *ExecutionState) GetTasksByStatus(status TaskStatus) []string {
	var taskIDs []string
	for taskID, result := range s.TaskStates {
		if result.Status == status {
			taskIDs = append(taskIDs, taskID)
		}
	}
	return taskIDs
}

// CanStartTask checks if a task can be started based on plan approval and dependencies.
func (s *ExecutionState) CanStartTask(taskID string, plan *Plan) (bool, string) {
	if plan == nil {
		return false, "no plan found"
	}

	if !plan.ApprovalStatus.IsApproved() {
		return false, "plan is not approved"
	}

	// Find the task in the plan
	var task *Task
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID {
			task = &plan.Tasks[i]
			break
		}
	}

	if task == nil {
		return false, "task not found in plan"
	}

	// Check if dependencies are met (all must be Done or Verified)
	for _, depID := range task.DependsOn {
		depStatus := s.GetTaskStatus(depID)
		if !depStatus.IsComplete() {
			return false, "dependencies not completed: " + depID
		}
	}

	return true, ""
}

// HasUnfinishedDependencies checks if a task has any unfinished dependencies.
func (s *ExecutionState) HasUnfinishedDependencies(taskID string, plan *Plan) bool {
	if plan == nil {
		return false
	}

	for _, task := range plan.Tasks {
		if task.ID == taskID {
			for _, depID := range task.DependsOn {
				depStatus := s.GetTaskStatus(depID)
				if !depStatus.IsComplete() {
					return true
				}
			}
			break
		}
	}
	return false
}

// StartTask records the start time for a task.
func (s *ExecutionState) StartTask(taskID string) {
	result := s.TaskStates[taskID]
	if result.Status == "" {
		result.Status = StatusPending
	}
	now := time.Now()
	result.StartedAt = &now
	s.TaskStates[taskID] = result
	s.UpdatedAt = now
}

// CompleteTask records the completion time and calculates elapsed minutes.
func (s *ExecutionState) CompleteTask(taskID string) {
	result := s.TaskStates[taskID]
	now := time.Now()
	result.CompletedAt = &now

	if result.StartedAt != nil {
		result.ElapsedMinutes = int(now.Sub(*result.StartedAt).Minutes())
	}

	s.TaskStates[taskID] = result
	s.UpdatedAt = now
}
