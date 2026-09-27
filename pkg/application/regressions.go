package application

import (
	"context"
	"fmt"
	"sort"

	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// Verified means proven when it was verified. Code keeps changing, so a
// proof can go stale: RecheckVerified re-runs the acceptance check of every
// verified task against the code as it is now and reports each one that no
// longer passes as regression drift. Every run is recorded in the task's
// check history, pass or fail, like any other check.

// RecheckResult is one verified task's check, run again.
type RecheckResult struct {
	TaskID string               `json:"task_id"`
	Result planning.CheckResult `json:"result"`
	// LastPass is the most recent passing run before this one, if any: the
	// commit the task was last known good at.
	LastPass *planning.CheckResult `json:"last_pass,omitempty"`
}

// RecheckVerified re-runs the run-checks of verified tasks and returns the
// regressions as drift issues, plus every run. Manual checks are skipped:
// only a person can confirm them.
func (s *TaskService) RecheckVerified(ctx context.Context, actor string, opts CheckOptions) ([]drift.Issue, []RecheckResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return nil, nil, err
	}
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return nil, nil, err
	}
	var ids []string
	for _, t := range plan.Tasks {
		if state.GetTaskStatus(t.ID) == planning.StatusVerified && !t.Check.IsZero() && t.Check.Kind() == planning.CheckKindRun {
			ids = append(ids, t.ID)
		}
	}
	sort.Strings(ids)

	var issues []drift.Issue
	var runs []RecheckResult
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return issues, runs, err
		}
		lastPass := lastPassing(state.TaskStates[id].Checks)
		res, err := s.RunCheck(ctx, id, actor, opts)
		if err != nil {
			return issues, runs, fmt.Errorf("re-run check of %s: %w", id, err)
		}
		runs = append(runs, RecheckResult{TaskID: id, Result: res, LastPass: lastPass})
		if res.Passed {
			continue
		}
		issues = append(issues, regressionIssue(id, res, lastPass))
		if s.audit != nil {
			meta := map[string]any{"task_id": id, "command": res.Command, "exit_code": res.ExitCode, "commit": res.Commit}
			if lastPass != nil {
				meta["last_pass_commit"] = lastPass.Commit
			}
			_ = s.audit.Log("task.regression", actor, meta)
		}
	}
	return issues, runs, nil
}

func lastPassing(checks []planning.CheckResult) *planning.CheckResult {
	for i := len(checks) - 1; i >= 0; i-- {
		if checks[i].Passed {
			c := checks[i]
			return &c
		}
	}
	return nil
}

func regressionIssue(taskID string, res planning.CheckResult, lastPass *planning.CheckResult) drift.Issue {
	msg := fmt.Sprintf("%s is verified, but its acceptance check fails now (exit %d at %s): %s",
		taskID, res.ExitCode, shortCommit(res.Commit), res.Command)
	if lastPass != nil && lastPass.Commit != "" {
		msg += fmt.Sprintf("; it last passed at %s", shortCommit(lastPass.Commit))
	}
	return drift.Issue{
		ID:          "regression-" + taskID,
		Type:        drift.DriftTypeCode,
		Category:    drift.CategoryRegression,
		Severity:    drift.SeverityHigh,
		ComponentID: taskID,
		Message:     msg,
		Hint:        fmt.Sprintf("Fix the code, or reopen the task (`roady task reopen %s`) if the requirement changed. `roady task check %s` re-runs it.", taskID, taskID),
	}
}

func shortCommit(c string) string {
	switch {
	case c == "":
		return "an unknown commit"
	case len(c) > 8:
		return c[:8]
	}
	return c
}
