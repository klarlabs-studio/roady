package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// DefaultCheckTimeout bounds a check run. A check is meant to be a focused
// test, not a full CI pipeline.
const DefaultCheckTimeout = 10 * time.Minute

// ErrNoCheck is returned when a task has no acceptance check to run.
var ErrNoCheck = errors.New("task has no acceptance check")

// ManualCheckError is returned when a manual check has not been confirmed.
// A manual check is a person's judgement; nothing an agent does on its own
// satisfies it.
type ManualCheckError struct {
	TaskID      string
	Description string
}

func (e *ManualCheckError) Error() string {
	return fmt.Sprintf("task %s has a manual check that a person must confirm: %q (confirm with `roady task check %s --confirm`)",
		e.TaskID, e.Description, e.TaskID)
}

// CheckRunner executes a check command in dir and reports its exit code and
// combined output. err is for failures to run at all, not for a non-zero exit.
type CheckRunner func(ctx context.Context, dir, command string) (exitCode int, output []byte, err error)

// ShellCheckRunner runs the command with `sh -c`, the way a Makefile target or
// CI step would.
func ShellCheckRunner(ctx context.Context, dir, command string) (int, []byte, error) {
	// #nosec G204 -- the command comes from the project's own spec, with the
	// same trust as a Makefile target in the repository.
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	if err == nil {
		return 0, out.Bytes(), nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), out.Bytes(), nil
	}
	if ctx.Err() != nil {
		return -1, out.Bytes(), fmt.Errorf("check timed out: %w", ctx.Err())
	}
	return -1, out.Bytes(), err
}

// CheckOptions controls a single check run.
type CheckOptions struct {
	// ConfirmManual records a person's confirmation of a manual check.
	ConfirmManual bool
	Timeout       time.Duration
}

// SetCheckRunner replaces the command runner; tests use it to avoid a shell.
func (s *TaskService) SetCheckRunner(r CheckRunner) { s.checkRunner = r }

// RunCheck runs a task's acceptance check and records the result as evidence
// on the task, whether it passes or not. A failing check is a result, not an
// error: the error return is for being unable to run it.
func (s *TaskService) RunCheck(ctx context.Context, taskID, actor string, opts CheckOptions) (planning.CheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return planning.CheckResult{}, fmt.Errorf("load plan: %w", err)
	}
	if plan == nil {
		return planning.CheckResult{}, fmt.Errorf("no plan found")
	}
	task, ok := findTask(plan, taskID)
	if !ok {
		return planning.CheckResult{}, fmt.Errorf("task not found in plan")
	}
	if task.Check.IsZero() {
		return planning.CheckResult{}, fmt.Errorf("%w: %s (add a check to its requirement in spec.yaml and re-run `roady plan generate`)", ErrNoCheck, taskID)
	}
	if err := task.Check.Validate(); err != nil {
		return planning.CheckResult{}, fmt.Errorf("task %s: %w", taskID, err)
	}

	root := s.projectRoot()
	commit, dirty := gitHead(ctx, root)
	result := planning.CheckResult{
		Kind:   task.Check.Kind(),
		By:     actor,
		At:     time.Now(),
		Commit: commit,
		Dirty:  dirty,
	}

	switch result.Kind {
	case planning.CheckKindManual:
		result.Command = task.Check.Manual
		if !opts.ConfirmManual {
			return planning.CheckResult{}, &ManualCheckError{TaskID: taskID, Description: task.Check.Manual}
		}
		result.Passed = true
	default:
		result.Command = task.Check.Run
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = DefaultCheckTimeout
		}
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		runner := s.checkRunner
		if runner == nil {
			runner = ShellCheckRunner
		}
		start := time.Now()
		code, out, runErr := runner(runCtx, root, task.Check.Run)
		result.Duration = time.Since(start).Round(time.Millisecond).String()
		result.ExitCode = code
		result.Output = planning.TailOutput(out)
		if runErr != nil {
			if result.Output != "" {
				result.Output += "\n"
			}
			result.Output += runErr.Error()
		}
		result.Passed = runErr == nil && code == 0
	}

	if err := s.recordCheck(taskID, result); err != nil {
		return result, err
	}
	if err := s.audit.Log("task.check", actor, map[string]any{
		"task_id":   taskID,
		"kind":      result.Kind,
		"command":   result.Command,
		"passed":    result.Passed,
		"exit_code": result.ExitCode,
		"commit":    result.Commit,
		"dirty":     result.Dirty,
	}); err != nil {
		return result, fmt.Errorf("write audit log: %w", err)
	}
	return result, nil
}

