package report

import (
	"strings"
	"testing"
	"time"

	domainaudit "github.com/felixgeelhaar/roady/pkg/domain/audit"
)

func sampleTrail() *domainaudit.Trail {
	ts := time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC)
	started := ts
	return &domainaudit.Trail{
		Subject:     domainaudit.Subject{Kind: "task", ID: "task-42"},
		GeneratedAt: ts,
		Integrity:   domainaudit.Integrity{Verified: true, CheckedChain: true, EventsInLog: 12},
		Task: &domainaudit.TaskFacts{
			ID: "task-42", Title: "Implement signup", Status: "done", Owner: "alice",
			Origin: "ai", SourceDoc: "docs/spec.md", SourceLine: 88,
			Evidence:     []string{"commit abc123f"},
			ExternalRefs: map[string]string{"jira": "ENG-7 (https://x/ENG-7)"},
			StartedAt:    &started,
		},
		Entries: []domainaudit.Entry{
			{At: ts, Action: "task.started", Actor: "alice", Agent: "claude-code", SessionID: "9f3c1b2a-1111-2222-3333-444455556666"},
		},
		Actors: []domainaudit.ActorRoll{
			{Actor: "alice", Agent: "claude-code", SessionID: "9f3c1b2a-1111-2222-3333-444455556666", Actions: 2, FirstSeen: ts, LastSeen: ts},
		},
	}
}

func TestTrailMarkdownStructure(t *testing.T) {
	out := TrailMarkdown(sampleTrail())

	for _, want := range []string{
		"# Audit trail — task `task-42`",
		"**Verified.**",
		"hash chain over 12 recorded events",
		"| Traces to | docs/spec.md:88 |",
		"commit abc123f",
		"jira: ENG-7",
		"claude-code",
		"task.started",
		"What this document attests",
		"not proof of identity",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("trail markdown missing %q", want)
		}
	}
}

func TestTrailMarkdownLeadsWithIntegrity(t *testing.T) {
	trail := sampleTrail()
	trail.Integrity = domainaudit.Integrity{
		CheckedChain: true,
		Verified:     false,
		Problems:     []string{"Event 3: Content hash mismatch."},
	}

	out := TrailMarkdown(trail)

	// A reviewer must not have to read to the bottom to learn the log failed.
	integrityAt := strings.Index(out, "**FAILED.**")
	entriesAt := strings.Index(out, "## Recorded events")
	if integrityAt < 0 {
		t.Fatalf("failure not reported:\n%s", out)
	}
	if integrityAt > entriesAt {
		t.Error("chain failure must appear before the event list")
	}
	if !strings.Contains(out, "Content hash mismatch") {
		t.Error("expected the specific problem to be named")
	}
}

func TestTrailMarkdownAlwaysCarriesAttestationLimit(t *testing.T) {
	// The limit must travel with every trail, including a clean one — a
	// document that looks like an attestation gets read as one.
	for _, trail := range []*domainaudit.Trail{
		sampleTrail(),
		{Subject: domainaudit.Subject{Kind: "agent", ID: "codex"}, GeneratedAt: time.Now()},
	} {
		out := TrailMarkdown(trail)
		if !strings.Contains(out, "not proof of identity") {
			t.Errorf("attestation limit missing from trail for %s", trail.Subject.ID)
		}
	}
}

func TestTrailMarkdownEmptySubject(t *testing.T) {
	out := TrailMarkdown(&domainaudit.Trail{
		Subject:     domainaudit.Subject{Kind: "agent", ID: "nobody"},
		GeneratedAt: time.Now(),
		Integrity:   domainaudit.Integrity{CheckedChain: true, Verified: true},
	})

	if !strings.Contains(out, "No recorded activity") {
		t.Errorf("expected an explicit empty finding:\n%s", out)
	}
	if !strings.Contains(out, "None.") {
		t.Error("expected the event table to say None")
	}
}

func TestShortSession(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "", want: ""},
		{in: "sess-aaa", want: "sess-aaa"},
		{in: "9f3c1b2a-1111-2222", want: "9f3c1b2a-111…"},
	}

	for _, tt := range tests {
		if got := shortSession(tt.in); got != tt.want {
			t.Errorf("shortSession(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
