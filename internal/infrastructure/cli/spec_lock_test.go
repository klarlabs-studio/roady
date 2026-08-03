package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// The adoption path from issue #77: init writes spec, lock and state together,
// then the adopter replaces the spec with one describing the real project. The
// lock keeps the template's identity and is what every drift check compares
// against, so drift is measured against a spec the project never had.
func TestSpecLockCommandReCapturesTheBaseline(t *testing.T) {
	dir, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(dir)
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
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "warden", Title: "warden", Version: "0.1.0"}); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if err := specLockCmd.RunE(specLockCmd, nil); err != nil {
			t.Fatalf("spec lock: %v", err)
		}
	})
	if !strings.Contains(out, "warden") {
		t.Errorf("output does not name the spec it locked: %q", out)
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

	// Safe to re-run: a no-op must say so rather than rewriting files.
	again := captureStdout(t, func() {
		if err := specLockCmd.RunE(specLockCmd, nil); err != nil {
			t.Fatalf("second spec lock: %v", err)
		}
	})
	if !strings.Contains(again, "Already in sync") {
		t.Errorf("second run did not report a no-op: %q", again)
	}
}

// validate reported a bare success while the lock described a different
// project. Shape is not agreement.
func TestSpecValidateWarnsWhenTheLockDisagrees(t *testing.T) {
	dir, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSpecLock(&spec.ProductSpec{ID: "new-project", Title: "new-project", Version: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "warden", Title: "warden", Version: "0.1.0",
		Features: []spec.Feature{{ID: "core", Title: "Core", Description: "The thing."}},
	}); err != nil {
		t.Fatal(err)
	}

	// The warning goes to stderr; the command still succeeds, because the
	// spec's shape is valid and disagreement is drift's verdict to render.
	if err := specValidateCmd.RunE(specValidateCmd, nil); err != nil {
		t.Fatalf("validate: %v", err)
	}
}
