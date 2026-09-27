package drift

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func TestUnplannedIsNotAnOrphan(t *testing.T) {
	sp := &spec.ProductSpec{Features: []spec.Feature{{ID: "f"}}}
	plan := &planning.Plan{Tasks: []planning.Task{
		{ID: "task-chore", Title: "Chore"},
		{ID: "task-goal", Title: "Goal work", Goal: "g"},
		{ID: "task-gone", Title: "Gone", FeatureID: "deleted"},
	}}
	byID := map[string]Issue{}
	for _, i := range NewDriftDetector().DetectPlanDrift(sp, plan) {
		byID[i.ComponentID] = i
	}
	if i := byID["task-chore"]; i.Category != CategoryUnplanned || i.Severity != SeverityInfo {
		t.Errorf("chore: %+v", i)
	}
	if i := byID["task-goal"]; i.Category != CategoryUnplanned {
		t.Errorf("goal work: %+v", i)
	}
	if i := byID["task-gone"]; i.Category != CategoryOrphan {
		t.Errorf("a task whose feature was deleted is still an orphan: %+v", i)
	}

	r := &Report{Issues: []Issue{byID["task-chore"]}}
	if len(r.AtOrAbove(SeverityLow)) != 0 || len(r.AtOrAbove(SeverityInfo)) != 1 {
		t.Error("info must sit below every gate but an explicit info one")
	}
	if s, err := ParseSeverity("INFO"); err != nil || s != SeverityInfo {
		t.Errorf("ParseSeverity(info) = %q, %v", s, err)
	}
}
