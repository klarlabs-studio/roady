package application_test

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// The realistic adoption path: `roady init --template x` then replace the
// generated spec with one describing the actual project. The lock and the
// execution state keep the template's identity, and nothing said so — while
// `spec validate` reported success. See issue #77.
func adoptedProject(t *testing.T) *storage.FilesystemRepository {
	t.Helper()
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	template := &spec.ProductSpec{ID: "new-project", Title: "new-project", Version: "0.1.0"}
	if err := repo.SaveSpec(template); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSpecLock(template); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveState(planning.NewExecutionState("new-project")); err != nil {
		t.Fatal(err)
	}
	// The adopter replaces the spec.
	real := &spec.ProductSpec{ID: "warden", Title: "warden", Version: "0.1.0"}
	if err := repo.SaveSpec(real); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestLockStatusReportsDerivedStateThatDisagrees(t *testing.T) {
	svc := application.NewSpecService(adoptedProject(t))

	status, err := svc.LockStatus()
	if err != nil {
		t.Fatal(err)
	}

	if status.InSync() {
		t.Fatal("derived state disagrees with the spec but reported in sync")
	}
	problems := strings.Join(status.Problems(), "\n")
	if !strings.Contains(problems, "spec.lock.json") {
		t.Errorf("the stale lock was not reported: %q", problems)
	}
	if !strings.Contains(problems, "state.json") {
		t.Errorf("the stale state project id was not reported: %q", problems)
	}
	// Naming the remedy matters: the reporter regenerated the lock by hand
	// with a Python one-liner because nothing offered to.
	if !strings.Contains(problems, "roady spec lock") {
		t.Errorf("no remedy named: %q", problems)
	}
}

func TestLockStatusCleanWhenDerivedStateAgrees(t *testing.T) {
	repo := adoptedProject(t)
	if _, err := application.NewSpecService(repo).WriteLock(); err != nil {
		t.Fatal(err)
	}

	status, err := application.NewSpecService(repo).LockStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.InSync() {
		t.Errorf("still out of sync after WriteLock: %v", status.Problems())
	}
}

// WriteLock re-derives both files from the spec and says what it changed.
func TestWriteLockReconcilesLockAndState(t *testing.T) {
	repo := adoptedProject(t)

	result, err := application.NewSpecService(repo).WriteLock()
	if err != nil {
		t.Fatalf("WriteLock: %v", err)
	}

	locked, err := repo.LoadSpecLock()
	if err != nil {
		t.Fatal(err)
	}
	if locked.ID != "warden" {
		t.Errorf("lock id = %q, want warden", locked.ID)
	}
	state, err := repo.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.ProjectID != "warden" {
		t.Errorf("state project_id = %q, want warden", state.ProjectID)
	}
	if !result.LockUpdated || !result.StateUpdated {
		t.Errorf("result did not report both updates: %+v", result)
	}
}

// Re-running must not churn: a no-op has to say it did nothing, so the command
// is safe in a script.
func TestWriteLockIsIdempotent(t *testing.T) {
	repo := adoptedProject(t)
	svc := application.NewSpecService(repo)
	if _, err := svc.WriteLock(); err != nil {
		t.Fatal(err)
	}

	second, err := svc.WriteLock()
	if err != nil {
		t.Fatal(err)
	}
	if second.LockUpdated || second.StateUpdated {
		t.Errorf("second WriteLock reported changes: %+v", second)
	}
}
