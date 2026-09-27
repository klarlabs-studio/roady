package planning

import "testing"

func TestNewExecutionStateInitializesMap(t *testing.T) {
	state := NewExecutionState("project-x")
	if state.ProjectID != "project-x" {
		t.Fatalf("expected project-x, got %s", state.ProjectID)
	}
	if state.TaskStates == nil {
		t.Fatalf("expected task map initialized")
	}
	if len(state.TaskStates) != 0 {
		t.Fatalf("expected empty task map, got %d", len(state.TaskStates))
	}
}

func TestExecutionState_GetTaskStatus(t *testing.T) {
	state := NewExecutionState("test")

	// Default status for unknown task is Pending
	if status := state.GetTaskStatus("unknown"); status != StatusPending {
		t.Errorf("expected pending for unknown task, got %s", status)
	}

	// Set status and verify
	state.TaskStates["t1"] = TaskResult{Status: StatusInProgress}
	if status := state.GetTaskStatus("t1"); status != StatusInProgress {
		t.Errorf("expected in_progress, got %s", status)
	}
}

func TestExecutionState_SetTaskStatus(t *testing.T) {
	state := NewExecutionState("test")

	state.SetTaskStatus("t1", StatusInProgress)
	if state.TaskStates["t1"].Status != StatusInProgress {
		t.Errorf("expected in_progress status")
	}

	state.SetTaskStatus("t1", StatusDone)
	if state.TaskStates["t1"].Status != StatusDone {
		t.Errorf("expected done status")
	}
}

func TestExecutionState_SetTaskOwner(t *testing.T) {
	state := NewExecutionState("test")

	state.SetTaskOwner("t1", "alice")
	if state.TaskStates["t1"].Owner != "alice" {
		t.Errorf("expected owner alice, got %s", state.TaskStates["t1"].Owner)
	}
}

func TestSetTaskOwner_InitializesStatusForNewTask(t *testing.T) {
	state := NewExecutionState("test")

	// Task not in map yet — SetTaskOwner should default Status to pending
	state.SetTaskOwner("new-task", "alice")

	result := state.TaskStates["new-task"]
	if result.Status != StatusPending {
		t.Errorf("expected status %q for new task, got %q", StatusPending, result.Status)
	}
	if result.Owner != "alice" {
		t.Errorf("expected owner alice, got %s", result.Owner)
	}
}

func TestSetTaskOwner_PreservesExistingStatus(t *testing.T) {
	state := NewExecutionState("test")
	state.TaskStates["t1"] = TaskResult{Status: StatusInProgress, Owner: "bob"}

	state.SetTaskOwner("t1", "alice")

	result := state.TaskStates["t1"]
	if result.Status != StatusInProgress {
		t.Errorf("expected status %q to be preserved, got %q", StatusInProgress, result.Status)
	}
	if result.Owner != "alice" {
		t.Errorf("expected owner alice, got %s", result.Owner)
	}
}

func TestExecutionState_AddEvidence(t *testing.T) {
	state := NewExecutionState("test")

	state.AddEvidence("t1", "commit-abc")
	state.AddEvidence("t1", "commit-def")

	if len(state.TaskStates["t1"].Evidence) != 2 {
		t.Errorf("expected 2 evidence items, got %d", len(state.TaskStates["t1"].Evidence))
	}
}

func TestAddEvidence_InitializesStatusForNewTask(t *testing.T) {
	state := NewExecutionState("test")

	state.AddEvidence("new-task", "commit-abc")

	result := state.TaskStates["new-task"]
	if result.Status != StatusPending {
		t.Errorf("expected status %q for new task, got %q", StatusPending, result.Status)
	}
	if len(result.Evidence) != 1 || result.Evidence[0] != "commit-abc" {
		t.Errorf("expected evidence [commit-abc], got %v", result.Evidence)
	}
}

func TestExecutionState_CountByStatus(t *testing.T) {
	state := NewExecutionState("test")
	state.TaskStates["t1"] = TaskResult{Status: StatusPending}
	state.TaskStates["t2"] = TaskResult{Status: StatusInProgress}
	state.TaskStates["t3"] = TaskResult{Status: StatusInProgress}
	state.TaskStates["t4"] = TaskResult{Status: StatusDone}

	if count := state.CountByStatus(StatusInProgress); count != 2 {
		t.Errorf("expected 2 in_progress tasks, got %d", count)
	}
	if count := state.CountByStatus(StatusPending); count != 1 {
		t.Errorf("expected 1 pending task, got %d", count)
	}
	if count := state.CountByStatus(StatusVerified); count != 0 {
		t.Errorf("expected 0 verified tasks, got %d", count)
	}
}

func TestExecutionState_GetTasksByStatus(t *testing.T) {
	state := NewExecutionState("test")
	state.TaskStates["t1"] = TaskResult{Status: StatusInProgress}
	state.TaskStates["t2"] = TaskResult{Status: StatusInProgress}
	state.TaskStates["t3"] = TaskResult{Status: StatusDone}

	tasks := state.GetTasksByStatus(StatusInProgress)
	if len(tasks) != 2 {
		t.Errorf("expected 2 in_progress tasks, got %d", len(tasks))
	}
}