func (s *TaskService) recordCheck(taskID string, result planning.CheckResult) error {
	state, err := s.repo.LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if state == nil {
		return fmt.Errorf("no execution state found")
	}
	if state.TaskStates == nil {
		state.TaskStates = map[string]planning.TaskResult{}
	}
	tr := state.TaskStates[taskID]
	if tr.Status == "" {
		tr.Status = planning.StatusPending
	}
	tr.Checks = append(tr.Checks, result)
	state.TaskStates[taskID] = tr
	if err := s.repo.SaveState(state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}

func (s *TaskService) projectRoot() string {
	if r, ok := s.repo.(rootedRepository); ok {
		return r.Root()
	}
	return "."
}

func findTask(plan *planning.Plan, id string) (planning.Task, bool) {
	for _, t := range plan.Tasks {
		if t.ID == id {
			return t, true
		}
	}
	return planning.Task{}, false
}

// gitHead returns the current commit and whether the tree has uncommitted
// changes. Both are best effort: outside a repository a check still runs.
func gitHead(ctx context.Context, dir string) (commit string, dirty bool) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", false
	}
	commit = strings.TrimSpace(string(out))
	// Roady's own files change whenever a task moves or a check is recorded,
	// so counting them would mark every run dirty and make the flag useless.
	status, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "--", ".", ":(exclude).roady").Output()
	if err == nil && len(bytes.TrimSpace(status)) > 0 {
		dirty = true
	}
	return commit, dirty
}

// CheckFailedError is returned when verification is refused because the
// task's acceptance check did not pass.
type CheckFailedError struct {
	TaskID string
	Result planning.CheckResult
}

func (e *CheckFailedError) Error() string {
	msg := fmt.Sprintf("cannot verify %s: its acceptance check failed (exit %d): %s", e.TaskID, e.Result.ExitCode, e.Result.Command)
	if out := strings.TrimSpace(e.Result.Output); out != "" {
		msg += "\n" + out
	}
	return msg
}

// ensureCheckPassed gates verification on a task's check, when it has one.
//
// A run check is executed now, against the code as it is, rather than trusting
// an earlier pass: verification is a claim about the current state. A manual
// check cannot be performed by the verifier, so it needs a confirmation already
// recorded by a person (`roady task check <id> --confirm`).
//
// A task without a check is not gated here; requiring one is a separate,
// policy-level decision.
func (s *TaskService) ensureCheckPassed(ctx context.Context, taskID, actor string) (*planning.CheckResult, error) {
	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return nil, nil // the coordinator reports a missing plan in its own terms
	}
	task, ok := findTask(plan, taskID)
	if !ok || task.Check.IsZero() {
		return nil, nil
	}
	// Leave an invalid transition (verifying a pending task, say) to the
	// coordinator's own error rather than running a check first.
	if state, err := s.repo.LoadState(); err == nil && state != nil {
		if !state.GetTaskStatus(taskID).CanTransitionWith("verify") {
			return nil, nil
		}
	}

	if task.Check.Kind() == planning.CheckKindManual {
		state, err := s.repo.LoadState()
		if err == nil && state != nil {
			if last, ok := state.TaskStates[taskID].LastCheck(); ok && last.Kind == planning.CheckKindManual && last.Passed {
				return &last, nil
			}
		}
		return nil, &ManualCheckError{TaskID: taskID, Description: task.Check.Manual}
	}

	result, err := s.RunCheck(ctx, taskID, actor, CheckOptions{})
	if err != nil {
		return nil, err
	}
	if !result.Passed {
		return nil, &CheckFailedError{TaskID: taskID, Result: result}
	}
	return &result, nil
}
