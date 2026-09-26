package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestTaskService_Transition_Mock(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	// 1. Valid
	err := service.TransitionTask("t1", "start", "test-user", "some evidence")
	if err != nil {
		t.Fatal(err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusInProgress {
		t.Error("Expected InProgress")
	}

	// 2. Task not found
	err = service.TransitionTask("missing", "start", "test-user", "")
	if err == nil {
		t.Error("Expected error for missing task")
	}

	// 3. Save error
	repo.SaveError = errors.New("save fail")
	err = service.TransitionTask("t1", "start", "test-user", "")
	if err == nil {
		t.Error("Expected error on save fail")
	}
}

func TestTaskService_Transition_Complete(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "test-user"},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.TransitionTask("t1", "complete", "test-user", "commit abc123")
	if err != nil {
		t.Fatalf("complete transition failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusDone {
		t.Error("Expected Done status after complete")
	}
}

func TestTaskService_Transition_Block(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "test-user"},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.TransitionTask("t1", "block", "test-user", "waiting on external API")
	if err != nil {
		t.Fatalf("block transition failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusBlocked {
		t.Error("Expected Blocked status after block")
	}
}

func TestTaskService_Transition_Unblock(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusBlocked},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.TransitionTask("t1", "unblock", "test-user", "")
	if err != nil {
		t.Fatalf("unblock transition failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusPending {
		t.Error("Expected Pending status after unblock")
	}
}

func TestTaskService_Transition_Verify(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusDone},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.TransitionTask("t1", "verify", "reviewer", "")
	if err != nil {
		t.Fatalf("verify transition failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusVerified {
		t.Error("Expected Verified status after verify")
	}
}

func TestTaskService_Transition_InvalidTransition(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	// Can't complete a pending task directly
	err := service.TransitionTask("t1", "complete", "test-user", "")
	if err == nil {
		t.Error("Expected error for invalid transition from pending to complete")
	}
}

func TestTaskService_Transition_PlanNotApproved(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalPending,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.TransitionTask("t1", "start", "test-user", "")
	if err == nil {
		t.Error("Expected error when plan is not approved")
	}
}

func TestTaskService_Transition_NoPlan(t *testing.T) {
	repo := &MockRepo{
		Plan: nil,
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.TransitionTask("t1", "start", "test-user", "")
	if err == nil {
		t.Error("Expected error when no plan exists")
	}
}

func TestTaskService_LinkTask(t *testing.T) {
	repo := &MockRepo{
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	ref := planning.ExternalRef{
		ID:         "123",
		Identifier: "EXT-1",
		URL:        "https://example.com/EXT-1",
	}
	if err := service.LinkTask("t1", "jira", ref); err != nil {
		t.Fatalf("LinkTask failed: %v", err)
	}
	if repo.State.TaskStates["t1"].ExternalRefs["jira"].Identifier != "EXT-1" {
		t.Fatalf("expected external ref to be stored")
	}
}

func TestTaskService_StartTask_Context(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.StartTask(context.TODO(), "t1", "test-user")
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusInProgress {
		t.Error("Expected InProgress status")
	}
}

func TestTaskService_CompleteTask_Context(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "test-user"},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	unlocked, err := service.CompleteTask(context.TODO(), "t1", "evidence")
	if err != nil {
		t.Fatalf("CompleteTask failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusDone {
		t.Error("Expected Done status")
	}
	// No dependent tasks, so unlocked should be empty or nil
	_ = unlocked
}

func TestTaskService_BlockTask_Context(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.BlockTask(context.TODO(), "t1", "waiting for API")
	if err != nil {
		t.Fatalf("BlockTask failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusBlocked {
		t.Error("Expected Blocked status")
	}
}

func TestTaskService_UnblockTask_Context(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusBlocked},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.UnblockTask(context.TODO(), "t1")
	if err != nil {
		t.Fatalf("UnblockTask failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusPending {
		t.Error("Expected Pending status")
	}
}

func TestTaskService_VerifyTask_Context(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusDone},
			},
		},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	err := service.VerifyTask(context.TODO(), "t1", "reviewer")
	if err != nil {
		t.Fatalf("VerifyTask failed: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusVerified {
		t.Error("Expected Verified status")
	}
}

func TestTaskService_GetCoordinator(t *testing.T) {
	repo := &MockRepo{}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	service := application.NewTaskService(repo, audit, policy)

	coord := service.GetCoordinator()
	if coord == nil {
		t.Error("Expected non-nil coordinator")
	}
}
