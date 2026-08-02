// Package events defines domain events for event sourcing.
package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// DomainEvent is the base interface for all domain events.
type DomainEvent interface {
	EventType() string
	AggregateID() string
	AggregateType() string
	OccurredAt() time.Time
	Version() int
}

// BaseEvent provides common fields for all events.
// Action mirrors Type for backward compatibility with domain.Event JSON format.
type BaseEvent struct {
	ID             string                 `json:"id"`
	Type           string                 `json:"type"`
	Action         string                 `json:"action,omitempty"`
	AggregateID_   string                 `json:"aggregate_id"`
	AggregateType_ string                 `json:"aggregate_type"`
	Timestamp      time.Time              `json:"timestamp"`
	Version_       interface{}            `json:"version"`
	Actor          string                 `json:"actor"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	PrevHash       string                 `json:"prev_hash,omitempty"`
	Hash           string                 `json:"hash,omitempty"`

	// HashAlgo names the algorithm used for Hash. Both writers to
	// events.jsonl must stamp it, or half the log stays unversioned and the
	// stamp cannot be relied on to tell an algorithm change from tampering.
	HashAlgo string `json:"hash_algo,omitempty"`
}

// Version returns the event version as an int.
func (e *BaseEvent) Version() int {
	if e.Version_ == nil {
		return 0
	}
	switch v := e.Version_.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		return 0
	default:
		return 0
	}
}

// EnsureAction sets Action to match Type for backward-compatible JSON serialization.
func (e *BaseEvent) EnsureAction() {
	if e.Action == "" {
		e.Action = e.Type
	}
}

func (e BaseEvent) EventType() string     { return e.Type }
func (e BaseEvent) AggregateID() string   { return e.AggregateID_ }
func (e BaseEvent) AggregateType() string { return e.AggregateType_ }
func (e BaseEvent) OccurredAt() time.Time { return e.Timestamp }

// CalculateHash generates a deterministic SHA256 hash of the event.
func (e *BaseEvent) CalculateHash() string {
	h := sha256.New()
	h.Write([]byte(e.PrevHash))
	h.Write([]byte(e.ID))
	h.Write([]byte(e.Timestamp.Format(time.RFC3339Nano)))
	h.Write([]byte(e.Type))
	h.Write([]byte(e.AggregateID_))
	h.Write([]byte(e.Actor))
	h.Write([]byte(canonicalJSON(e.Metadata)))
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalJSON produces a deterministic JSON representation.
func canonicalJSON(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	ordered := make([]byte, 0, 256)
	ordered = append(ordered, '{')
	for i, k := range keys {
		if i > 0 {
			ordered = append(ordered, ',')
		}
		keyJSON, _ := json.Marshal(k)
		valJSON, _ := json.Marshal(m[k])
		ordered = append(ordered, keyJSON...)
		ordered = append(ordered, ':')
		ordered = append(ordered, valJSON...)
	}
	ordered = append(ordered, '}')
	return string(ordered)
}

// =============================================================================
// Plan Events
// =============================================================================

// PlanCreated is emitted when a new plan is generated.
type PlanCreated struct {
	BaseEvent
	PlanID    string `json:"plan_id"`
	SpecID    string `json:"spec_id"`
	TaskCount int    `json:"task_count"`
	Source    string `json:"source"` // "ai" or "manual"
}

// PlanApproved is emitted when a plan is approved.
type PlanApproved struct {
	BaseEvent
	PlanID   string `json:"plan_id"`
	Approver string `json:"approver"`
}

// PlanRejected is emitted when a plan is rejected.
type PlanRejected struct {
	BaseEvent
	PlanID string `json:"plan_id"`
	Reason string `json:"reason"`
}

// =============================================================================
// Task Events
// =============================================================================

// TaskStarted is emitted when work begins on a task.
type TaskStarted struct {
	BaseEvent
	TaskID string `json:"task_id"`
	Owner  string `json:"owner"`
}

// TaskCompleted is emitted when a task is marked as done.
type TaskCompleted struct {
	BaseEvent
	TaskID   string `json:"task_id"`
	Evidence string `json:"evidence,omitempty"`
}

// TaskVerified is emitted when a task passes verification.
type TaskVerified struct {
	BaseEvent
	TaskID   string `json:"task_id"`
	Verifier string `json:"verifier"`
}

// TaskBlocked is emitted when a task becomes blocked.
type TaskBlocked struct {
	BaseEvent
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// TaskUnblocked is emitted when a blocked task is unblocked.
type TaskUnblocked struct {
	BaseEvent
	TaskID string `json:"task_id"`
}

// TaskTransitioned is emitted for any status change.
type TaskTransitioned struct {
	BaseEvent
	TaskID     string              `json:"task_id"`
	FromStatus planning.TaskStatus `json:"from_status"`
	ToStatus   planning.TaskStatus `json:"to_status"`
}

// =============================================================================
// Sync Events
// =============================================================================

// ExternalRefLinked is emitted when a task is linked to an external system.
type ExternalRefLinked struct {
	BaseEvent
	TaskID     string `json:"task_id"`
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
	URL        string `json:"url,omitempty"`
}

// SyncCompleted is emitted after a successful sync operation.
type SyncCompleted struct {
	BaseEvent
	Provider     string   `json:"provider"`
	TasksUpdated []string `json:"tasks_updated"`
	TasksCreated []string `json:"tasks_created"`
	Errors       []string `json:"errors,omitempty"`
}

// =============================================================================
// Drift Events
// =============================================================================

// DriftDetected is emitted when drift is found between plan and reality.
type DriftDetected struct {
	BaseEvent
	IssueCount int      `json:"issue_count"`
	Severities []string `json:"severities"`
}

// DriftResolved is emitted when drift issues are resolved.
type DriftResolved struct {
	BaseEvent
	ResolvedCount int `json:"resolved_count"`
}

// =============================================================================
// File Events
// =============================================================================

// FileChanged is emitted when a watched file is modified.
type FileChanged struct {
	BaseEvent
	FilePath   string `json:"file_path"`
	ChangeType string `json:"change_type"` // "create", "write", "remove", "rename"
}

// =============================================================================
// Event Type Constants
// =============================================================================

const (
	EventTypePlanCreated       = "plan.created"
	EventTypePlanApproved      = "plan.approved"
	EventTypePlanRejected      = "plan.rejected"
	EventTypeTaskStarted       = "task.started"
	EventTypeTaskCompleted     = "task.completed"
	EventTypeTaskVerified      = "task.verified"
	EventTypeTaskBlocked       = "task.blocked"
	EventTypeTaskUnblocked     = "task.unblocked"
	EventTypeTaskTransitioned  = "task.transitioned"
	EventTypeExternalRefLinked = "external_ref.linked"
	EventTypeSyncCompleted     = "sync.completed"
	EventTypeDriftDetected     = "drift.detected"
	EventTypeDriftAccepted     = "drift.accepted"
	EventTypeDriftResolved     = "drift.resolved"
	EventTypeFileChanged       = "file.changed"

	// EventTypeReportDigest carries a prerendered progress summary out to
	// notification adapters. It is dispatched on demand, never recorded in
	// the audit log — a digest reports on history rather than making it.
	EventTypeReportDigest = "report.digest"

	// Billing events
	EventTypeRateAdded             = "billing.rate_added"
	EventTypeRateRemoved           = "billing.rate_removed"
	EventTypeDefaultRateSet        = "billing.default_rate_set"
	EventTypeTaxConfigured         = "billing.tax_configured"
	EventTypeTimeLogged            = "billing.time_logged"
	EventTypeTaskStarted_Billing   = "billing.task_started"
	EventTypeTaskCompleted_Billing = "billing.task_completed"
)

// AggregateTypes
const (
	AggregateTypePlan    = "plan"
	AggregateTypeTask    = "task"
	AggregateTypeSync    = "sync"
	AggregateTypeBilling = "billing"
)

// HashAlgoCurrent is the algorithm new events are stamped with. It matches
// domain.HashAlgoCurrent: both writers append to the same events.jsonl, so a
// stamp that differed between them would make the version meaningless.
const HashAlgoCurrent = "sha256-canonical-v1"

// HashMatches reports whether the recorded hash reproduces from the content.
func (e *BaseEvent) HashMatches() bool {
	return e.Hash == e.CalculateHash()
}

// Verifiable reports whether this entry can be checked at all with the
// algorithms this build knows.
//
// An entry stamped with an unknown algorithm is not evidence of tampering — it
// is evidence that it was written by a different version of Roady. Saying
// "possible tampering" in that case is a false accusation, and false
// accusations are how a security signal gets ignored.
func (e *BaseEvent) Verifiable() bool {
	return e.HashAlgo == "" || e.HashAlgo == HashAlgoCurrent
}
