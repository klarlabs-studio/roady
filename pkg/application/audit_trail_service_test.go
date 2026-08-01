package application

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
)

func TestMatchesQuery(t *testing.T) {
	claudeMeta := map[string]any{
		"task_id":               "task-1",
		provenance.KeyAgent:     "claude-code",
		provenance.KeySessionID: "sess-a",
	}
	codexMeta := map[string]any{
		"task_id":               "task-2",
		provenance.KeyAgent:     "codex",
		provenance.KeySessionID: "sess-b",
	}

	tests := []struct {
		name        string
		metadata    map[string]any
		aggregateID string
		query       TrailQuery
		want        bool
	}{
		{name: "empty query matches everything", metadata: claudeMeta, want: true},
		{name: "task id matches", metadata: claudeMeta, query: TrailQuery{TaskID: "task-1"}, want: true},
		{name: "task id case-insensitive", metadata: claudeMeta, query: TrailQuery{TaskID: "TASK-1"}, want: true},
		{name: "task id mismatch", metadata: claudeMeta, query: TrailQuery{TaskID: "task-9"}, want: false},
		{
			name:        "falls back to aggregate id when metadata lacks task_id",
			metadata:    map[string]any{},
			aggregateID: "task-1",
			query:       TrailQuery{TaskID: "task-1"},
			want:        true,
		},
		{name: "agent matches", metadata: codexMeta, query: TrailQuery{Agent: "codex"}, want: true},
		{name: "agent mismatch", metadata: codexMeta, query: TrailQuery{Agent: "claude-code"}, want: false},
		{name: "session matches", metadata: codexMeta, query: TrailQuery{SessionID: "sess-b"}, want: true},
		{name: "session mismatch", metadata: codexMeta, query: TrailQuery{SessionID: "sess-a"}, want: false},
		{
			name:     "task and agent compose",
			metadata: claudeMeta,
			query:    TrailQuery{TaskID: "task-1", Agent: "claude-code"},
			want:     true,
		},
		{
			name:     "task matches but agent does not",
			metadata: claudeMeta,
			query:    TrailQuery{TaskID: "task-1", Agent: "codex"},
			want:     false,
		},
		{
			name:     "unattributed event excluded by an agent filter",
			metadata: map[string]any{"task_id": "task-1"},
			query:    TrailQuery{Agent: "claude-code"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesQuery(tt.metadata, tt.aggregateID, tt.query); got != tt.want {
				t.Errorf("matchesQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventAction(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		action    string
		want      string
	}{
		{name: "prefers type", eventType: "task.completed", action: "ignored", want: "task.completed"},
		{name: "falls back to action", eventType: "", action: "task.transition", want: "task.transition"},
		{name: "whitespace type falls back", eventType: "  ", action: "task.start", want: "task.start"},
		{name: "both empty", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventAction(tt.eventType, tt.action); got != tt.want {
				t.Errorf("eventAction(%q, %q) = %q, want %q", tt.eventType, tt.action, got, tt.want)
			}
		})
	}
}

func TestSubjectFor(t *testing.T) {
	tests := []struct {
		name     string
		query    TrailQuery
		wantKind string
		wantID   string
	}{
		{name: "task", query: TrailQuery{TaskID: "t1"}, wantKind: "task", wantID: "t1"},
		{name: "session", query: TrailQuery{SessionID: "s1"}, wantKind: "session", wantID: "s1"},
		{name: "agent", query: TrailQuery{Agent: "codex"}, wantKind: "agent", wantID: "codex"},
		{name: "task wins over agent", query: TrailQuery{TaskID: "t1", Agent: "codex"}, wantKind: "task", wantID: "t1"},
		{name: "empty", query: TrailQuery{}, wantKind: "project", wantID: "all"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subjectFor(tt.query)
			if got.Kind != tt.wantKind || got.ID != tt.wantID {
				t.Errorf("subjectFor() = %s/%s, want %s/%s", got.Kind, got.ID, tt.wantKind, tt.wantID)
			}
		})
	}
}
