package application

import (
	"context"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestEventSourcedAuditService_Log(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	svc, err := NewEventSourcedAuditService(store, publisher)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Log an event
	err = svc.Log("task.started", "alice", map[string]interface{}{
		"task_id": "task-1",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify event was stored
	evts, err := svc.LoadEvents()
	if err != nil {
		t.Fatalf("LoadEvents failed: %v", err)
	}
	if len(evts) != 1 {
		t.Errorf("Expected 1 event, got %d", len(evts))
	}
	if evts[0].Type != "task.started" {
		t.Errorf("Expected task.started, got %s", evts[0].Type)
	}
	if evts[0].Actor != "alice" {
		t.Errorf("Expected alice, got %s", evts[0].Actor)
	}
}

func TestEventSourcedAuditService_Projections(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	svc, err := NewEventSourcedAuditService(store, publisher)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Log task events
	_ = svc.Log(events.EventTypeTaskStarted, "alice", map[string]interface{}{
		"task_id": "task-1",
	})
	_ = svc.Log(events.EventTypeTaskCompleted, "alice", map[string]interface{}{
		"task_id": "task-1",
	})

	// Check timeline projection
	timeline := svc.GetTimeline()
	if len(timeline) != 2 {
		t.Errorf("Expected 2 timeline entries, got %d", len(timeline))
	}

	// Check recent timeline
	recent := svc.GetRecentTimeline(1)
	if len(recent) != 1 {
		t.Errorf("Expected 1 recent entry, got %d", len(recent))
	}
}

func TestEventSourcedAuditService_TaskState(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	svc, err := NewEventSourcedAuditService(store, publisher)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Log task started
	_ = svc.Log(events.EventTypeTaskStarted, "alice", map[string]interface{}{
		"task_id": "task-1",
	})

	// Check task state
	state := svc.GetTaskState("task-1")
	if state == nil {
		t.Fatal("Expected task state")
	}
	if state.Owner != "alice" {
		t.Errorf("Expected owner alice, got %s", state.Owner)
	}

	// Get all states
	states := svc.GetAllTaskStates()
	if len(states) != 1 {
		t.Errorf("Expected 1 state, got %d", len(states))
	}
}

func TestEventSourcedAuditService_Velocity(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	svc, err := NewEventSourcedAuditService(store, publisher)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Verify starts at zero
	velocity := svc.GetCompletionVelocity()
	if velocity != 0 {
		t.Errorf("Expected 0 velocity, got %f", velocity)
	}

	// Log completions
	for i := 0; i < 7; i++ {
		_ = svc.Log(events.EventTypeTaskCompleted, "alice", map[string]interface{}{
			"task_id": "task-" + string(rune('0'+i)),
		})
	}

	// Velocity should now be ~1 per day
	velocity = svc.GetCompletionVelocity()
	if velocity < 0.9 || velocity > 1.1 {
		t.Errorf("Expected velocity around 1.0, got %f", velocity)
	}
}

func TestEventSourcedAuditService_VerifyIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	svc, err := NewEventSourcedAuditService(store, nil)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Log events
	_ = svc.Log("task.started", "alice", nil)
	_ = svc.Log("task.completed", "alice", nil)

	// Verify integrity
	violations, err := svc.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if len(violations) != 0 {
		t.Errorf("Expected no violations, got: %v", violations)
	}
}

func TestEventSourcedAuditService_LoadEventsSince(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	svc, err := NewEventSourcedAuditService(store, nil)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Record time before logging
	before := time.Now().Add(-time.Second)

	_ = svc.Log("task.started", "alice", nil)
	_ = svc.Log("task.completed", "alice", nil)

	// Load since before
	evts, err := svc.LoadEventsSince(before)
	if err != nil {
		t.Fatalf("LoadEventsSince failed: %v", err)
	}
	if len(evts) != 2 {
		t.Errorf("Expected 2 events, got %d", len(evts))
	}

	// Load since future
	future := time.Now().Add(time.Hour)
	evts, err = svc.LoadEventsSince(future)
	if err != nil {
		t.Fatalf("LoadEventsSince failed: %v", err)
	}
	if len(evts) != 0 {
		t.Errorf("Expected 0 events, got %d", len(evts))
	}
}

func TestEventSourcedAuditService_RebuildFromExisting(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Pre-populate events directly in store
	_ = store.Append(&events.BaseEvent{
		Type:  events.EventTypeTaskStarted,
		Actor: "bob",
		Metadata: map[string]interface{}{
			"task_id": "task-1",
		},
	})

	// Create service - should rebuild projections
	svc, err := NewEventSourcedAuditService(store, nil)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Check projection was rebuilt
	state := svc.GetTaskState("task-1")
	if state == nil {
		t.Fatal("Expected task state from rebuilt projection")
	}
	if state.Owner != "bob" {
		t.Errorf("Expected owner bob, got %s", state.Owner)
	}
}

func TestEventSourcedAuditService_NilPublisher(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	// Create with nil publisher - should not panic
	svc, err := NewEventSourcedAuditService(store, nil)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Log should still work (just no publish)
	err = svc.Log("test.action", "alice", nil)
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	evts, _ := svc.LoadEvents()
	if len(evts) != 1 {
		t.Errorf("Expected 1 event, got %d", len(evts))
	}
}

func TestEventSourcedAuditService_SetDispatcher(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	svc, err := NewEventSourcedAuditService(store, nil)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Initially no dispatcher
	if svc.GetDispatcher() != nil {
		t.Error("Expected nil dispatcher initially")
	}

	// Set dispatcher
	dispatcher := events.NewEventDispatcher()
	svc.SetDispatcher(dispatcher)

	if svc.GetDispatcher() == nil {
		t.Error("Expected dispatcher to be set")
	}
	if svc.GetDispatcher() != dispatcher {
		t.Error("Expected same dispatcher instance")
	}
}

func TestEventSourcedAuditService_RegisterHandler(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	svc, err := NewEventSourcedAuditService(store, nil)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Register handler creates dispatcher if nil
	handlerCalled := make(chan bool, 1)
	svc.RegisterHandler(events.HandlerRegistration{
		Name: "test-handler",
		Handler: func(ctx context.Context, event events.DomainEvent) error {
			handlerCalled <- true
			return nil
		},
		EventTypes: []string{"task.started"},
	})

	// Dispatcher should be created
	if svc.GetDispatcher() == nil {
		t.Error("Expected dispatcher to be created")
	}

	// Log an event
	err = svc.Log("task.started", "alice", map[string]interface{}{
		"task_id": "task-1",
	})
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Wait for async dispatch with timeout
	select {
	case <-handlerCalled:
		// Handler was called
	case <-time.After(time.Second):
		t.Error("Handler was not called within timeout")
	}
}

func TestEventSourcedAuditService_DispatcherIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	svc, err := NewEventSourcedAuditService(store, publisher)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService failed: %v", err)
	}

	// Track dispatched events
	dispatched := make(chan string, 10)
	dispatcher := events.NewEventDispatcher()
	dispatcher.RegisterWildcard("tracker", func(ctx context.Context, event events.DomainEvent) error {
		dispatched <- event.EventType()
		return nil
	})
	svc.SetDispatcher(dispatcher)

	// Log multiple events
	_ = svc.Log(events.EventTypeTaskStarted, "alice", map[string]interface{}{"task_id": "task-1"})
	_ = svc.Log(events.EventTypeTaskCompleted, "alice", map[string]interface{}{"task_id": "task-1"})
	_ = svc.Log(events.EventTypeTaskVerified, "alice", map[string]interface{}{"task_id": "task-1"})

	// Wait for dispatches with timeout
	received := make([]string, 0, 3)
	timeout := time.After(2 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case evt := <-dispatched:
			received = append(received, evt)
		case <-timeout:
			t.Fatalf("Timeout waiting for dispatches, received: %v", received)
		}
	}

	if len(received) != 3 {
		t.Errorf("Expected 3 dispatched events, got %d", len(received))
	}
}
