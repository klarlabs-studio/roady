package audit

import (
	"strings"
	"testing"
	"time"
)

func at(min int) time.Time {
	return time.Date(2026, 8, 1, 9, min, 0, 0, time.UTC)
}

func TestTrailFindings(t *testing.T) {
	tests := []struct {
		name         string
		trail        Trail
		wantContains []string
		wantAbsent   []string
	}{
		{
			name: "clean trail has no findings",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
				Entries:   []Entry{{At: at(1), Agent: "claude-code", SessionID: "s1"}},
				Task:      &TaskFacts{Status: "done", Evidence: []string{"abc123"}},
			},
		},
		{
			name: "failed chain is the first finding",
			trail: Trail{
				Integrity: Integrity{Verified: false, CheckedChain: true},
				Entries:   []Entry{{At: at(1), Agent: "a", SessionID: "s"}},
				Task:      &TaskFacts{Status: "done", Evidence: []string{"x"}},
			},
			wantContains: []string{"verification FAILED"},
		},
		{
			name: "done without evidence is flagged",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
				Entries:   []Entry{{At: at(1), Agent: "a", SessionID: "s"}},
				Task:      &TaskFacts{Status: "done"},
			},
			wantContains: []string{"no evidence"},
		},
		{
			name: "verified without evidence is also flagged",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
				Entries:   []Entry{{At: at(1), Agent: "a", SessionID: "s"}},
				Task:      &TaskFacts{Status: "verified"},
			},
			wantContains: []string{"no evidence"},
		},
		{
			name: "in-progress without evidence is not a finding",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
				Entries:   []Entry{{At: at(1), Agent: "a", SessionID: "s"}},
				Task:      &TaskFacts{Status: "in_progress"},
			},
			wantAbsent: []string{"no evidence"},
		},
		{
			name: "external refs count as evidence",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
				Entries:   []Entry{{At: at(1), Agent: "a", SessionID: "s"}},
				Task:      &TaskFacts{Status: "done", ExternalRefs: map[string]string{"jira": "ENG-1"}},
			},
			wantAbsent: []string{"no evidence"},
		},
		{
			name: "unattributed entries are surfaced",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
				Entries:   []Entry{{At: at(1)}, {At: at(2), Agent: "a"}},
				Task:      &TaskFacts{Status: "done", Evidence: []string{"x"}},
			},
			wantContains: []string{"1 entry has", "no agent or session"},
		},
		{
			name: "empty trail says so",
			trail: Trail{
				Integrity: Integrity{Verified: true, CheckedChain: true},
			},
			wantContains: []string{"No recorded activity"},
		},
		{
			name: "unchecked chain is not reported as failure",
			trail: Trail{
				Integrity: Integrity{CheckedChain: false},
				Entries:   []Entry{{At: at(1), Agent: "a", SessionID: "s"}},
			},
			wantAbsent: []string{"FAILED"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			joined := strings.Join(tt.trail.Findings(), " | ")

			if len(tt.wantContains) == 0 && len(tt.wantAbsent) == 0 && joined != "" {
				t.Errorf("expected no findings, got %q", joined)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(joined, want) {
					t.Errorf("findings %q should contain %q", joined, want)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(joined, absent) {
					t.Errorf("findings %q should not contain %q", joined, absent)
				}
			}
		})
	}
}

func TestBuildActorRoll(t *testing.T) {
	entries := []Entry{
		{At: at(3), Actor: "bob", Agent: "codex", SessionID: "s2"},
		{At: at(1), Actor: "alice", Agent: "claude-code", SessionID: "s1"},
		{At: at(5), Actor: "alice", Agent: "claude-code", SessionID: "s1"},
		{At: at(4), Actor: "alice", Agent: "claude-code", SessionID: "s3"},
	}

	rolls := BuildActorRoll(entries)

	if len(rolls) != 3 {
		t.Fatalf("expected 3 rolls (same actor in two sessions counts separately), got %d", len(rolls))
	}

	// Ordered by first appearance.
	if rolls[0].Actor != "alice" || rolls[0].SessionID != "s1" {
		t.Errorf("expected alice/s1 first, got %s/%s", rolls[0].Actor, rolls[0].SessionID)
	}
	if rolls[0].Actions != 2 {
		t.Errorf("alice/s1 Actions = %d, want 2", rolls[0].Actions)
	}
	if !rolls[0].FirstSeen.Equal(at(1)) || !rolls[0].LastSeen.Equal(at(5)) {
		t.Errorf("alice/s1 window = %v..%v, want %v..%v", rolls[0].FirstSeen, rolls[0].LastSeen, at(1), at(5))
	}
	if rolls[1].Actor != "bob" {
		t.Errorf("expected bob second, got %s", rolls[1].Actor)
	}
}

func TestBuildActorRollEmpty(t *testing.T) {
	if got := BuildActorRoll(nil); len(got) != 0 {
		t.Errorf("expected no rolls, got %d", len(got))
	}
}

func TestEntryFrom(t *testing.T) {
	metadata := map[string]any{
		"session_id": "s1",
		"agent":      "claude-code",
		"surface":    "mcp",
		"task_id":    "task-1",
		"event":      "complete",
	}

	got := EntryFrom(at(1), "task.transition", "alice", "hash123", metadata)

	if got.Agent != "claude-code" || got.SessionID != "s1" || got.Surface != "mcp" {
		t.Errorf("provenance not extracted: %+v", got)
	}
	if got.TaskID != "task-1" || got.Detail != "complete" {
		t.Errorf("task fields not extracted: %+v", got)
	}
	if got.EventHash != "hash123" {
		t.Errorf("EventHash = %q, want hash123", got.EventHash)
	}
}

func TestEntryFromUnattributed(t *testing.T) {
	got := EntryFrom(at(1), "task.started", "alice", "h", nil)

	if got.Agent != "" || got.SessionID != "" {
		t.Errorf("expected no provenance, got %+v", got)
	}
	// Surface must not read back as the literal "unknown" in a report table.
	if got.Surface != "" {
		t.Errorf("Surface = %q, want empty", got.Surface)
	}
}

func TestHasEvidence(t *testing.T) {
	tests := []struct {
		name  string
		trail Trail
		want  bool
	}{
		{name: "no task", trail: Trail{}, want: false},
		{name: "no evidence", trail: Trail{Task: &TaskFacts{}}, want: false},
		{name: "commit evidence", trail: Trail{Task: &TaskFacts{Evidence: []string{"abc"}}}, want: true},
		{name: "external ref", trail: Trail{Task: &TaskFacts{ExternalRefs: map[string]string{"jira": "E-1"}}}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.trail.HasEvidence(); got != tt.want {
				t.Errorf("HasEvidence() = %v, want %v", got, tt.want)
			}
		})
	}
}
