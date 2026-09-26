package application

import (
	"context"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

// PlanRepositoryAdapter adapts WorkspaceRepository to project.PlanRepository.
type PlanRepositoryAdapter struct {
	repo domain.WorkspaceRepository
}

// NewPlanRepositoryAdapter creates a new adapter.
func NewPlanRepositoryAdapter(repo domain.WorkspaceRepository) *PlanRepositoryAdapter {
	return &PlanRepositoryAdapter{repo: repo}
}

// Load implements project.PlanRepository.
func (a *PlanRepositoryAdapter) Load(ctx context.Context) (*planning.Plan, error) {
	return a.repo.LoadPlan()
}

// Save implements project.PlanRepository.
func (a *PlanRepositoryAdapter) Save(ctx context.Context, plan *planning.Plan) error {
	return a.repo.SavePlan(plan)
}

// StateRepositoryAdapter adapts WorkspaceRepository to project.StateRepository.
type StateRepositoryAdapter struct {
	repo domain.WorkspaceRepository
}

// NewStateRepositoryAdapter creates a new adapter.
func NewStateRepositoryAdapter(repo domain.WorkspaceRepository) *StateRepositoryAdapter {
	return &StateRepositoryAdapter{repo: repo}
}

// Load implements project.StateRepository.
func (a *StateRepositoryAdapter) Load(ctx context.Context) (*planning.ExecutionState, error) {
	return a.repo.LoadState()
}

// Save implements project.StateRepository.
func (a *StateRepositoryAdapter) Save(ctx context.Context, state *planning.ExecutionState) error {
	return a.repo.SaveState(state)
}

// AuditEventPublisher adapts AuditLogger to project.EventPublisher.
type AuditEventPublisher struct {
	audit domain.AuditLogger
}

// NewAuditEventPublisher creates a new adapter.
func NewAuditEventPublisher(audit domain.AuditLogger) *AuditEventPublisher {
	return &AuditEventPublisher{audit: audit}
}

// PublishPlanApproved implements project.EventPublisher.
func (p *AuditEventPublisher) PublishPlanApproved(ctx context.Context, planID, approver string) error {
	return p.audit.Log("plan.approved", approver, map[string]interface{}{
		"plan_id": planID,
	})
}

// PublishTaskStarted implements project.EventPublisher.
func (p *AuditEventPublisher) PublishTaskStarted(ctx context.Context, taskID, owner string) error {
	return p.audit.Log("task.started", owner, map[string]interface{}{
		"task_id": taskID,
	})
}

// PublishTaskCompleted implements project.EventPublisher.
func (p *AuditEventPublisher) PublishTaskCompleted(ctx context.Context, taskID, evidence string) error {
	return p.audit.Log("task.completed", "system", map[string]interface{}{
		"task_id":  taskID,
		"evidence": evidence,
	})
}

// PublishTaskBlocked implements project.EventPublisher.
func (p *AuditEventPublisher) PublishTaskBlocked(ctx context.Context, taskID, reason string) error {
	return p.audit.Log("task.blocked", "system", map[string]interface{}{
		"task_id": taskID,
		"reason":  reason,
	})
}

// PublishTaskUnblocked implements project.EventPublisher.
func (p *AuditEventPublisher) PublishTaskUnblocked(ctx context.Context, taskID string) error {
	return p.audit.Log("task.unblocked", "system", map[string]interface{}{
		"task_id": taskID,
	})
}

// NewProjectCoordinator creates a Coordinator using the workspace repository.
func NewProjectCoordinator(repo domain.WorkspaceRepository, audit domain.AuditLogger) *project.Coordinator {
	planRepo := NewPlanRepositoryAdapter(repo)
	stateRepo := NewStateRepositoryAdapter(repo)
	var publisher project.EventPublisher
	if audit != nil {
		publisher = NewAuditEventPublisher(audit)
	}
	return project.NewCoordinator(planRepo, stateRepo, publisher)
}

// Ensure adapters implement their interfaces
var (
	_ project.PlanRepository  = (*PlanRepositoryAdapter)(nil)
	_ project.StateRepository = (*StateRepositoryAdapter)(nil)
	_ project.EventPublisher  = (*AuditEventPublisher)(nil)
)
