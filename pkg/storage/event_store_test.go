package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/events"
)

func TestFileEventStore_AppendAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Append events
	event1 := &events.BaseEvent{
		Type:           events.EventTypeTaskStarted,
		AggregateID_:   "task-1",
		AggregateType_: events.AggregateTypeTask,
		Timestamp:      time.Now(),
		Actor:          "alice",
		Metadata: map[string]interface{}{
			"task_id": "task-1",
		},
	}
	if err := store.Append(event1); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	event2 := &events.BaseEvent{
		Type:           events.EventTypeTaskCompleted,
		AggregateID_:   "task-1",
		AggregateType_: events.AggregateTypeTask,
		Timestamp:      time.Now(),
		Actor:          "alice",
		Metadata: map[string]interface{}{
			"task_id": "task-1",
		},
	}
	if err := store.Append(event2); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Load all events
	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("Expected 2 events, got %d", len(loaded))
	}

	// Verify hash chaining
	if loaded[0].PrevHash != "" {
		t.Error("First event should have empty PrevHash")
	}
	if loaded[1].PrevHash != loaded[0].Hash {
		t.Error("Second event's PrevHash should match first event's Hash")
	}
}

func TestFileEventStore_HashChainIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Append events
	for i := 0; i < 5; i++ {
		event := &events.BaseEvent{
			Type:      events.EventTypeTaskStarted,
			Timestamp: time.Now(),
			Actor:     "test",
			Metadata:  map[string]interface{}{"index": i},
		}
		if err := store.Append(event); err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	}

	// Verify integrity
	violations, err := store.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("Expected no violations, got: %v", violations)
	}
}

func TestFileEventStore_LoadByAggregate(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Append events for different aggregates
	if err := store.Append(&events.BaseEvent{
		Type:           events.EventTypeTaskStarted,
		AggregateID_:   "task-1",
		AggregateType_: events.AggregateTypeTask,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(&events.BaseEvent{
		Type:           events.EventTypeTaskStarted,
		AggregateID_:   "task-2",
		AggregateType_: events.AggregateTypeTask,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(&events.BaseEvent{
		Type:           events.EventTypePlanCreated,
		AggregateID_:   "plan-1",
		AggregateType_: events.AggregateTypePlan,
	}); err != nil {
		t.Fatal(err)
	}

	// Load by aggregate
	taskEvents, err := store.LoadByAggregate(events.AggregateTypeTask, "task-1")
	if err != nil {
		t.Fatalf("LoadByAggregate failed: %v", err)
	}
	if len(taskEvents) != 1 {
		t.Errorf("Expected 1 event for task-1, got %d", len(taskEvents))
	}
}

func TestFileEventStore_LoadByType(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	if err := store.Append(&events.BaseEvent{Type: events.EventTypeTaskStarted}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(&events.BaseEvent{Type: events.EventTypeTaskCompleted}); err != nil {
		t.Fatal(err)
	}
	if err := store.Append(&events.BaseEvent{Type: events.EventTypeTaskStarted}); err != nil {
		t.Fatal(err)
	}

	started, err := store.LoadByType(events.EventTypeTaskStarted)
	if err != nil {
		t.Fatalf("LoadByType failed: %v", err)
	}
	if len(started) != 2 {
		t.Errorf("Expected 2 started events, got %d", len(started))
	}
}

func TestFileEventStore_LoadSince(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Add old event
	oldEvent := &events.BaseEvent{
		Type:      events.EventTypeTaskStarted,
		Timestamp: time.Now().Add(-48 * time.Hour),
	}
	_ = store.Append(oldEvent)

	// Add recent event
	newEvent := &events.BaseEvent{
		Type:      events.EventTypeTaskCompleted,
		Timestamp: time.Now(),
	}
	_ = store.Append(newEvent)

	// Load since 24 hours ago
	recent, err := store.LoadSince(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("LoadSince failed: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("Expected 1 recent event, got %d", len(recent))
	}
}

func TestFileEventStore_GetLastEvent(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Empty store
	last, err := store.GetLastEvent()
	if err != nil {
		t.Fatalf("GetLastEvent failed: %v", err)
	}
	if last != nil {
		t.Error("Expected nil for empty store")
	}

	// Add events
	_ = store.Append(&events.BaseEvent{Type: events.EventTypeTaskStarted, Metadata: map[string]interface{}{"index": 1}})
	_ = store.Append(&events.BaseEvent{Type: events.EventTypeTaskCompleted, Metadata: map[string]interface{}{"index": 2}})

	last, err = store.GetLastEvent()
	if err != nil {
		t.Fatalf("GetLastEvent failed: %v", err)
	}
	if last == nil {
		t.Fatal("Expected non-nil last event")
	}
	if last.Type != events.EventTypeTaskCompleted {
		t.Errorf("Expected last event to be TaskCompleted, got %s", last.Type)
	}
}

func TestFileEventStore_Count(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	count, _ := store.Count()
	if count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}

	_ = store.Append(&events.BaseEvent{Type: events.EventTypeTaskStarted})
	_ = store.Append(&events.BaseEvent{Type: events.EventTypeTaskCompleted})

	count, _ = store.Count()
	if count != 2 {
		t.Errorf("Expected count 2, got %d", count)
	}
}

func TestFileEventStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create store and add events
	store1, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}
	if err := store1.Append(&events.BaseEvent{Type: events.EventTypeTaskStarted, Actor: "alice"}); err != nil {
		t.Fatal(err)
	}
	if err := store1.Append(&events.BaseEvent{Type: events.EventTypeTaskCompleted, Actor: "alice"}); err != nil {
		t.Fatal(err)
	}

	// Create new store instance from same path
	store2, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	loaded, err := store2.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("Expected 2 events from persisted store, got %d", len(loaded))
	}
}

func TestFileEventStore_NonExistentPath(t *testing.T) {
	tmpDir := t.TempDir()
	newPath := filepath.Join(tmpDir, "nested", "path", "events")

	store, err := NewFileEventStore(newPath)
	if err != nil {
		t.Fatalf("NewFileEventStore should create nested path: %v", err)
	}

	err = store.Append(&events.BaseEvent{Type: events.EventTypeTaskStarted})
	if err != nil {
		t.Fatalf("Append should work with new path: %v", err)
	}
}

func TestFileEventStore_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create empty events file
	eventsFile := filepath.Join(tmpDir, "events.jsonl")
	if err := os.WriteFile(eventsFile, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	store, err := NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	loaded, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("Expected 0 events from empty file, got %d", len(loaded))
	}
}
