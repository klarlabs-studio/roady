package storage_test

import (
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// plan.json carries an updated_at that drift reads to decide whether the plan
// has been left behind. Every writer had to remember to set it by hand, and
// approving a plan did not — so a plan could be rewritten while its timestamp
// stayed months old, and drift then reported it stale at critical severity.
// Stamping at the single write funnel makes that impossible to forget.
// See issue #76.
func TestSavePlanStampsUpdatedAt(t *testing.T) {
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}

	stale := time.Now().Add(-84 * 24 * time.Hour)
	plan := &planning.Plan{
		ID:        "plan-1",
		SpecID:    "spec-1",
		CreatedAt: stale,
		UpdatedAt: stale,
		Tasks:     []planning.Task{{ID: "task-1", Title: "One"}},
	}

	before := time.Now()
	if err := repo.SavePlan(plan); err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt = %s, want a stamp from this write (>= %s)", loaded.UpdatedAt, before)
	}
	// CreatedAt is history and must survive untouched.
	if !loaded.CreatedAt.Equal(stale) {
		t.Errorf("CreatedAt = %s, want the original %s", loaded.CreatedAt, stale)
	}
}

// The stamp reflects the write that happened, so a caller that already set the
// field does not end up with an older value than the file it just produced.
func TestSavePlanStampIsNotOlderThanTheWrite(t *testing.T) {
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}

	plan := &planning.Plan{ID: "p", SpecID: "s", Tasks: []planning.Task{{ID: "t", Title: "T"}}}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	first, err := repo.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	plan.Tasks = append(plan.Tasks, planning.Task{ID: "t2", Title: "T2"})
	if err := repo.SavePlan(plan); err != nil {
		t.Fatal(err)
	}
	second, err := repo.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}

	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("second write did not advance UpdatedAt: %s then %s", first.UpdatedAt, second.UpdatedAt)
	}
}
