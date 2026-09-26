package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
)

func setupRepo(t *testing.T) *FilesystemRepository {
	t.Helper()
	dir := t.TempDir()
	repo := NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return repo
}

// --- Root() ---

func TestRoot(t *testing.T) {
	dir := t.TempDir()
	repo := NewFilesystemRepository(dir)
	if repo.Root() != dir {
		t.Errorf("Root() = %q, want %q", repo.Root(), dir)
	}
}

// --- LoadUsage / UpdateUsage ---

// --- SaveWebhookConfig / LoadWebhookConfig ---

// --- SaveRates / LoadRates ---

// --- SaveTimeEntries / LoadTimeEntries ---

// --- SaveMessagingConfig / LoadMessagingConfig ---

// --- SaveTeam / LoadTeam ---

// --- LoadSpecLock not found ---

func TestLoadSpecLock_NotFound(t *testing.T) {
	repo := setupRepo(t)
	_, err := repo.LoadSpecLock()
	if err == nil {
		t.Error("expected error loading missing spec lock")
	}
}

// --- GetDependency not found ---

// --- LoadRange (event store) ---

func TestFileEventStore_LoadRange(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileEventStore(dir)
	if err != nil {
		t.Fatalf("NewFileEventStore: %v", err)
	}

	// Append events at different times
	now := time.Now()
	for i := 0; i < 5; i++ {
		evt := &events.BaseEvent{
			Type:      "test.event",
			Timestamp: now.Add(time.Duration(i) * time.Hour),
		}
		if err := store.Append(evt); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Load range: events 1-3 (hours 1-3)
	from := now.Add(1 * time.Hour)
	to := now.Add(3 * time.Hour)
	result, err := store.LoadRange(from, to)
	if err != nil {
		t.Fatalf("LoadRange: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("got %d events, want 3 (hours 1,2,3)", len(result))
	}
}

func TestFileEventStore_LoadRange_Empty(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileEventStore(dir)
	if err != nil {
		t.Fatalf("NewFileEventStore: %v", err)
	}

	from := time.Now()
	to := from.Add(time.Hour)
	result, err := store.LoadRange(from, to)
	if err != nil {
		t.Fatalf("LoadRange: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("got %d events, want 0", len(result))
	}
}

// --- ResolvePath edge cases ---

// --- File permission checks ---

func TestSaveCreatesFileWithRestrictedPermissions(t *testing.T) {
	repo := setupRepo(t)
	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}
	path, _ := repo.ResolvePath(PolicyFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}
}

func TestResolvePath_ValidFiles(t *testing.T) {
	repo := setupRepo(t)
	for _, f := range []string{SpecFile, SpecLockFile, PlanFile, StateFile, PolicyFile, EventsFile} {
		t.Run(f, func(t *testing.T) {
			path, err := repo.ResolvePath(f)
			if err != nil {
				t.Errorf("ResolvePath(%q) failed: %v", f, err)
			}
			if expected := filepath.Join(repo.root, RoadyDir, f); path != expected {
				t.Errorf("got %q, want %q", path, expected)
			}
		})
	}
}
