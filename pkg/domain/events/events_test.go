package events

import (
	"testing"
	"time"
)

func TestBaseEvent_CalculateHash(t *testing.T) {
	event := &BaseEvent{
		ID:             "evt-123",
		Type:           EventTypeTaskCompleted,
		AggregateID_:   "task-1",
		AggregateType_: AggregateTypeTask,
		Timestamp:      time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		Actor:          "alice",
		Metadata: map[string]interface{}{
			"task_id": "task-1",
		},
		PrevHash: "abc123",
	}

	hash := event.CalculateHash()
	if hash == "" {
		t.Error("Expected non-empty hash")
	}

	// Hash should be deterministic
	hash2 := event.CalculateHash()
	if hash != hash2 {
		t.Error("Hash should be deterministic")
	}

	// Changing data should change hash
	event.Actor = "bob"
	hash3 := event.CalculateHash()
	if hash == hash3 {
		t.Error("Changing data should change hash")
	}
}

func TestBaseEvent_CalculateHash_EmptyMetadata(t *testing.T) {
	event := &BaseEvent{
		ID:        "evt-123",
		Type:      EventTypeTaskStarted,
		Timestamp: time.Now(),
		Actor:     "alice",
	}

	hash := event.CalculateHash()
	if hash == "" {
		t.Error("Expected non-empty hash even with empty metadata")
	}
}

func TestCanonicalJSON(t *testing.T) {
	// Keys should be sorted
	m := map[string]interface{}{
		"zebra": 1,
		"alpha": 2,
		"beta":  3,
	}

	result := canonicalJSON(m)
	expected := `{"alpha":2,"beta":3,"zebra":1}`
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCanonicalJSON_Empty(t *testing.T) {
	result := canonicalJSON(nil)
	if result != "" {
		t.Errorf("Expected empty string, got %s", result)
	}

	result = canonicalJSON(map[string]interface{}{})
	if result != "" {
		t.Errorf("Expected empty string for empty map, got %s", result)
	}
}

func TestDomainEventInterface(t *testing.T) {
	now := time.Now()
	event := BaseEvent{
		ID:             "evt-123",
		Type:           EventTypeTaskCompleted,
		AggregateID_:   "task-1",
		AggregateType_: AggregateTypeTask,
		Timestamp:      now,
		Version_:       1,
	}

	// Verify interface implementation
	de := &event
	if de.EventType() != EventTypeTaskCompleted {
		t.Errorf("Expected %s, got %s", EventTypeTaskCompleted, de.EventType())
	}
	if de.AggregateID() != "task-1" {
		t.Errorf("Expected task-1, got %s", de.AggregateID())
	}
	if de.AggregateType() != AggregateTypeTask {
		t.Errorf("Expected %s, got %s", AggregateTypeTask, de.AggregateType())
	}
	if de.Version() != 1 {
		t.Errorf("Expected version 1, got %d", de.Version())
	}
}

func TestBaseEvent_Version(t *testing.T) {
	tests := []struct {
		name    string
		version interface{}
		want    int
	}{
		{"nil version", nil, 0},
		{"float64 version", float64(3), 3},
		{"int version", int(5), 5},
		{"string version", "1", 0},
		{"unknown type version", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &BaseEvent{Version_: tt.version}
			if got := event.Version(); got != tt.want {
				t.Errorf("Version() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBaseEvent_EnsureAction(t *testing.T) {
	t.Run("sets Action from Type when Action is empty", func(t *testing.T) {
		event := &BaseEvent{Type: EventTypeTaskCompleted, Action: ""}
		event.EnsureAction()
		if event.Action != EventTypeTaskCompleted {
			t.Errorf("expected Action = %s, got %s", EventTypeTaskCompleted, event.Action)
		}
	})

	t.Run("does not overwrite existing Action", func(t *testing.T) {
		event := &BaseEvent{Type: EventTypeTaskCompleted, Action: "custom_action"}
		event.EnsureAction()
		if event.Action != "custom_action" {
			t.Errorf("expected Action = custom_action, got %s", event.Action)
		}
	})
}