func TestExecutionState_CanStartTask(t *testing.T) {
	state := NewExecutionState("test")

	// No plan - cannot start
	canStart, _ := state.CanStartTask("t1", nil)
	if canStart {
		t.Error("expected false when no plan")
	}

	// Unapproved plan - cannot start
	plan := &Plan{
		ApprovalStatus: ApprovalPending,
		Tasks:          []Task{{ID: "t1", Title: "Task 1"}},
	}
	canStart, _ = state.CanStartTask("t1", plan)
	if canStart {
		t.Error("expected false when plan not approved")
	}

	// Approved plan, no dependencies - can start
	plan.ApprovalStatus = ApprovalApproved
	canStart, _ = state.CanStartTask("t1", plan)
	if !canStart {
		t.Error("expected true when plan approved and no deps")
	}

	// With unfinished dependency - cannot start
	plan.Tasks = []Task{
		{ID: "t1", Title: "Task 1"},
		{ID: "t2", Title: "Task 2", DependsOn: []string{"t1"}},
	}
	state.TaskStates["t1"] = TaskResult{Status: StatusPending}
	canStart, reason := state.CanStartTask("t2", plan)
	if canStart {
		t.Error("expected false when dependency not done")
	}
	if reason != "dependencies not completed: t1" {
		t.Errorf("unexpected reason: %s", reason)
	}

	// Finished dependency - can start
	state.TaskStates["t1"] = TaskResult{Status: StatusDone}
	canStart, _ = state.CanStartTask("t2", plan)
	if !canStart {
		t.Error("expected true when dependency done")
	}
}

func TestExecutionState_HasUnfinishedDependencies(t *testing.T) {
	state := NewExecutionState("test")

	plan := &Plan{
		Tasks: []Task{
			{ID: "t1", Title: "Task 1"},
			{ID: "t2", Title: "Task 2", DependsOn: []string{"t1"}},
		},
	}

	// Dependency pending
	state.TaskStates["t1"] = TaskResult{Status: StatusPending}
	if !state.HasUnfinishedDependencies("t2", plan) {
		t.Error("expected true when dependency pending")
	}

	// Dependency done
	state.TaskStates["t1"] = TaskResult{Status: StatusDone}
	if state.HasUnfinishedDependencies("t2", plan) {
		t.Error("expected false when dependency done")
	}

	// No dependencies
	if state.HasUnfinishedDependencies("t1", plan) {
		t.Error("expected false when no dependencies")
	}
}

func TestConflictError_Error(t *testing.T) {
	err := &ConflictError{Expected: 3, Actual: 5}
	msg := err.Error()
	if msg != "conflict: expected version 3 but found 5; reload and retry" {
		t.Errorf("unexpected error message: %s", msg)
	}
}

func TestExecutionState_GetTaskResult(t *testing.T) {
	state := NewExecutionState("test")

	// Task not in map
	_, ok := state.GetTaskResult("unknown")
	if ok {
		t.Error("expected ok=false for unknown task")
	}

	// Task in map
	state.TaskStates["t1"] = TaskResult{Status: StatusInProgress, Owner: "alice"}
	result, ok := state.GetTaskResult("t1")
	if !ok {
		t.Error("expected ok=true for existing task")
	}
	if result.Status != StatusInProgress {
		t.Errorf("expected in_progress, got %s", result.Status)
	}
	if result.Owner != "alice" {
		t.Errorf("expected owner alice, got %s", result.Owner)
	}
}

func TestExecutionState_StartTask(t *testing.T) {
	state := NewExecutionState("test")

	state.StartTask("t1")

	result := state.TaskStates["t1"]
	if result.Status != StatusPending {
		t.Errorf("expected pending status for new task, got %s", result.Status)
	}
	if result.StartedAt == nil {
		t.Error("expected StartedAt to be set")
	}

	// Start existing task with status
	state.TaskStates["t2"] = TaskResult{Status: StatusInProgress}
	state.StartTask("t2")

	result2 := state.TaskStates["t2"]
	if result2.StartedAt == nil {
		t.Error("expected StartedAt to be set for existing task")
	}
	if result2.Status != StatusInProgress {
		t.Errorf("expected in_progress preserved, got %s", result2.Status)
	}
}

func TestExecutionState_CompleteTask(t *testing.T) {
	state := NewExecutionState("test")

	// Complete a task that was started
	state.StartTask("t1")
	state.CompleteTask("t1")

	result := state.TaskStates["t1"]
	if result.CompletedAt == nil {
		t.Error("expected CompletedAt to be set")
	}
	if result.ElapsedMinutes < 0 {
		t.Errorf("expected non-negative elapsed minutes, got %d", result.ElapsedMinutes)
	}

	// Complete a task that was never started (no StartedAt)
	state.CompleteTask("t2")
	result2 := state.TaskStates["t2"]
	if result2.CompletedAt == nil {
		t.Error("expected CompletedAt to be set even without StartedAt")
	}
	if result2.ElapsedMinutes != 0 {
		t.Errorf("expected 0 elapsed minutes without StartedAt, got %d", result2.ElapsedMinutes)
	}
}

func TestExecutionState_CanStartTask_TaskNotInPlan(t *testing.T) {
	state := NewExecutionState("test")
	plan := &Plan{
		ApprovalStatus: ApprovalApproved,
		Tasks:          []Task{{ID: "t1", Title: "Task 1"}},
	}

	canStart, reason := state.CanStartTask("nonexistent", plan)
	if canStart {
		t.Error("expected false for task not in plan")
	}
	if reason != "task not found in plan" {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestExecutionState_HasUnfinishedDependencies_NilPlan(t *testing.T) {
	state := NewExecutionState("test")
	if state.HasUnfinishedDependencies("t1", nil) {
		t.Error("expected false for nil plan")
	}
}
