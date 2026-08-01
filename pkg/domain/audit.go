package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

// Event represents a single auditable action in the system.
type Event struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Action    string                 `json:"action"`
	Actor     string                 `json:"actor"` // "human" or "ai"
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	PrevHash  string                 `json:"prev_hash,omitempty"` // Hash of the preceding event
	Hash      string                 `json:"hash,omitempty"`      // Deterministic hash of this event

	// HashAlgo names the algorithm used to compute Hash. Absent means the
	// original, unversioned scheme.
	//
	// This exists because changing the hash silently invalidated every
	// historical entry once already: commit fe1c290 folded canonicalJSON
	// into the digest, and every event written before it stopped verifying.
	// Nobody noticed for months, because a mismatch is reported as possible
	// tampering and there was no way to tell the two apart. Versioning the
	// algorithm means the next change can be recognised rather than
	// mistaken for an attack.
	HashAlgo string `json:"hash_algo,omitempty"`

	// Type and AggregateID are written by the event-sourced audit path
	// (events.BaseEvent) and are absent from events written directly here.
	// They are carried so verification can reproduce whichever hash the
	// writer actually used — see CalculateHashEventSourced.
	Type        string `json:"type,omitempty"`
	AggregateID string `json:"aggregate_id,omitempty"`
}

// CalculateHashEventSourced reproduces events.BaseEvent.CalculateHash.
//
// Two writers append to events.jsonl and they hash differently: this package
// hashes Action, while BaseEvent hashes Type and AggregateID. An event
// carrying an aggregate ID therefore verifies under one scheme and fails
// under the other. Verification has to try both, because the alternative is
// re-hashing history — which would destroy the tamper-evidence it exists to
// provide.
//
// Keep this in lockstep with events.BaseEvent.CalculateHash. The field order
// is the hash; changing either side alone silently invalidates the log.
func (e *Event) CalculateHashEventSourced() string {
	h := sha256.New()
	h.Write([]byte(e.PrevHash))
	h.Write([]byte(e.ID))
	h.Write([]byte(e.Timestamp.Format(time.RFC3339Nano)))
	h.Write([]byte(e.Type))
	h.Write([]byte(e.AggregateID))
	h.Write([]byte(e.Actor))
	h.Write([]byte(canonicalJSON(e.Metadata)))
	return hex.EncodeToString(h.Sum(nil))
}

// HashAlgoCurrent is the algorithm new events are stamped with.
const HashAlgoCurrent = "sha256-canonical-v1"

// HashMatches reports whether the recorded hash matches either writer's
// scheme.
func (e *Event) HashMatches() bool {
	return e.Hash == e.CalculateHash() || e.Hash == e.CalculateHashEventSourced()
}

// Verifiable reports whether this entry can be checked at all with the
// algorithms this build knows.
//
// An entry stamped with an unknown algorithm is not evidence of tampering —
// it is evidence that it was written by a different version of Roady. Saying
// "possible tampering" in that case is a false accusation, and false
// accusations are how a security signal gets ignored.
func (e *Event) Verifiable() bool {
	return e.HashAlgo == "" || e.HashAlgo == HashAlgoCurrent
}

// CalculateHash generates a deterministic SHA256 hash of the event data.
func (e *Event) CalculateHash() string {
	h := sha256.New()
	// Deterministic sequence: PrevHash + ID + Timestamp + Action + Actor + Metadata
	h.Write([]byte(e.PrevHash))
	h.Write([]byte(e.ID))
	h.Write([]byte(e.Timestamp.Format(time.RFC3339Nano)))
	h.Write([]byte(e.Action))
	h.Write([]byte(e.Actor))
	h.Write([]byte(canonicalJSON(e.Metadata)))
	return hex.EncodeToString(h.Sum(nil))
}

// canonicalJSON produces a deterministic JSON representation of metadata.
// Keys are sorted alphabetically to ensure consistent hashing.
func canonicalJSON(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build ordered map representation
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

// UsageStats tracks the "cost" and telemetry of operations.
type UsageStats struct {
	TotalCommands int            `json:"total_commands"`
	LastCommandAt time.Time      `json:"last_command_at"`
	ProviderStats map[string]int `json:"provider_stats"` // e.g., "gemini-tokens": 500
}
