package project

import (
	"context"
	"errors"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// Mock implementations for testing

type mockPlanRepo struct {
	plan *planning.Plan
	err  error
}

func (m *mockPlanRepo) Load(ctx context.Context) (*planning.Plan, error) {
	return m.plan, m.err
}

func (m *mockPlanRepo) Save(ctx context.Context, plan *planning.Plan) error {
	m.plan = plan
	return m.err
}

type mockStateRepo struct {
	state *planning.ExecutionState
	err   error
}

func (m *mockStateRepo) Load(ctx context.Context) (*planning.ExecutionState, error) {
	return m.state, m.err
}

func (m *mockStateRepo) Save(ctx context.Context, state *planning.ExecutionState) error {
	m.state = state
	return m.err
}

type mockPublisher struct {
	events []string
}

func (m *mockPublisher) PublishPlanApproved(ctx context.Context, planID, approver string) error {
	m.events = append(m.events, "plan.approved:"+planID)
	return nil
}

func (m *mockPublisher) PublishTaskStarted(ctx context.Context, taskID, owner string) error {
	m.events = append(m.events, "task.started:"+taskID)
	return nil
}

func (m *mockPublisher) PublishTaskCompleted(ctx context.Context, taskID, evidence string) error {
	m.events = append(m.events, "task.completed:"+taskID)
	return nil
}

func (m *mockPublisher) PublishTaskBlocked(ctx context.Context, taskID, reason string) error {
	m.events = append(m.events, "task.blocked:"+taskID)
	return nil
}

func (m *mockPublisher) PublishTaskUnblocked(ctx context.Context, taskID string) error {
	m.events = append(m.events, "task.unblocked:"+taskID)
	return nil
}

func createTestPlan() *planning.Plan {
	return &planning.Plan{
		ID:             "plan-1",
		ApprovalStatus: planning.ApprovalPending,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "First Task", DependsOn: []string{}},
			{ID: "task-2", Title: "Second Task", DependsOn: []string{"task-1"}},
			{ID: "task-3", Title: "Third Task", DependsOn: []string{"task-1", "task-2"}},
		},
	}
}

func TestCoordinator_ApprovePlan(t *testing.T) {
	plan := createTestPlan()
	planRepo := &mockPlanRepo{plan: plan}
	stateRepo := &mockStateRepo{state: nil}
	publisher := &mockPublisher{}

	coord := NewCoordinator(planRepo, stateRepo, publisher)

	err := coord.ApprovePlan(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ApprovePlan failed: %v", err)
	}

	// Check plan was approved
	if !planRepo.plan.ApprovalStatus.IsApproved() {
		t.Error("Expected plan to be approved")
	}

	// Check state was initialized
	if stateRepo.state == nil {
		t.Fatal("Expected state to be initialized")
	}
	if len(stateRepo.state.TaskStates) != 3 {
		t.Errorf("Expected 3 task states, got %d", len(stateRepo.state.TaskStates))
	}

	// Check event published
	if len(publisher.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(publisher.events))
	}
}

func TestCoordinator_ApprovePlan_AlreadyApproved(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}
	stateRepo := &mockStateRepo{state: planning.NewExecutionState("plan-1")}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.ApprovePlan(context.Background(), "alice")
	if err != nil {
		t.Errorf("Expected no error for already approved plan, got: %v", err)
	}
}

func TestCoordinator_ApprovePlan_NoPlan(t *testing.T) {
	planRepo := &mockPlanRepo{plan: nil}
	stateRepo := &mockStateRepo{}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.ApprovePlan(context.Background(), "alice")
	if err != ErrNoPlan {
		t.Errorf("Expected ErrNoPlan, got: %v", err)
	}
}

func TestCoordinator_StartTask(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}
	publisher := &mockPublisher{}

	coord := NewCoordinator(planRepo, stateRepo, publisher)

	err := coord.StartTask(context.Background(), "task-1", "alice")
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	// Check task status
	if stateRepo.state.GetTaskStatus("task-1") != planning.StatusInProgress {
		t.Error("Expected task to be in progress")
	}

	// Check owner
	result, _ := stateRepo.state.GetTaskResult("task-1")
	if result.Owner != "alice" {
		t.Errorf("Expected owner alice, got %s", result.Owner)
	}

	// Check event published
	if len(publisher.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(publisher.events))
	}
}

func TestCoordinator_StartTask_DependencyNotMet(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	// Try to start task-2 which depends on task-1
	err := coord.StartTask(context.Background(), "task-2", "alice")
	if err == nil {
		t.Error("Expected error for unmet dependency")
	}

	var depErr *DependencyError
	if !errors.As(err, &depErr) {
		t.Errorf("Expected DependencyError, got: %T", err)
	}
	if depErr != nil && depErr.DependencyID != "task-1" {
		t.Errorf("Expected dependency task-1, got %s", depErr.DependencyID)
	}
}

