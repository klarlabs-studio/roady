package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestStatusWordTitleDoctorWarns(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	repo := storage.NewFilesystemRepository(".")
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "s1", Title: "T", Features: []spec.Feature{{ID: "f", Title: "F"}}})
	_ = repo.SavePolicy(&domain.PolicyConfig{})
	var tasks []planning.Task
	for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
		tasks = append(tasks, planning.Task{ID: id, Title: "done", FeatureID: "f"})
	}
	tasks = append(tasks, planning.Task{ID: "g", Title: "Real work", FeatureID: "f"})
	_ = repo.SavePlan(&planning.Plan{ID: "p", SpecID: "s1", Tasks: tasks})
	st := planning.NewExecutionState("p")
	_ = repo.SaveState(st)

	out := captureStdout(t, func() { _ = doctorCmd.RunE(doctorCmd, nil) })
	if !strings.Contains(out, "6 task(s) are titled with a status word") || !strings.Contains(out, "a, b, c, d, e, … 1 more") {
		t.Errorf("doctor output:\n%s", out)
	}
}
