package domain

import (
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// WorkspaceRepository handles the persistence of roady artifacts in the .roady/ directory.
// This is a composed interface that embeds all domain-specific repositories for backward compatibility.
// New code should depend on the specific sub-interfaces they need.
type WorkspaceRepository interface {
	// Workspace lifecycle
	Initialize() error
	IsInitialized() bool

	// Spec operations (use spec.Repository for new code)
	SaveSpec(spec *spec.ProductSpec) error
	LoadSpec() (*spec.ProductSpec, error)
	SaveSpecLock(spec *spec.ProductSpec) error
	LoadSpecLock() (*spec.ProductSpec, error)

	// Plan operations (use planning.PlanRepository for new code)
	SavePlan(plan *planning.Plan) error
	LoadPlan() (*planning.Plan, error)

	// State operations (use planning.StateRepository for new code)
	SaveState(state *planning.ExecutionState) error
	LoadState() (*planning.ExecutionState, error)

	// Policy operations (use policy.Repository for new code)
	SavePolicy(cfg *PolicyConfig) error
	LoadPolicy() (*PolicyConfig, error)

	// Audit operations (use AuditRepository for new code)
	RecordEvent(event Event) error
	LoadEvents() ([]Event, error)
}

// PolicyConfig is the serialized representation of policy.yaml
// Deprecated: Use policy.PolicyConfig instead
type PolicyConfig = policy.PolicyConfig
