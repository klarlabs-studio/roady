package planning

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusBlocked    TaskStatus = "blocked"
	StatusDone       TaskStatus = "done"
	StatusVerified   TaskStatus = "verified"
)

type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// Plan represents a derived execution graph from a Spec.
type Plan struct {
	ID             string         `json:"id" yaml:"id"`
	SpecID         string         `json:"spec_id" yaml:"spec_id"` // Links back to the Spec it implements
	Tasks          []Task         `json:"tasks" yaml:"tasks"`
	ApprovalStatus ApprovalStatus `json:"approval_status" yaml:"approval_status"`
	CreatedAt      time.Time      `json:"created_at" yaml:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at" yaml:"updated_at"`
}

type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityMedium TaskPriority = "medium"
	PriorityHigh   TaskPriority = "high"
)

// TaskOrigin records who or what produced a task. Persisted on Task so
// reviewers can scrutinise AI-produced tasks differently from heuristic or
// human-authored ones.
type TaskOrigin string

const (
	// OriginHeuristic marks tasks emitted by the deterministic 1-requirement
	// = 1-task planner. Empty string deserialises to this value as a
	// backward-compatible default for plans persisted before the field
	// existed.
	OriginHeuristic TaskOrigin = "heuristic"
	// OriginAI marks tasks proposed by an AI provider via the AI planning
	// service or smart-decompose.
	OriginAI TaskOrigin = "ai"
	// OriginHuman marks tasks written or amended by a human operator (e.g.,
	// directly editing plan.json or via a future `roady task add`).
	OriginHuman TaskOrigin = "human"
)

// NormalisedOrigin returns the canonical origin for a task, defaulting to
// heuristic when the field is empty so legacy plan.json files keep working.
func (t Task) NormalisedOrigin() TaskOrigin {
	if t.Origin == "" {
		return OriginHeuristic
	}
	return t.Origin
}

// TaskSource records the document and line from which the originating spec
// element was derived. Optional; an empty struct means no provenance was
// recorded.
type TaskSource struct {
	Doc  string `json:"doc,omitempty" yaml:"doc,omitempty"`
	Line int    `json:"line,omitempty" yaml:"line,omitempty"`
}

// IsZero reports whether no source has been recorded for the task.
func (s TaskSource) IsZero() bool { return s.Doc == "" && s.Line == 0 }

// Task is a unit of work (structural intent).
type Task struct {
	ID          string       `json:"id" yaml:"id"`
	Title       string       `json:"title" yaml:"title"`
	Description string       `json:"description" yaml:"description"`
	Priority    TaskPriority `json:"priority" yaml:"priority"`
	Estimate    string       `json:"estimate" yaml:"estimate"`     // e.g., "4h", "1d"
	DependsOn   []string     `json:"depends_on" yaml:"depends_on"` // IDs of tasks this task depends on
	FeatureID   string       `json:"feature_id" yaml:"feature_id"` // Link to the feature in the spec
	Origin      TaskOrigin   `json:"origin,omitempty" yaml:"origin,omitempty"`
	Source      TaskSource   `json:"source,omitempty" yaml:"source,omitempty"`
	// Check is how completion is shown, copied from the requirement when the
	// plan is generated. Nil means the task has no check yet.
	Check *Check `json:"check,omitempty" yaml:"check,omitempty"`
}

// Hash returns a deterministic hash of the plan structure.
func (p *Plan) Hash() string {
	h := sha256.New()
	h.Write([]byte(p.ID))
	h.Write([]byte(p.SpecID))
	for _, t := range p.Tasks {
		h.Write([]byte(t.ID))
	}
	return hex.EncodeToString(h.Sum(nil))
}
