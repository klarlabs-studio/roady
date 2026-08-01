// Package audit models an evidence trail for governance, risk, and
// compliance review: the complete recorded history of a task, or of
// everything one agent or session touched.
//
// # What a trail proves, and what it does not
//
// A trail is assembled from the hash-chained event log, so it is
// tamper-evident: if any recorded entry were altered or removed after the
// fact, chain verification fails and the trail says so. That is a real
// guarantee and worth having.
//
// It is not proof of identity. Actor, agent, and session are asserted by the
// caller and never authenticated, so a trail attests to what was claimed at
// the time, not to who acted. See pkg/domain/provenance for the full
// discussion. Present a trail as "the complete, tamper-evident record of what
// was asserted", never as "proof that agent X did this".
package audit

import (
	"sort"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
)

// Trail is the recorded history behind one subject — a task, an agent, or a
// session.
type Trail struct {
	Subject     Subject     `json:"subject"`
	GeneratedAt time.Time   `json:"generated_at"`
	Entries     []Entry     `json:"entries"`
	Integrity   Integrity   `json:"integrity"`
	Task        *TaskFacts  `json:"task,omitempty"`
	Actors      []ActorRoll `json:"actors"`
}

// Subject names what the trail was assembled about.
type Subject struct {
	Kind      string     `json:"kind"` // task | agent | session
	ID        string     `json:"id"`
	Since     *time.Time `json:"since,omitempty"`
	AgentName string     `json:"agent,omitempty"`
	SessionID string     `json:"session_id,omitempty"`
}

// Entry is one recorded event, flattened for review.
type Entry struct {
	At        time.Time `json:"at"`
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	Agent     string    `json:"agent,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Surface   string    `json:"surface,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	EventHash string    `json:"event_hash,omitempty"`
}

// Integrity reports the verification status of the underlying event chain.
// Verified being false is itself an audit finding, which is why it carries
// the reasons rather than a bare boolean.
type Integrity struct {
	Verified     bool     `json:"verified"`
	EventsInLog  int      `json:"events_in_log"`
	Problems     []string `json:"problems,omitempty"`
	CheckedChain bool     `json:"checked_chain"`
}

// TaskFacts carries the non-event evidence attached to a task: what it was
// meant to implement, and what was offered as proof it happened.
type TaskFacts struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	Owner        string            `json:"owner,omitempty"`
	FeatureID    string            `json:"feature_id,omitempty"`
	Origin       string            `json:"origin,omitempty"`
	SourceDoc    string            `json:"source_doc,omitempty"`
	SourceLine   int               `json:"source_line,omitempty"`
	Evidence     []string          `json:"evidence,omitempty"`
	ExternalRefs map[string]string `json:"external_refs,omitempty"`
	StartedAt    *time.Time        `json:"started_at,omitempty"`
	CompletedAt  *time.Time        `json:"completed_at,omitempty"`
}

// ActorRoll summarises one actor's involvement, so a reviewer can see at a
// glance who was present without reading every entry.
type ActorRoll struct {
	Actor     string    `json:"actor"`
	Agent     string    `json:"agent,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	Actions   int       `json:"actions"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// HasEvidence reports whether anything was offered as proof the work
// happened — a commit hash, a link, or an external issue reference. A task
// marked done with no evidence is the classic audit finding.
func (t *Trail) HasEvidence() bool {
	if t.Task == nil {
		return false
	}
	return len(t.Task.Evidence) > 0 || len(t.Task.ExternalRefs) > 0
}

// Unattributed reports how many entries carry no agent or session. Entries
// recorded before provenance capture existed will be unattributed, and a
// reviewer needs to know that rather than assume full coverage.
func (t *Trail) Unattributed() int {
	n := 0
	for _, e := range t.Entries {
		if e.Agent == "" && e.SessionID == "" {
			n++
		}
	}
	return n
}

// Findings lists the audit concerns a reviewer should see stated plainly,
// rather than having to infer them from the entries.
func (t *Trail) Findings() []string {
	var findings []string

	if t.Integrity.CheckedChain && !t.Integrity.Verified {
		findings = append(findings, "Audit chain verification FAILED — the event log has been altered or truncated.")
	}
	if len(t.Entries) == 0 {
		findings = append(findings, "No recorded activity for this subject.")
	}
	if t.Task != nil && isComplete(t.Task.Status) && !t.HasEvidence() {
		findings = append(findings, "Task is marked "+t.Task.Status+" but carries no evidence (no commit, link, or external reference).")
	}
	if n := t.Unattributed(); n > 0 {
		findings = append(findings,
			plural(n, "entry has", "entries have")+" no agent or session recorded; they predate provenance capture or came from an unidentified caller.")
	}

	return findings
}

func isComplete(status string) bool {
	return status == "done" || status == "verified"
}

// BuildActorRoll collapses entries into a per-actor summary, ordered by first
// appearance so the roll reads chronologically.
func BuildActorRoll(entries []Entry) []ActorRoll {
	index := map[string]*ActorRoll{}

	for _, e := range entries {
		key := e.Actor + "\x00" + e.Agent + "\x00" + e.SessionID
		roll, ok := index[key]
		if !ok {
			index[key] = &ActorRoll{
				Actor:     e.Actor,
				Agent:     e.Agent,
				SessionID: e.SessionID,
				Actions:   1,
				FirstSeen: e.At,
				LastSeen:  e.At,
			}
			continue
		}

		roll.Actions++
		if e.At.Before(roll.FirstSeen) {
			roll.FirstSeen = e.At
		}
		if e.At.After(roll.LastSeen) {
			roll.LastSeen = e.At
		}
	}

	rolls := make([]ActorRoll, 0, len(index))
	for _, r := range index {
		rolls = append(rolls, *r)
	}

	sort.Slice(rolls, func(i, j int) bool {
		if !rolls[i].FirstSeen.Equal(rolls[j].FirstSeen) {
			return rolls[i].FirstSeen.Before(rolls[j].FirstSeen)
		}
		return rolls[i].Actor < rolls[j].Actor
	})

	return rolls
}

// EntryFrom flattens a recorded event into a trail entry.
func EntryFrom(at time.Time, action, actor, hash string, metadata map[string]any) Entry {
	prov := provenance.FromMetadata(metadata)

	entry := Entry{
		At:        at,
		Action:    action,
		Actor:     actor,
		Agent:     prov.Agent,
		SessionID: prov.SessionID,
		EventHash: hash,
	}
	if prov.Surface != provenance.SurfaceUnknown {
		entry.Surface = string(prov.Surface)
	}
	if v, ok := metadata["task_id"].(string); ok {
		entry.TaskID = v
	}
	if v, ok := metadata["event"].(string); ok {
		entry.Detail = v
	}

	return entry
}
