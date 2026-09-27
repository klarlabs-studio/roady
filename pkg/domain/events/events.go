// Package events defines domain events for event sourcing.
package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

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

// Event type names.
const (
	EventTypePlanCreated      = "plan.created"
	EventTypePlanApproved     = "plan.approved"
	EventTypePlanRejected     = "plan.rejected"
	EventTypeTaskStarted      = "task.started"
	EventTypeTaskCompleted    = "task.completed"
	EventTypeTaskVerified     = "task.verified"
	EventTypeTaskBlocked      = "task.blocked"
	EventTypeTaskUnblocked    = "task.unblocked"
	EventTypeTaskTransitioned = "task.transitioned"
	EventTypeDriftDetected    = "drift.detected"
	EventTypeDriftAccepted    = "drift.accepted"
	EventTypeDriftResolved    = "drift.resolved"
)

// AggregateTypes
const (
	AggregateTypePlan = "plan"
	AggregateTypeTask = "task"
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
