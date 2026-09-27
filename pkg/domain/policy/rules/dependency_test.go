package rules

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestDependencyRuleID(t *testing.T) {
	rule := &DependencyRule{}
	if id := rule.ID(); id != "dependency-check" {
		t.Fatalf("expected dependency-check, got %s", id)
	}
}

func TestDependencyRuleDetectsViolation(t *testing.T) {
	rule := &DependencyRule{}
	plan := &planning.Plan{
		Tasks: []planning.Task{
			{ID: "task-main", DependsOn: []string{"task-pre"}},
		},
	}
	state := planning.NewExecutionState("proj")
	state.TaskStates["task-main"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["task-pre"] = planning.TaskResult{Status: planning.StatusPending}

	violations := rule.Validate(plan, state)
	if len(violations) != 1 {
		t.Fatalf("expected violation, got %d", len(violations))
	}
	if violations[0].RuleID != "dependency-check" {
		t.Fatalf("rule id mismatch: %s", violations[0].RuleID)
	}
}

// A verified dependency is complete; so is one in another project, which
// this project's state cannot speak for.
func TestDependencyRuleAcceptsVerifiedAndExternal(t *testing.T) {
	plan := &planning.Plan{Tasks: []planning.Task{
		{ID: "a"}, {ID: "b", DependsOn: []string{"a", "@other:x"}},
	}}
	state := planning.NewExecutionState("p")
	state.SetTaskStatus("a", planning.StatusVerified)
	state.SetTaskStatus("b", planning.StatusInProgress)
	if v := (&DependencyRule{}).Validate(plan, state); len(v) != 0 {
		t.Errorf("violations %+v", v)
	}
}