func TestCoordinator_StartTask_OwnerRequired(t *testing.T) {
	coord := NewCoordinator(nil, nil, nil)

	err := coord.StartTask(context.Background(), "task-1", "")
	if err != ErrOwnerRequired {
		t.Errorf("Expected ErrOwnerRequired, got: %v", err)
	}
}

func TestCoordinator_StartTask_PlanNotApproved(t *testing.T) {
	plan := createTestPlan() // Status is pending
	planRepo := &mockPlanRepo{plan: plan}
	stateRepo := &mockStateRepo{state: planning.NewExecutionState("plan-1")}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.StartTask(context.Background(), "task-1", "alice")
	if err != ErrPlanNotApproved {
		t.Errorf("Expected ErrPlanNotApproved, got: %v", err)
	}
}

func TestCoordinator_CompleteTask(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "alice"}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["task-3"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}
	publisher := &mockPublisher{}

	coord := NewCoordinator(planRepo, stateRepo, publisher)

	unlocked, err := coord.CompleteTask(context.Background(), "task-1", "commit-abc")
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}

	// Check task status
	if stateRepo.state.GetTaskStatus("task-1") != planning.StatusDone {
		t.Error("Expected task to be done")
	}

	// Check evidence
	result, _ := stateRepo.state.GetTaskResult("task-1")
	if len(result.Evidence) != 1 || result.Evidence[0] != "commit-abc" {
		t.Errorf("Expected evidence 'commit-abc', got %v", result.Evidence)
	}

	// Check unlocked tasks
	if len(unlocked) != 1 || unlocked[0] != "task-2" {
		t.Errorf("Expected [task-2] to be unlocked, got %v", unlocked)
	}

	// Check event published
	if len(publisher.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(publisher.events))
	}
}

func TestCoordinator_CompleteTask_InvalidTransition(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	_, err := coord.CompleteTask(context.Background(), "task-1", "")
	if err == nil {
		t.Error("Expected error for completing pending task")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Errorf("Expected TransitionError, got: %T", err)
	}
}

func TestCoordinator_BlockTask(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress}
	stateRepo := &mockStateRepo{state: state}
	publisher := &mockPublisher{}

	coord := NewCoordinator(nil, stateRepo, publisher)

	err := coord.BlockTask(context.Background(), "task-1", "waiting for design review")
	if err != nil {
		t.Fatalf("BlockTask failed: %v", err)
	}

	if stateRepo.state.GetTaskStatus("task-1") != planning.StatusBlocked {
		t.Error("Expected task to be blocked")
	}

	if len(publisher.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(publisher.events))
	}
}

func TestCoordinator_UnblockTask(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusBlocked}
	stateRepo := &mockStateRepo{state: state}
	publisher := &mockPublisher{}

	coord := NewCoordinator(nil, stateRepo, publisher)

	err := coord.UnblockTask(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("UnblockTask failed: %v", err)
	}

	if stateRepo.state.GetTaskStatus("task-1") != planning.StatusPending {
		t.Error("Expected task to be pending")
	}

	if len(publisher.events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(publisher.events))
	}
}

func TestCoordinator_VerifyTask(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusDone}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.VerifyTask(context.Background(), "task-1", "bob")
	if err != nil {
		t.Fatalf("VerifyTask failed: %v", err)
	}

	if stateRepo.state.GetTaskStatus("task-1") != planning.StatusVerified {
		t.Error("Expected task to be verified")
	}
}

func TestDependencyError(t *testing.T) {
	err := &DependencyError{
		TaskID:       "task-2",
		DependencyID: "task-1",
		Status:       "pending",
	}

	if !errors.Is(err, ErrDependenciesNotMet) {
		t.Error("Expected DependencyError to match ErrDependenciesNotMet")
	}

	expected := "task task-2 blocked by dependency task-1 (status: pending)"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestTransitionError(t *testing.T) {
	err := &TransitionError{
		TaskID:     "task-1",
		FromStatus: "pending",
		ToStatus:   "done",
		Event:      "complete",
	}

	if !errors.Is(err, ErrInvalidTransition) {
		t.Error("Expected TransitionError to match ErrInvalidTransition")
	}

	expected := "cannot transition task task-1 from pending to done via complete"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestCoordinator_StartTask_TaskNotFound(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.StartTask(context.Background(), "nonexistent", "alice")
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound, got: %v", err)
	}
}

func TestCoordinator_StartTask_NoState(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}
	stateRepo := &mockStateRepo{state: nil}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.StartTask(context.Background(), "task-1", "alice")
	if err != ErrNoState {
		t.Errorf("Expected ErrNoState, got: %v", err)
	}
}

