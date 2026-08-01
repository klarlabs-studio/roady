package drift

import (
	"time"
)

type DriftType string

const (
	DriftTypeSpec   DriftType = "spec"   // Spec vs Intent
	DriftTypePlan   DriftType = "plan"   // Plan vs Spec
	DriftTypeCode   DriftType = "code"   // Code vs Plan
	DriftTypePolicy DriftType = "policy" // Policy vs State
)

type DriftCategory string

const (
	CategoryMissing        DriftCategory = "MISSING"        // Something exists in source but not in target
	CategoryOrphan         DriftCategory = "ORPHAN"         // Something exists in target but not in source
	CategoryMismatch       DriftCategory = "MISMATCH"       // Source and target exist but differ (e.g. description/estimate)
	CategoryDebt           DriftCategory = "DEBT"           // Known alignment issue that hasn't been resolved
	CategoryViolation      DriftCategory = "VIOLATION"      // Policy violation
	CategoryImplementation DriftCategory = "IMPLEMENTATION" // Code reality doesn't match state
	CategoryStale          DriftCategory = "STALE"          // The artifact is internally consistent but the repository has moved past it
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Issue represents a single detected discrepancy.
type Issue struct {
	ID          string        `json:"id" yaml:"id"`
	Type        DriftType     `json:"type" yaml:"type"`
	Category    DriftCategory `json:"category" yaml:"category"`
	Severity    Severity      `json:"severity" yaml:"severity"`
	ComponentID string        `json:"component_id" yaml:"component_id"` // ID of the feature or task
	Message     string        `json:"message" yaml:"message"`
	Path        string        `json:"path" yaml:"path"` // Path to the source of drift
	Hint        string        `json:"hint" yaml:"hint"` // Suggested resolution
}

// Report is a collection of issues found during a drift detection run.
type Report struct {
	ID        string    `json:"id" yaml:"id"`
	Issues    []Issue   `json:"issues" yaml:"issues"`
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
}

func (r *Report) HasCriticalDrift() bool {
	for _, i := range r.Issues {
		if i.Severity == SeverityCritical {
			return true
		}
	}
	return false
}
