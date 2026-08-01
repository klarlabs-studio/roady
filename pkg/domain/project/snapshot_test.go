package project

import (
	"context"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestCoordinator_GetProjectSnapshot(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task"},
			{ID: "task-2", Title: "Second Task", DependsOn: []string{"task-1"}},
			{ID: "task-3", Title: "Third Task", DependsOn: []string{"task-1"}},
			{ID: "task-4", Title: "Fourth Task", DependsOn: []string{"task-2", "task-3"}},
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "alice"}
	state.TaskStates["task-3"] = planning.TaskResult{Status: planning.StatusBlocked}
	state.TaskStates["task-4"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	snapshot, err := coord.GetProjectSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetProjectSnapshot failed: %v", err)
	}

	// Check progress (1 done out of 4 = 25%)
	if snapshot.Progress != 25 {
		t.Errorf("Expected 25%% progress, got %.1f%%", snapshot.Progress)
	}

	// Check categorization
	if len(snapshot.Completed) != 1 {
		t.Errorf("Expected 1 completed, got %d", len(snapshot.Completed))
	}
	if len(snapshot.InProgress) != 1 {
		t.Errorf("Expected 1 in progress, got %d", len(snapshot.InProgress))
	}
	if len(snapshot.BlockedTasks) != 1 {
		t.Errorf("Expected 1 blocked, got %d", len(snapshot.BlockedTasks))
	}

	// task-4 is pending but task-2 (in progress) and task-3 (blocked) not complete
	// so task-4 should NOT be unlocked
	if len(snapshot.UnlockedTasks) != 0 {
		t.Errorf("Expected 0 unlocked, got %v", snapshot.UnlockedTasks)
	}
}

func TestCoordinator_GetProjectSnapshot_UnlockedTasks(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task"},
			{ID: "task-2", Title: "Second Task", DependsOn: []string{"task-1"}},
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	snapshot, err := coord.GetProjectSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetProjectSnapshot failed: %v", err)
	}

	// task-2 should be unlocked since task-1 is done
	if len(snapshot.UnlockedTasks) != 1 || snapshot.UnlockedTasks[0] != "task-2" {
		t.Errorf("Expected [task-2] unlocked, got %v", snapshot.UnlockedTasks)
	}
}

func TestCoordinator_GetProjectSnapshot_NilPlan(t *testing.T) {
	planRepo := &mockPlanRepo{plan: nil}
	stateRepo := &mockStateRepo{state: nil}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	snapshot, err := coord.GetProjectSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetProjectSnapshot failed: %v", err)
	}

	if snapshot.Plan != nil {
		t.Error("Expected nil plan in snapshot")
	}
	if snapshot.Progress != 0 {
		t.Error("Expected 0 progress for nil plan")
	}
}

func TestCoordinator_GetTaskSummaries(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task", Priority: planning.PriorityHigh},
			{ID: "task-2", Title: "Second Task", Priority: planning.PriorityMedium, DependsOn: []string{"task-1"}},
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "alice"}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	summaries, err := coord.GetTaskSummaries(context.Background())
	if err != nil {
		t.Fatalf("GetTaskSummaries failed: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("Expected 2 summaries, got %d", len(summaries))
	}

	// Check first task
	if summaries[0].ID != "task-1" {
		t.Errorf("Expected task-1, got %s", summaries[0].ID)
	}
	if summaries[0].Owner != "alice" {
		t.Errorf("Expected owner alice, got %s", summaries[0].Owner)
	}
	if summaries[0].Priority != planning.PriorityHigh {
		t.Errorf("Expected high priority, got %v", summaries[0].Priority)
	}

	// Check second task
	if !summaries[1].Status.IsPending() {
		t.Error("Expected task-2 to be pending")
	}
	// task-2 is NOT unlocked because task-1 is in_progress, not complete
	if summaries[1].IsUnlocked {
		t.Error("Expected task-2 to NOT be unlocked while task-1 is in progress")
	}
}

func TestCoordinator_GetReadyTasks(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task"},
			{ID: "task-2", Title: "Second Task", DependsOn: []string{"task-1"}},
			{ID: "task-3", Title: "Third Task"}, // No dependencies
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["task-3"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	ready, err := coord.GetReadyTasks(context.Background())
	if err != nil {
		t.Fatalf("GetReadyTasks failed: %v", err)
	}

	// Both task-2 and task-3 should be ready
	if len(ready) != 2 {
		t.Errorf("Expected 2 ready tasks, got %d", len(ready))
	}
}

func TestCoordinator_GetBlockedTasks(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task"},
			{ID: "task-2", Title: "Second Task"},
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusBlocked}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusInProgress}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	blocked, err := coord.GetBlockedTasks(context.Background())
	if err != nil {
		t.Fatalf("GetBlockedTasks failed: %v", err)
	}

	if len(blocked) != 1 {
		t.Errorf("Expected 1 blocked task, got %d", len(blocked))
	}
	if blocked[0].ID != "task-1" {
		t.Errorf("Expected task-1, got %s", blocked[0].ID)
	}
}

func TestCoordinator_GetInProgressTasks(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task"},
			{ID: "task-2", Title: "Second Task"},
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "alice"}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	inProgress, err := coord.GetInProgressTasks(context.Background())
	if err != nil {
		t.Fatalf("GetInProgressTasks failed: %v", err)
	}

	if len(inProgress) != 1 {
		t.Errorf("Expected 1 in-progress task, got %d", len(inProgress))
	}
	if inProgress[0].ID != "task-1" {
		t.Errorf("Expected task-1, got %s", inProgress[0].ID)
	}
}

func TestCoordinator_GetTasksByOwner(t *testing.T) {
	plan := &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "Alice work"},
			{ID: "task-2", Title: "Bob work"},
			{ID: "task-3", Title: "Alice other"},
			{ID: "task-4", Title: "Nobody's work"},
		},
	}
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "alice"}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "bob"}
	state.TaskStates["task-3"] = planning.TaskResult{Status: planning.StatusDone, Owner: "Alice"}
	state.TaskStates["task-4"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	tests := []struct {
		name    string
		owner   string
		wantIDs []string
	}{
		{name: "exact match", owner: "bob", wantIDs: []string{"task-2"}},
		{name: "case-insensitive across entries", owner: "alice", wantIDs: []string{"task-1", "task-3"}},
		{name: "surrounding whitespace tolerated", owner: "  bob  ", wantIDs: []string{"task-2"}},
		{name: "empty owner returns unassigned", owner: "", wantIDs: []string{"task-4"}},
		{name: "unknown owner returns none", owner: "carol", wantIDs: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := coord.GetTasksByOwner(context.Background(), tt.owner)
			if err != nil {
				t.Fatalf("GetTasksByOwner failed: %v", err)
			}
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("Expected %d tasks %v, got %d (%v)", len(tt.wantIDs), tt.wantIDs, len(got), ids(got))
			}
			for i, want := range tt.wantIDs {
				if got[i].ID != want {
					t.Errorf("Expected %s at index %d, got %s", want, i, got[i].ID)
				}
			}
		})
	}
}

func ids(summaries []TaskSummary) []string {
	out := make([]string, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, s.ID)
	}
	return out
}
