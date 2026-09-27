package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestTaskListCommand(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	repo := storage.NewFilesystemRepository(".")
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "s1", Title: "T", Features: []spec.Feature{{ID: "f", Title: "F"}}})
	_ = repo.SavePolicy(&domain.PolicyConfig{})
	_ = repo.SavePlan(&planning.Plan{ID: "p", SpecID: "s1", ApprovalStatus: planning.ApprovalApproved, Tasks: []planning.Task{
		{ID: "t-a", Title: "Alpha", FeatureID: "f"}, {ID: "t-b", Title: "Beta", FeatureID: "f"}, {ID: "t-c", Title: "Gamma", FeatureID: "f"},
	}})
	st := planning.NewExecutionState("p")
	st.TaskStates["t-b"] = planning.TaskResult{Status: planning.StatusDone}
	_ = repo.SaveState(st)

	run := func(args ...string) (string, error) {
		taskListStatus, taskListLimit, taskQueryJSON = "all", 50, false
		var b bytes.Buffer
		taskListCmd.SetOut(&b)
		defer taskListCmd.SetOut(nil)
		if err := taskListCmd.ParseFlags(args); err != nil {
			return "", err
		}
		err := taskListCmd.RunE(taskListCmd, nil)
		return b.String(), err
	}
	out, err := run()
	if err != nil || !strings.Contains(out, "Tasks (all): 3") || !strings.Contains(out, "[done]") || !strings.Contains(out, "Gamma") {
		t.Fatalf("all: %v\n%s", err, out)
	}
	out, _ = run("--status", "done")
	if !strings.Contains(out, "Tasks (done): 1") || strings.Contains(out, "Alpha") {
		t.Errorf("done:\n%s", out)
	}
	out, _ = run("--status", "in-progress")
	if !strings.Contains(out, "Tasks (in_progress): 0") {
		t.Errorf("in-progress spelling:\n%s", out)
	}
	out, _ = run("--limit", "1")
	if !strings.Contains(out, "… 2 more") {
		t.Errorf("limit:\n%s", out)
	}
	if _, err := run("--status", "nope"); err == nil {
		t.Error("an unknown status should fail")
	}
}
