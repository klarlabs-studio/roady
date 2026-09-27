package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

func TestCLIError(t *testing.T) {
	t.Run("Error with cause", func(t *testing.T) {
		cause := errors.New("root cause")
		e := NewCLIError("something failed", "try this", cause)
		if e.Error() != "something failed: root cause" {
			t.Fatalf("unexpected: %s", e.Error())
		}
		if e.ExitCode != 1 {
			t.Fatalf("expected exit code 1, got %d", e.ExitCode)
		}
	})

	t.Run("Error without cause", func(t *testing.T) {
		e := NewCLIError("something failed", "try this", nil)
		if e.Error() != "something failed" {
			t.Fatalf("unexpected: %s", e.Error())
		}
	})

	t.Run("Unwrap returns cause", func(t *testing.T) {
		cause := errors.New("root")
		e := NewCLIError("msg", "", cause)
		if !errors.Is(e, cause) {
			t.Fatal("errors.Is should match wrapped cause")
		}
	})
}

func TestMapError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantHint string
		wantCLI  bool
	}{
		{
			name: "nil returns nil",
			err:  nil,
		},
		{
			name:     "ErrNoPlan",
			err:      project.ErrNoPlan,
			wantHint: "Run 'roady init <name>' to initialize a project",
			wantCLI:  true,
		},
		{
			name:     "ErrPlanNotApproved",
			err:      project.ErrPlanNotApproved,
			wantHint: "Run 'roady plan approve' first",
			wantCLI:  true,
		},
		{
			name:     "ErrTaskNotFound",
			err:      project.ErrTaskNotFound,
			wantHint: "Run 'roady plan show' to list available tasks",
			wantCLI:  true,
		},
		{
			name:     "ErrNoState",
			err:      project.ErrNoState,
			wantHint: "Run 'roady init' to initialize",
			wantCLI:  true,
		},
		{
			name:     "wrapped ErrNoPlan",
			err:      fmt.Errorf("failed: %w", project.ErrNoPlan),
			wantHint: "Run 'roady init <name>' to initialize a project",
			wantCLI:  true,
		},
		{
			name: "DependencyError",
			err: &project.DependencyError{
				TaskID:       "t1",
				DependencyID: "t0",
				Status:       "pending",
			},
			wantHint: "Complete task 't0' first, then retry",
			wantCLI:  true,
		},
		{
			name: "TransitionError",
			err: &project.TransitionError{
				TaskID:     "t1",
				FromStatus: "pending",
				ToStatus:   "done",
				Event:      "complete",
			},
			wantHint: "Task 't1' is 'pending' — check valid transitions with 'roady status'",
			wantCLI:  true,
		},
		{
			name: "unmapped error passes through",
			err:  errors.New("something else"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapError(tt.err)
			if tt.err == nil {
				if result != nil {
					t.Fatal("expected nil")
				}
				return
			}
			if !tt.wantCLI {
				if result != tt.err {
					t.Fatal("unmapped error should pass through unchanged")
				}
				return
			}
			var cliErr *CLIError
			if !errors.As(result, &cliErr) {
				t.Fatalf("expected CLIError, got %T", result)
			}
			if cliErr.Hint != tt.wantHint {
				t.Fatalf("hint = %q, want %q", cliErr.Hint, tt.wantHint)
			}
			// Verify original error is preserved
			if !errors.Is(cliErr, tt.err) {
				t.Fatal("CLIError should wrap original error")
			}
		})
	}
}
