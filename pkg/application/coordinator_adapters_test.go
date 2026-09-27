package application_test

import (
	"context"
	"errors"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// Shared mock for repository adapter tests
type adapterMockRepo struct {
	Spec      *spec.ProductSpec
	Plan      *planning.Plan
	State     *planning.ExecutionState
	Policy    *domain.PolicyConfig
	SaveError error
	LoadError error
}

func (m *adapterMockRepo) Initialize() error                        { return nil }
func (m *adapterMockRepo) IsInitialized() bool                      { return true }
func (m *adapterMockRepo) SaveSpec(s *spec.ProductSpec) error       { m.Spec = s; return m.SaveError }
func (m *adapterMockRepo) LoadSpec() (*spec.ProductSpec, error)     { return m.Spec, m.LoadError }
func (m *adapterMockRepo) SaveSpecLock(s *spec.ProductSpec) error   { return m.SaveError }
func (m *adapterMockRepo) LoadSpecLock() (*spec.ProductSpec, error) { return m.Spec, m.LoadError }
func (m *adapterMockRepo) SavePlan(p *planning.Plan) error          { m.Plan = p; return m.SaveError }
func (m *adapterMockRepo) LoadPlan() (*planning.Plan, error)        { return m.Plan, m.LoadError }
func (m *adapterMockRepo) SaveState(s *planning.ExecutionState) error {
	m.State = s
	return m.SaveError
}
func (m *adapterMockRepo) LoadState() (*planning.ExecutionState, error) { return m.State, m.LoadError }
func (m *adapterMockRepo) SavePolicy(c *domain.PolicyConfig) error      { m.Policy = c; return m.SaveError }
func (m *adapterMockRepo) LoadPolicy() (*domain.PolicyConfig, error)    { return m.Policy, m.LoadError }
func (m *adapterMockRepo) RecordEvent(e domain.Event) error             { return m.SaveError }
func (m *adapterMockRepo) LoadEvents() ([]domain.Event, error)          { return []domain.Event{}, m.LoadError }

type adapterMockAudit struct {
	logs []map[string]interface{}
}

func (m *adapterMockAudit) Log(action, actor string, metadata map[string]interface{}) error {
	m.logs = append(m.logs, map[string]interface{}{
		"action":   action,
		"actor":    actor,
		"metadata": metadata,
	})
	return nil
}

func TestPlanRepositoryAdapter_Load(t *testing.T) {
	plan := &planning.Plan{ID: "test-plan"}
	repo := &adapterMockRepo{Plan: plan}
	adapter := application.NewPlanRepositoryAdapter(repo)

	loaded, err := adapter.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.ID != "test-plan" {
		t.Errorf("expected plan ID test-plan, got %s", loaded.ID)
	}
}

func TestPlanRepositoryAdapter_Load_Error(t *testing.T) {
	expectedErr := errors.New("load error")
	repo := &adapterMockRepo{LoadError: expectedErr}
	adapter := application.NewPlanRepositoryAdapter(repo)

	_, err := adapter.Load(context.Background())
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestPlanRepositoryAdapter_Save(t *testing.T) {
	repo := &adapterMockRepo{}
	adapter := application.NewPlanRepositoryAdapter(repo)

	plan := &planning.Plan{ID: "new-plan"}
	err := adapter.Save(context.Background(), plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.Plan.ID != "new-plan" {
		t.Errorf("expected saved plan ID new-plan, got %s", repo.Plan.ID)
	}
}

func TestStateRepositoryAdapter_Load(t *testing.T) {
	state := planning.NewExecutionState("test-plan")
	repo := &adapterMockRepo{State: state}
	adapter := application.NewStateRepositoryAdapter(repo)

	loaded, err := adapter.Load(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.ProjectID != "test-plan" {
		t.Errorf("expected state project ID test-plan, got %s", loaded.ProjectID)
	}
}

func TestStateRepositoryAdapter_Save(t *testing.T) {
	repo := &adapterMockRepo{}
	adapter := application.NewStateRepositoryAdapter(repo)

	state := planning.NewExecutionState("new-plan")
	err := adapter.Save(context.Background(), state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.State.ProjectID != "new-plan" {
		t.Errorf("expected saved state project ID new-plan, got %s", repo.State.ProjectID)
	}
}

func TestAuditEventPublisher_PublishPlanApproved(t *testing.T) {
	audit := &adapterMockAudit{}
	publisher := application.NewAuditEventPublisher(audit)

	err := publisher.PublishPlanApproved(context.Background(), "plan-1", "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(audit.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(audit.logs))
	}
	if audit.logs[0]["action"] != "plan.approved" {
		t.Errorf("expected action plan.approved, got %v", audit.logs[0]["action"])
	}
	if audit.logs[0]["actor"] != "alice" {
		t.Errorf("expected actor alice, got %v", audit.logs[0]["actor"])
	}
}

func TestAuditEventPublisher_PublishTaskCompleted(t *testing.T) {
	audit := &adapterMockAudit{}
	publisher := application.NewAuditEventPublisher(audit)

	err := publisher.PublishTaskCompleted(context.Background(), "task-1", "commit-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(audit.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(audit.logs))
	}
	if audit.logs[0]["action"] != "task.completed" {
		t.Errorf("expected action task.completed, got %v", audit.logs[0]["action"])
	}
}

func TestAuditEventPublisher_PublishTaskBlocked(t *testing.T) {
	audit := &adapterMockAudit{}
	publisher := application.NewAuditEventPublisher(audit)

	err := publisher.PublishTaskBlocked(context.Background(), "task-1", "waiting for review")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(audit.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(audit.logs))
	}
	if audit.logs[0]["action"] != "task.blocked" {
		t.Errorf("expected action task.blocked, got %v", audit.logs[0]["action"])
	}
}

func TestAuditEventPublisher_PublishTaskUnblocked(t *testing.T) {
	audit := &adapterMockAudit{}
	publisher := application.NewAuditEventPublisher(audit)

	err := publisher.PublishTaskUnblocked(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(audit.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(audit.logs))
	}
	if audit.logs[0]["action"] != "task.unblocked" {
		t.Errorf("expected action task.unblocked, got %v", audit.logs[0]["action"])
	}
}

func TestNewProjectCoordinator(t *testing.T) {
	repo := &adapterMockRepo{
		Plan:  &planning.Plan{ID: "test-plan", ApprovalStatus: planning.ApprovalPending},
		State: planning.NewExecutionState("test-plan"),
	}
	audit := &adapterMockAudit{}

	coordinator := application.NewProjectCoordinator(repo, audit)
	if coordinator == nil {
		t.Fatal("expected non-nil coordinator")
	}
}

func TestNewProjectCoordinator_NilAudit(t *testing.T) {
	repo := &adapterMockRepo{
		Plan:  &planning.Plan{ID: "test-plan", ApprovalStatus: planning.ApprovalPending},
		State: planning.NewExecutionState("test-plan"),
	}

	// Should not panic with nil audit
	coordinator := application.NewProjectCoordinator(repo, nil)
	if coordinator == nil {
		t.Fatal("expected non-nil coordinator")
	}
}

func TestProjectCoordinator_ApprovePlanViaAdapter(t *testing.T) {
	repo := &adapterMockRepo{
		Plan: &planning.Plan{
			ID:             "test-plan",
			ApprovalStatus: planning.ApprovalPending,
			Tasks: []planning.Task{
				{ID: "task-1", Title: "Task 1"},
			},
		},
		State: nil, // Will be initialized
	}
	audit := &adapterMockAudit{}

	coordinator := application.NewProjectCoordinator(repo, audit)
	err := coordinator.ApprovePlan(context.Background(), "alice")
	if err != nil {
		t.Fatalf("ApprovePlan failed: %v", err)
	}

	// Check plan was approved
	if !repo.Plan.ApprovalStatus.IsApproved() {
		t.Error("expected plan to be approved")
	}

	// Check state was initialized
	if repo.State == nil {
		t.Fatal("expected state to be initialized")
	}
	if len(repo.State.TaskStates) != 1 {
		t.Errorf("expected 1 task state, got %d", len(repo.State.TaskStates))
	}

	// Check audit event was published
	if len(audit.logs) != 1 {
		t.Errorf("expected 1 audit log, got %d", len(audit.logs))
	}
	if audit.logs[0]["action"] != "plan.approved" {
		t.Errorf("expected action plan.approved, got %v", audit.logs[0]["action"])
	}
}

func TestAuditEventPublisher_PublishTaskStarted(t *testing.T) {
	audit := &adapterMockAudit{}
	publisher := application.NewAuditEventPublisher(audit)

	if err := publisher.PublishTaskStarted(context.Background(), "task-1", "bob"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(audit.logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(audit.logs))
	}
	if audit.logs[0]["action"] != "task.started" {
		t.Errorf("expected action task.started, got %v", audit.logs[0]["action"])
	}
	metadata := audit.logs[0]["metadata"].(map[string]interface{})
	if metadata["task_id"] != "task-1" {
		t.Errorf("expected task_id task-1, got %v", metadata["task_id"])
	}
}
