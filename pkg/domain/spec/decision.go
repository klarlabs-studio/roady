package spec

import (
	"fmt"
	"strings"
)

// Decision records a choice made along the way — what was decided, why, and
// what follows — attached to the goals, features and requirements it
// constrains. An agent picking up a task sees the decisions behind it in its
// brief instead of re-litigating them, or silently reversing one it never
// knew was made.
//
// Like goals, decisions are not part of the spec hash: recording one does
// not change what the plan was approved for.
type Decision struct {
	ID           string `json:"id" yaml:"id"`
	Title        string `json:"title" yaml:"title"`
	Context      string `json:"context,omitempty" yaml:"context,omitempty"`
	Choice       string `json:"choice" yaml:"choice"`
	Consequences string `json:"consequences,omitempty" yaml:"consequences,omitempty"`
	// Date is when it was decided, YYYY-MM-DD.
	Date string `json:"date,omitempty" yaml:"date,omitempty"`
	// Status is accepted (the default) or superseded.
	Status       DecisionStatus `json:"status,omitempty" yaml:"status,omitempty"`
	SupersededBy string         `json:"superseded_by,omitempty" yaml:"superseded_by,omitempty"`
	// What it applies to. A decision linked to nothing applies to the
	// whole project.
	Goals        []string `json:"goals,omitempty" yaml:"goals,omitempty"`
	Features     []string `json:"features,omitempty" yaml:"features,omitempty"`
	Requirements []string `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

// DecisionStatus is whether a decision still stands.
type DecisionStatus string

const (
	DecisionAccepted   DecisionStatus = "accepted"
	DecisionSuperseded DecisionStatus = "superseded"
)

// Stands reports whether the decision is in force.
func (d Decision) Stands() bool { return d.Status == "" || d.Status == DecisionAccepted }

// DecisionIndex returns the index of the decision with id, or -1.
func (s *ProductSpec) DecisionIndex(id string) int {
	for i := range s.Decisions {
		if s.Decisions[i].ID == id {
			return i
		}
	}
	return -1
}

// DecisionsFor returns the standing decisions that apply to a requirement
// of a feature, or to a task under a goal: those linked to the requirement,
// the feature, the goal, or to nothing (project-wide). Newest first.
func (s *ProductSpec) DecisionsFor(featureID, requirementID, goalID string) []Decision {
	var out []Decision
	for _, d := range s.Decisions {
		if !d.Stands() {
			continue
		}
		global := len(d.Goals)+len(d.Features)+len(d.Requirements) == 0
		if global || (requirementID != "" && contains(d.Requirements, requirementID)) ||
			(featureID != "" && contains(d.Features, featureID)) || (goalID != "" && contains(d.Goals, goalID)) {
			out = append(out, d)
		}
	}
	// Dates are YYYY-MM-DD, so string order is time order; keep file order
	// among the same day.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Date > out[j-1].Date; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// validateDecisions checks decision fields and links.
func (s *ProductSpec) validateDecisions() []error {
	var errs []error
	goals, features, reqs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, g := range s.Goals {
		goals[g.ID] = true
	}
	for _, f := range s.Features {
		features[f.ID] = true
		for _, r := range f.Requirements {
			reqs[r.ID] = true
		}
	}
	seen := map[string]bool{}
	for i, d := range s.Decisions {
		switch {
		case d.ID == "":
			errs = append(errs, fmt.Errorf("decision at index %d missing ID", i))
		case seen[d.ID]:
			errs = append(errs, fmt.Errorf("duplicate decision ID: %s", d.ID))
		}
		seen[d.ID] = true
		if strings.TrimSpace(d.Title) == "" || strings.TrimSpace(d.Choice) == "" {
			errs = append(errs, fmt.Errorf("decision '%s' needs a title and a choice", d.ID))
		}
		if d.Status != "" && d.Status != DecisionAccepted && d.Status != DecisionSuperseded {
			errs = append(errs, fmt.Errorf("decision '%s': unknown status %q (accepted or superseded)", d.ID, d.Status))
		}
		for _, g := range d.Goals {
			if !goals[g] {
				errs = append(errs, fmt.Errorf("decision '%s' links to unknown goal '%s'", d.ID, g))
			}
		}
		for _, f := range d.Features {
			if !features[f] {
				errs = append(errs, fmt.Errorf("decision '%s' links to unknown feature '%s'", d.ID, f))
			}
		}
		for _, r := range d.Requirements {
			if !reqs[r] {
				errs = append(errs, fmt.Errorf("decision '%s' links to unknown requirement '%s'", d.ID, r))
			}
		}
	}
	for _, d := range s.Decisions {
		if d.SupersededBy != "" && !seen[d.SupersededBy] {
			errs = append(errs, fmt.Errorf("decision '%s' is superseded by unknown decision '%s'", d.ID, d.SupersededBy))
		}
	}
	return errs
}
