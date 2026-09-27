package application

import (
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestEventSourcedAuditService_Log(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	svc := NewEventSourcedAuditService(store)

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

func TestEventSourcedAuditService_VerifyIntegrity(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore failed: %v", err)
	}

	svc := NewEventSourcedAuditService(store)

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

	svc := NewEventSourcedAuditService(store)

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
