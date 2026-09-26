package wiring

import (
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// AppServices exposes the application layer services wired together with a workspace.
type AppServices struct {
	Workspace  *Workspace
	Init       *application.InitService
	Spec       *application.SpecService
	Plan       *application.PlanService
	Drift      *application.DriftService
	Policy     *application.PolicyService
	Task       *application.TaskService
	Git        *application.GitService
	Audit      *application.EventSourcedAuditService // Event-sourced audit with dispatcher and projections
	Prompt     *application.PromptService            // Builds model prompts; Roady runs no inference
	AuditTrail *application.AuditTrailService        // Evidence trails for GRC review
	Dispatch   *application.DispatchService          // Hands a ready task to a subagent
	Capture    *application.CaptureService           // One write for intent of any size
	Publisher  *storage.InMemoryEventPublisher
}

// BuildAppServices constructs the workbench of services for a repo root.
//
// Roady runs no inference of its own: PromptService assembles the context a
// model needs and hands it to the caller, which already has one.
func BuildAppServices(root string) (*AppServices, error) {
	return BuildAppServicesForProject(root, "")
}

// BuildAppServicesForProject constructs services scoped to a sub-project under
// <root>/.roady/projects/<project>/. When project is empty, behaves like
// BuildAppServices and uses the root project at <root>/.roady/.
func BuildAppServicesForProject(root, project string) (*AppServices, error) {
	workspace, err := NewWorkspaceForProject(root, project)
	if err != nil {
		return nil, err
	}
	return buildServices(workspace)
}

// buildServices is the shared implementation for building app services.
func buildServices(workspace *Workspace) (*AppServices, error) {
	// Create event store and publisher for event-sourced audit.
	// Events live next to the project's other files (so sub-projects have isolated event streams).
	eventStore, err := storage.NewFileEventStore(workspace.Repo.ProjectBase())
	if err != nil {
		return nil, fmt.Errorf("create event store: %w", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	// Create event-sourced audit service with dispatcher and projections
	auditSvc, err := application.NewEventSourcedAuditService(eventStore, publisher)
	if err != nil {
		return nil, fmt.Errorf("create event-sourced audit: %w", err)
	}
	// Every event this process records carries the agent and session behind
	// it, so an audit trail can answer which agent did what.
	auditSvc.SetProvenance(AmbientProvenance())

	// Create and wire event dispatcher with handlers
	dispatcher := events.NewEventDispatcher()
	dispatcher.Register(events.NewLoggingHandler(nil).Registration())
	dispatcher.Register(events.NewDriftWarningHandler(nil, nil).Registration())
	dispatcher.Register(events.NewTaskTransitionHandler(nil).Registration())
	auditSvc.SetDispatcher(dispatcher)

	// Create services in dependency order
	policySvc := application.NewPolicyService(workspace.Repo)
	planSvc := application.NewPlanService(workspace.Repo, auditSvc)
	// Cross-project dependencies (@project:task-id) resolve against sibling
	// sub-projects under .roady/projects/.
	planSvc.GetCoordinator().SetExternalResolver(application.NewSubProjectResolver(workspace.Repo.Root()))
	taskSvc := application.NewTaskService(workspace.Repo, auditSvc, policySvc)
	driftSvc := application.NewDriftService(workspace.Repo, auditSvc, storage.NewCodebaseInspector(), policySvc)
	// Staleness detection needs to know how far the repository has moved.
	driftSvc.SetActivityInspector(storage.NewGitActivityInspector(workspace.Repo.Root()))

	dispatchSvc := application.NewDispatchService(workspace.Repo, planSvc, taskSvc)
	// Attribute a dispatched claim to the subagent, not to the dispatcher.
	dispatchSvc.SetAuditProvenance(workspace.Audit, auditSvc)

	services := &AppServices{
		Workspace:  workspace,
		Init:       application.NewInitService(workspace.Repo, auditSvc),
		Spec:       application.NewSpecService(workspace.Repo),
		Plan:       planSvc,
		Drift:      driftSvc,
		Policy:     policySvc,
		Task:       taskSvc,
		Capture:    application.NewCaptureService(workspace.Repo, auditSvc),
		Git:        application.NewGitService(workspace.Repo, taskSvc),
		Audit:      auditSvc,
		Prompt:     application.NewPromptService(workspace.Repo),
		AuditTrail: application.NewAuditTrailService(auditSvc, workspace.Audit, planSvc, workspace.Repo),
		Dispatch:   dispatchSvc,
		Publisher:  publisher,
	}

	return services, nil
}
