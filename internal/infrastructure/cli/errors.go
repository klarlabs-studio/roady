package cli

import (
	"errors"
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

// CLIError wraps domain errors with user-facing messages and actionable hints.
type CLIError struct {
	Message  string
	Hint     string
	Err      error
	ExitCode int
}

func (e *CLIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *CLIError) Unwrap() error {
	return e.Err
}

// NewCLIError creates a CLIError with a default exit code of 1.
func NewCLIError(msg, hint string, err error) *CLIError {
	return &CLIError{
		Message:  msg,
		Hint:     hint,
		Err:      err,
		ExitCode: 1,
	}
}

// MapError converts known domain errors into CLIErrors with actionable hints.
// Unmapped errors are returned as-is.
func MapError(err error) error {
	if err == nil {
		return nil
	}

	var depErr *project.DependencyError
	if errors.As(err, &depErr) {
		return NewCLIError(
			depErr.Error(),
			fmt.Sprintf("Complete task '%s' first, then retry", depErr.DependencyID),
			err,
		)
	}

	var transErr *project.TransitionError
	if errors.As(err, &transErr) {
		return NewCLIError(
			transErr.Error(),
			fmt.Sprintf("Task '%s' is '%s' — check valid transitions with 'roady status'", transErr.TaskID, transErr.FromStatus),
			err,
		)
	}

	switch {
	case errors.Is(err, project.ErrNoPlan):
		return NewCLIError("no plan found", "Run 'roady init <name>' to initialize a project", err)
	case errors.Is(err, project.ErrPlanNotApproved):
		return NewCLIError("plan is not approved", "Run 'roady plan approve' first", err)
	case errors.Is(err, project.ErrTaskNotFound):
		return NewCLIError("task not found", "Run 'roady plan show' to list available tasks", err)
	case errors.Is(err, project.ErrNoState):
		return NewCLIError("no execution state found", "Run 'roady init' to initialize", err)
	}

	return err
}
