package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// ProductSpec represents the top-level specification of what is being built.
type ProductSpec struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description" yaml:"description"`
	// Goals are the roadmap: outcomes with a horizon, which features link to.
	Goals       []Goal       `json:"goals,omitempty" yaml:"goals,omitempty"`
	Features    []Feature    `json:"features" yaml:"features"`
	Constraints []Constraint `json:"constraints" yaml:"constraints"`
	Version     string       `json:"version" yaml:"version"`
}

// Source pins a spec element back to the document it was derived from.
// Optional: Doc empty means "no source recorded" (e.g. specs authored by
// hand or programmatically). Line is 1-based.
type Source struct {
	Doc  string `json:"doc,omitempty" yaml:"doc,omitempty"`
	Line int    `json:"line,omitempty" yaml:"line,omitempty"`
}

// IsZero reports whether no source has been recorded.
func (s Source) IsZero() bool { return s.Doc == "" && s.Line == 0 }

// String returns "doc:line" when both are present, "doc" when only Doc is
// set, or "" when no source has been recorded.
func (s Source) String() string {
	if s.IsZero() {
		return ""
	}
	if s.Line > 0 {
		return s.Doc + ":" + itoa(s.Line)
	}
	return s.Doc
}

// itoa avoids pulling strconv just for this helper.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Feature represents a specific functional unit within the spec.
type Feature struct {
	ID           string        `json:"id" yaml:"id"`
	Title        string        `json:"title" yaml:"title"`
	Description  string        `json:"description" yaml:"description"`
	Requirements []Requirement `json:"requirements" yaml:"requirements"`
	Source       Source        `json:"source,omitempty" yaml:"source,omitempty"`
	// Goal is the ID of the goal this feature serves. Optional.
	Goal string `json:"goal,omitempty" yaml:"goal,omitempty"`
}

// Requirement represents a granular condition that a feature must satisfy.
type Requirement struct {
	ID          string   `json:"id" yaml:"id"`
	Title       string   `json:"title" yaml:"title"`
	Description string   `json:"description" yaml:"description"`
	Priority    string   `json:"priority" yaml:"priority"`
	Estimate    string   `json:"estimate" yaml:"estimate"`
	DependsOn   []string `json:"depends_on" yaml:"depends_on"`
	Source      Source   `json:"source,omitempty" yaml:"source,omitempty"`
	// Check states how the requirement is shown to be met: a command (run)
	// or a named manual check (manual). Copied onto the task at planning.
	Check *Check `json:"check,omitempty" yaml:"check,omitempty"`
	// Goal links the requirement to a goal other than its feature's.
	Goal string `json:"goal,omitempty" yaml:"goal,omitempty"`
}

// Check is a requirement's acceptance check. It mirrors planning.Check; the
// spec package does not depend on planning, so the plan service maps one to
// the other.
type Check struct {
	Run    string `json:"run,omitempty" yaml:"run,omitempty"`
	Manual string `json:"manual,omitempty" yaml:"manual,omitempty"`
}

// Constraint represents non-functional requirements or policies.
type Constraint struct {
	ID          string `json:"id" yaml:"id"`
	Description string `json:"description" yaml:"description"`
}

// Hash returns a deterministic hash of the spec for drift detection.
func (s *ProductSpec) Hash() string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s:%s:%s", s.ID, s.Version, s.Title)

	// Hash features and descriptions
	for _, f := range s.Features {
		h.Write([]byte(f.ID))
		h.Write([]byte(f.Description))
		for _, r := range f.Requirements {
			h.Write([]byte(r.ID))
			h.Write([]byte(r.Description))
			// A check is part of the intent: loosening it must show up as a
			// spec change. Hashed only when present, so specs without checks
			// keep the hash they already had and existing locks stay valid.
			if r.Check != nil {
				_, _ = fmt.Fprintf(h, "check:%s|%s", r.Check.Run, r.Check.Manual)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Validate checks the spec for structural integrity.
func (s *ProductSpec) Validate() []error {
	var errs []error
	if s.ID == "" {
		errs = append(errs, fmt.Errorf("spec ID is required"))
	}
	if s.Title == "" {
		errs = append(errs, fmt.Errorf("spec Title is required"))
	}
	if len(s.Features) == 0 {
		errs = append(errs, fmt.Errorf("spec must have at least one feature"))
	}

	seenIDs := make(map[string]bool)
	for i, f := range s.Features {
		if f.ID == "" {
			errs = append(errs, fmt.Errorf("feature at index %d missing ID", i))
		}
		if seenIDs[f.ID] {
			errs = append(errs, fmt.Errorf("duplicate feature ID: %s", f.ID))
		}
		seenIDs[f.ID] = true

		for j, r := range f.Requirements {
			if r.ID == "" {
				errs = append(errs, fmt.Errorf("feature '%s' requirement at index %d missing ID", f.ID, j))
			}
			if r.Title == "" {
				errs = append(errs, fmt.Errorf("feature '%s' requirement '%s' missing title", f.ID, r.ID))
			}
			if r.Check != nil {
				run, manual := strings.TrimSpace(r.Check.Run) != "", strings.TrimSpace(r.Check.Manual) != ""
				if run == manual {
					errs = append(errs, fmt.Errorf("feature '%s' requirement '%s': check needs exactly one of run (a command) or manual (a description)", f.ID, r.ID))
				}
			}
		}
	}
	return append(errs, s.validateGoals()...)
}
