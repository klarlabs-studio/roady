package watch

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestFSWatcher_DetectsFileWrite(t *testing.T) {
	dir := t.TempDir()

	// Create a file before starting the watcher
	testFile := filepath.Join(dir, "test.md")
	if err := os.WriteFile(testFile, []byte("initial"), 0600); err != nil {
		t.Fatal(err)
	}

	var eventCount atomic.Int32
	// lastChange is written from the watcher's callback goroutine and read here,
	// so it needs the same protection eventCount already gets from being atomic.
	var mu sync.Mutex
	var lastChange ChangeEvent

	w, err := NewFSWatcher(50*time.Millisecond, func(e ChangeEvent) {
		eventCount.Add(1)
		mu.Lock()
		lastChange = e
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := w.WatchRecursive(dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = w.Run(ctx)
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	if err := os.WriteFile(testFile, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce
	time.Sleep(200 * time.Millisecond)
	cancel()

	if eventCount.Load() == 0 {
		t.Error("expected at least one change event")
	}
	mu.Lock()
	gotType := lastChange.ChangeType
	mu.Unlock()
	if gotType == "" {
		t.Error("expected a non-empty change type")
	}
}

func TestFSWatcher_DetectsNewFile(t *testing.T) {
	dir := t.TempDir()

	var eventCount atomic.Int32

	w, err := NewFSWatcher(50*time.Millisecond, func(e ChangeEvent) {
		eventCount.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := w.WatchRecursive(dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = w.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	// Create a new file
	newFile := filepath.Join(dir, "new.md")
	if err := os.WriteFile(newFile, []byte("new content"), 0600); err != nil {
		t.Fatal(err)
	}

	time.Sleep(200 * time.Millisecond)
	cancel()

	if eventCount.Load() == 0 {
		t.Error("expected at least one change event for new file")
	}
}

func TestFSWatcher_ContextCancellation(t *testing.T) {
	dir := t.TempDir()

	w, err := NewFSWatcher(50*time.Millisecond, func(e ChangeEvent) {})
	if err != nil {
		t.Fatal(err)
	}

	if err := w.WatchRecursive(dir); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- w.Run(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("watcher did not stop after context cancellation")
	}
}

func TestFSWatcher_SetFilter(t *testing.T) {
	w, err := NewFSWatcher(50*time.Millisecond, func(e ChangeEvent) {})
	if err != nil {
		t.Fatal(err)
	}

	filter := NewPatternFilter([]string{"*.md"}, nil)
	w.SetFilter(filter)

	if w.filter == nil {
		t.Error("expected filter to be set")
	}
}

func TestOpToChangeType(t *testing.T) {
	tests := []struct {
		name     string
		op       fsnotify.Op
		expected string
	}{
		{"create", fsnotify.Create, "create"},
		{"write", fsnotify.Write, "write"},
		{"remove", fsnotify.Remove, "remove"},
		{"rename", fsnotify.Rename, "rename"},
		{"unknown", fsnotify.Op(0), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := opToChangeType(tt.op)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