func TestCoordinator_CompleteTask_TaskNotFound(t *testing.T) {
	plan := createTestPlan()
	plan.ApprovalStatus = planning.ApprovalApproved
	planRepo := &mockPlanRepo{plan: plan}

	state := planning.NewExecutionState("plan-1")
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	_, err := coord.CompleteTask(context.Background(), "nonexistent", "")
	if err != ErrTaskNotFound {
		t.Errorf("Expected ErrTaskNotFound, got: %v", err)
	}
}

func TestCoordinator_CompleteTask_NoState(t *testing.T) {
	plan := createTestPlan()
	planRepo := &mockPlanRepo{plan: plan}
	stateRepo := &mockStateRepo{state: nil}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	_, err := coord.CompleteTask(context.Background(), "task-1", "")
	if err != ErrNoState {
		t.Errorf("Expected ErrNoState, got: %v", err)
	}
}

func TestCoordinator_BlockTask_NoState(t *testing.T) {
	stateRepo := &mockStateRepo{state: nil}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.BlockTask(context.Background(), "task-1", "reason")
	if err != ErrNoState {
		t.Errorf("Expected ErrNoState, got: %v", err)
	}
}

func TestCoordinator_BlockTask_InvalidTransition(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	// Done tasks cannot be blocked
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusDone}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.BlockTask(context.Background(), "task-1", "reason")
	if err == nil {
		t.Error("Expected error for blocking done task")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Errorf("Expected TransitionError, got: %T", err)
	}
}

func TestCoordinator_UnblockTask_NoState(t *testing.T) {
	stateRepo := &mockStateRepo{state: nil}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.UnblockTask(context.Background(), "task-1")
	if err != ErrNoState {
		t.Errorf("Expected ErrNoState, got: %v", err)
	}
}

func TestCoordinator_UnblockTask_InvalidTransition(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.UnblockTask(context.Background(), "task-1")
	if err == nil {
		t.Error("Expected error for unblocking in-progress task")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Errorf("Expected TransitionError, got: %T", err)
	}
}

func TestCoordinator_VerifyTask_NoState(t *testing.T) {
	stateRepo := &mockStateRepo{state: nil}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.VerifyTask(context.Background(), "task-1", "verifier")
	if err != ErrNoState {
		t.Errorf("Expected ErrNoState, got: %v", err)
	}
}

func TestCoordinator_VerifyTask_InvalidTransition(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusPending}
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(nil, stateRepo, nil)

	err := coord.VerifyTask(context.Background(), "task-1", "verifier")
	if err == nil {
		t.Error("Expected error for verifying pending task")
	}

	var transErr *TransitionError
	if !errors.As(err, &transErr) {
		t.Errorf("Expected TransitionError, got: %T", err)
	}
}

func TestCoordinator_GetPlan(t *testing.T) {
	plan := createTestPlan()
	planRepo := &mockPlanRepo{plan: plan}

	coord := NewCoordinator(planRepo, nil, nil)

	got, err := coord.GetPlan(context.Background())
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	if got.ID != plan.ID {
		t.Errorf("Expected plan ID %s, got %s", plan.ID, got.ID)
	}
}

func TestCoordinator_GetState(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	stateRepo := &mockStateRepo{state: state}

	coord := NewCoordinator(nil, stateRepo, nil)

	got, err := coord.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if got == nil {
		t.Error("Expected state to be returned")
	}
}

func TestCoordinator_ApprovePlan_LoadError(t *testing.T) {
	planRepo := &mockPlanRepo{err: errors.New("load error")}
	stateRepo := &mockStateRepo{}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.ApprovePlan(context.Background(), "alice")
	if err == nil {
		t.Error("Expected error from plan load")
	}
}

func TestCoordinator_StartTask_NoPlan(t *testing.T) {
	planRepo := &mockPlanRepo{plan: nil}
	stateRepo := &mockStateRepo{}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	err := coord.StartTask(context.Background(), "task-1", "alice")
	if err != ErrNoPlan {
		t.Errorf("Expected ErrNoPlan, got: %v", err)
	}
}

func TestCoordinator_CompleteTask_NoPlan(t *testing.T) {
	planRepo := &mockPlanRepo{plan: nil}
	stateRepo := &mockStateRepo{}

	coord := NewCoordinator(planRepo, stateRepo, nil)

	_, err := coord.CompleteTask(context.Background(), "task-1", "")
	if err != ErrNoPlan {
		t.Errorf("Expected ErrNoPlan, got: %v", err)
	}
}
