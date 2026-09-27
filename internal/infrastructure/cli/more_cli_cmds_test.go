package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// ---------------------------------------------------------------------------
// Messaging command tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Doctor command tests
// ---------------------------------------------------------------------------

func TestDoctorCmd_Uninitialized(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	// Do NOT initialize the repo -- doctor should detect the missing .roady dir.
	err := doctorCmd.RunE(doctorCmd, []string{})
	if err == nil {
		t.Fatal("expected doctor to report issues for uninitialized directory")
	}
	if !strings.Contains(err.Error(), "doctor found issues") {
		t.Fatalf("expected 'doctor found issues' error, got: %v", err)
	}
}

func TestDoctorCmd_AllPass(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	_ = repo.SaveSpec(&spec.ProductSpec{ID: "spec-1", Title: "Project", Features: []spec.Feature{{ID: "f", Title: "F"}}})
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	state := planning.NewExecutionState("p1")
	state.ProjectID = "p1"
	_ = repo.SaveState(state)
	_ = repo.SavePolicy(&domain.PolicyConfig{})

	audit := application.NewAuditService(repo)
	if err := audit.Log("spec.update", "tester", nil); err != nil {
		t.Fatalf("log event: %v", err)
	}

	output := captureStdout(t, func() {
		if err := doctorCmd.RunE(doctorCmd, []string{}); err != nil {
			t.Fatalf("doctor failed: %v", err)
		}
	})

	if !strings.Contains(output, "Everything looks good") {
		t.Fatalf("expected all-pass output, got:\n%s", output)
	}
}

// ---------------------------------------------------------------------------
// Org command tests
// ---------------------------------------------------------------------------
