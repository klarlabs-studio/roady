package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestRecheckVerifiedReportsRegressions(t *testing.T) {
	dir := claimsProject(t, "1h")
	svc, repo := agent(dir)
	plan, _ := repo.LoadPlan()
	plan.Tasks = []planning.Task{
		{ID: "task-ok", Title: "ok", FeatureID: "f", Check: &planning.Check{Run: "pass"}},
		{ID: "task-broken", Title: "broken", FeatureID: "f", Check: &planning.Check{Run: "fail"}},
		{ID: "task-manual", Title: "manual", FeatureID: "f", Check: &planning.Check{Manual: "look at it"}},
		{ID: "task-none", Title: "none", FeatureID: "f"},
		{ID: "task-open", Title: "open", FeatureID: "f", Check: &planning.Check{Run: "fail"}},
	}
	_ = repo.SavePlan(plan)
	st, _ := repo.LoadState()
	for _, id := range []string{"task-ok", "task-broken", "task-manual", "task-none"} {
		st.SetTaskStatus(id, planning.StatusVerified)
	}
	st.SetTaskStatus("task-open", planning.StatusDone)
	st.TaskStates["task-broken"] = func() planning.TaskResult {
		r := st.TaskStates["task-broken"]
		r.Checks = []planning.CheckResult{{Kind: "run", Passed: true, Commit: "1234567890abcdef"}}
		return r
	}()
	_ = repo.SaveState(st)

	var ran []string
	svc.SetCheckRunner(func(_ context.Context, _ string, cmd string) (int, []byte, error) {
		ran = append(ran, cmd)
		if cmd == "fail" {
			return 1, []byte("boom"), nil
		}
		return 0, nil, nil
	})
	issues, runs, err := svc.RecheckVerified(t.Context(), "ci", application.CheckOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ran, ",") != "fail,pass" {
		t.Errorf("ran %v: only verified tasks with a run check are re-run, in id order", ran)
	}
	if len(runs) != 2 || len(issues) != 1 {
		t.Fatalf("runs %d, issues %+v", len(runs), issues)
	}
	is := issues[0]
	if is.Category != drift.CategoryRegression || is.ComponentID != "task-broken" || is.Severity != drift.SeverityHigh ||
		!strings.Contains(is.Message, "last passed at 12345678") || !strings.Contains(is.Hint, "roady task reopen task-broken") {
		t.Errorf("issue %+v", is)
	}
	st, _ = repo.LoadState()
	if last, _ := st.TaskStates["task-broken"].LastCheck(); last.Passed || last.By != "ci" {
		t.Errorf("the failing run was not recorded: %+v", last)
	}
	if st.GetTaskStatus("task-broken") != planning.StatusVerified {
		t.Error("a regression changes no status; drift reports it")
	}
	events, _ := repo.LoadEvents()
	found := false
	for _, e := range events {
		found = found || (e.Action == "task.regression" && e.Metadata["task_id"] == "task-broken")
	}
	if !found {
		t.Error("the regression was not recorded in the audit log")
	}
}
