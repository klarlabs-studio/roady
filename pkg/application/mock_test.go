package application_test

import (
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

type MockRepo struct {
	Spec        *spec.ProductSpec
	SpecLock    *spec.ProductSpec // nil: the lock reads as the current spec
	Plan        *planning.Plan
	State       *planning.ExecutionState
	Policy      *domain.PolicyConfig
	Initialized bool
	SaveError   error
	LoadError   error
}

func (m *MockRepo) Initialize() error                    { m.Initialized = true; return nil }
func (m *MockRepo) IsInitialized() bool                  { return m.Initialized }
func (m *MockRepo) SaveSpec(s *spec.ProductSpec) error   { m.Spec = s; return m.SaveError }
func (m *MockRepo) LoadSpec() (*spec.ProductSpec, error) { return m.Spec, m.LoadError }
func (m *MockRepo) SaveSpecLock(s *spec.ProductSpec) error {
	if m.SaveError == nil && s != nil {
		c := *s
		c.Features = append([]spec.Feature(nil), s.Features...)
		m.SpecLock = &c
	}
	return m.SaveError
}
func (m *MockRepo) LoadSpecLock() (*spec.ProductSpec, error) {
	if m.SpecLock != nil {
		return m.SpecLock, m.LoadError
	}
	return m.Spec, m.LoadError
}
func (m *MockRepo) SavePlan(p *planning.Plan) error              { m.Plan = p; return m.SaveError }
func (m *MockRepo) LoadPlan() (*planning.Plan, error)            { return m.Plan, m.LoadError }
func (m *MockRepo) SaveState(s *planning.ExecutionState) error   { m.State = s; return m.SaveError }
func (m *MockRepo) LoadState() (*planning.ExecutionState, error) { return m.State, m.LoadError }
func (m *MockRepo) SavePolicy(c *domain.PolicyConfig) error      { m.Policy = c; return m.SaveError }
func (m *MockRepo) LoadPolicy() (*domain.PolicyConfig, error)    { return m.Policy, m.LoadError }
func (m *MockRepo) RecordEvent(e domain.Event) error             { return m.SaveError }
func (m *MockRepo) LoadEvents() ([]domain.Event, error)          { return []domain.Event{}, m.LoadError }

type MockInspector struct {
	Exists       bool
	NotEmpty     bool
	GitStatusVal string
}

func (m *MockInspector) FileExists(path string) (bool, error)   { return m.Exists, nil }
func (m *MockInspector) FileNotEmpty(path string) (bool, error) { return m.NotEmpty, nil }
func (m *MockInspector) GitStatus(path string) (string, error)  { return m.GitStatusVal, nil }
