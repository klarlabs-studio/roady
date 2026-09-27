package application

import "github.com/felixgeelhaar/roady/pkg/domain"

// domainWorkspaceRepo embeds the full interface so stubs only implement the
// methods a test exercises.
type domainWorkspaceRepo = domain.WorkspaceRepository
