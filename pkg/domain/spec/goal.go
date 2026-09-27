package spec

import (
	"fmt"
	"strings"
)

// Goal is an outcome on the roadmap: why a group of features exists and
// when it is meant to land. A goal can be only an idea — it needs no
// features or requirements yet — so everything a ROADMAP.md holds, from
// "shipped in v0.21" to "someday, maybe" to "never", has a place in roady.
//
// Goals order work; they are not intent in the drift sense. Moving a goal
// between horizons does not change what the plan was approved for, so goals
// are not part of the spec hash.
type Goal struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Horizon is when the goal is meant to land: now, next or later. A
	// shipped or out-of-scope goal has none.
	Horizon Horizon `json:"horizon,omitempty" yaml:"horizon,omitempty"`
	// Status is how far the goal has come; empty reads as planned.
	Status GoalStatus `json:"status,omitempty" yaml:"status,omitempty"`
	// Milestone names the release or checkpoint the goal belongs to
	// ("v0.22.0", "public beta"). Optional.
	Milestone string `json:"milestone,omitempty" yaml:"milestone,omitempty"`
	Source    Source `json:"source,omitempty" yaml:"source,omitempty"`
}

// Horizon is when a goal is meant to land.
type Horizon string

const (
	HorizonNow   Horizon = "now"
	HorizonNext  Horizon = "next"
	HorizonLater Horizon = "later"
)

// Horizons lists the horizons in roadmap order.
var Horizons = []Horizon{HorizonNow, HorizonNext, HorizonLater}

// GoalStatus is how far a goal has come.
type GoalStatus string

const (
	// GoalIdea is worth keeping but not yet agreed; it needs no features.
	GoalIdea GoalStatus = "idea"
	// GoalPlanned is agreed and waiting for, or getting, work.
	GoalPlanned GoalStatus = "planned"
	// GoalShipped is done; its milestone says where it landed.
	GoalShipped GoalStatus = "shipped"
	// GoalOutOfScope is a deliberate no, kept so it is not proposed again.
	GoalOutOfScope GoalStatus = "out_of_scope"
)

// GoalStatuses lists every status.
var GoalStatuses = []GoalStatus{GoalIdea, GoalPlanned, GoalShipped, GoalOutOfScope}

// EffectiveStatus is the status with the default applied.
func (g Goal) EffectiveStatus() GoalStatus {
	if g.Status == "" {
		return GoalPlanned
	}
	return g.Status
}

// Open reports whether the goal is still ahead: an idea or planned.
func (g Goal) Open() bool {
	s := g.EffectiveStatus()
	return s == GoalIdea || s == GoalPlanned
}

// ParseHorizon accepts a horizon name, case-insensitively. Empty is valid.
func ParseHorizon(s string) (Horizon, error) {
	h := Horizon(strings.ToLower(strings.TrimSpace(s)))
	if h == "" {
		return "", nil
	}
	for _, v := range Horizons {
		if h == v {
			return h, nil
		}
	}
	return "", fmt.Errorf("unknown horizon %q: use now, next or later", s)
}

// ParseGoalStatus accepts a status name, case-insensitively; "out-of-scope"
// is accepted for out_of_scope. Empty is valid.
func ParseGoalStatus(s string) (GoalStatus, error) {
	v := GoalStatus(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "-", "_"))
	if v == "" {
		return "", nil
	}
	for _, st := range GoalStatuses {
		if v == st {
			return v, nil
		}
	}
	return "", fmt.Errorf("unknown goal status %q: use idea, planned, shipped or out_of_scope", s)
}

// GoalIndex returns the index of the goal with id, or -1.
func (s *ProductSpec) GoalIndex(id string) int {
	for i := range s.Goals {
		if s.Goals[i].ID == id {
			return i
		}
	}
	return -1
}

// validateGoals checks goal fields and every link to a goal.
func (s *ProductSpec) validateGoals() []error {
	var errs []error
	seen := map[string]bool{}
	for i, g := range s.Goals {
		if g.ID == "" {
			errs = append(errs, fmt.Errorf("goal at index %d missing ID", i))
		} else if seen[g.ID] {
			errs = append(errs, fmt.Errorf("duplicate goal ID: %s", g.ID))
		}
		seen[g.ID] = true
		if strings.TrimSpace(g.Title) == "" {
			errs = append(errs, fmt.Errorf("goal '%s' missing title", g.ID))
		}
		if h, err := ParseHorizon(string(g.Horizon)); err != nil || h != g.Horizon {
			errs = append(errs, fmt.Errorf("goal '%s': unknown horizon %q (now, next or later)", g.ID, g.Horizon))
		}
		if st, err := ParseGoalStatus(string(g.Status)); err != nil || st != g.Status {
			errs = append(errs, fmt.Errorf("goal '%s': unknown status %q (idea, planned, shipped or out_of_scope)", g.ID, g.Status))
		}
	}
	for _, f := range s.Features {
		if f.Goal != "" && !seen[f.Goal] {
			errs = append(errs, fmt.Errorf("feature '%s' links to unknown goal '%s'", f.ID, f.Goal))
		}
		for _, r := range f.Requirements {
			if r.Goal != "" && !seen[r.Goal] {
				errs = append(errs, fmt.Errorf("requirement '%s' links to unknown goal '%s'", r.ID, r.Goal))
			}
		}
	}
	return errs
}

// GoalOf returns the goal a requirement serves: its own link, else its
// feature's. Empty when neither links one.
func (s *ProductSpec) GoalOf(f Feature, r Requirement) string {
	if r.Goal != "" {
		return r.Goal
	}
	return f.Goal
}
