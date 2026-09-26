package wiring

import (
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// Workspace bundles core infrastructure dependencies.
type Workspace struct {
	Repo  *storage.FilesystemRepository
	Audit *application.AuditService
}

// NewWorkspace constructs a workspace scoped to the root project at <root>/.roady/.
// For a sub-project (<root>/.roady/projects/<name>/) use NewWorkspaceForProject.
func NewWorkspace(root string) *Workspace {
	ws, _ := NewWorkspaceForProject(root, "")
	return ws
}

// NewWorkspaceForProject constructs a workspace scoped to a named sub-project
// at <root>/.roady/projects/<project>/. When project is empty, behaves like
// NewWorkspace. Returns an error if the project name is invalid.
func NewWorkspaceForProject(root, project string) (*Workspace, error) {
	repo, err := storage.NewFilesystemRepositoryForProject(root, project)
	if err != nil {
		return nil, err
	}

	auditSvc := application.NewAuditService(repo)
	// Every event recorded through this workspace carries the agent and
	// session behind it. The CLI builds task services from here rather than
	// from BuildAppServices, so stamping only the event-sourced service
	// would leave CLI-driven history unattributed.
	auditSvc.SetProvenance(AmbientProvenance())

	return &Workspace{
		Repo:  repo,
		Audit: auditSvc,
	}, nil
}
