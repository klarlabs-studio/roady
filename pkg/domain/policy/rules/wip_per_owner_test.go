package rules

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestMaxWIPPerOwnerRule(t *testing.T) {
	plan := &planning.Plan{Tasks: []planning.Task{
		{ID: "t1"}, {ID: "t2"}, {ID: "t3"}, {ID: "t4"}, {ID: "t5"},
	}}

	tests := []struct {
		name           string
		limit          int
		states         map[string]planning.TaskResult
		wantViolations int
		wantContains   []string
	}{
		{
			name:  "disabled when limit is zero",
			limit: 0,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "alice"},
				"t2": {Status: planning.StatusInProgress, Owner: "alice"},
				"t3": {Status: planning.StatusInProgress, Owner: "alice"},
			},
			wantViolations: 0,
		},
		{
			name:  "within limit",
			limit: 2,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "alice"},
				"t2": {Status: planning.StatusInProgress, Owner: "bob"},
			},
			wantViolations: 0,
		},
		{
			name:  "one owner over the limit",
			limit: 2,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "alice"},
				"t2": {Status: planning.StatusInProgress, Owner: "alice"},
				"t3": {Status: planning.StatusInProgress, Owner: "alice"},
				"t4": {Status: planning.StatusInProgress, Owner: "bob"},
			},
			wantViolations: 1,
			wantContains:   []string{"alice", "3", "2"},
		},
		{
			name:  "two owners over the limit each get a violation",
			limit: 1,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "alice"},
				"t2": {Status: planning.StatusInProgress, Owner: "alice"},
				"t3": {Status: planning.StatusInProgress, Owner: "bob"},
				"t4": {Status: planning.StatusInProgress, Owner: "bob"},
			},
			wantViolations: 2,
		},
		{
			name:  "owner matching is case-insensitive so one person is not two",
			limit: 1,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "alice"},
				"t2": {Status: planning.StatusInProgress, Owner: "Alice"},
			},
			wantViolations: 1,
		},
		{
			name:  "unassigned in-progress work is not attributed to anyone",
			limit: 1,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress},
				"t2": {Status: planning.StatusInProgress},
			},
			wantViolations: 0,
		},
		{
			name:  "only in-progress work counts",
			limit: 1,
			states: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress, Owner: "alice"},
				"t2": {Status: planning.StatusDone, Owner: "alice"},
				"t3": {Status: planning.StatusBlocked, Owner: "alice"},
				"t4": {Status: planning.StatusPending, Owner: "alice"},
			},
			wantViolations: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := planning.NewExecutionState("plan-1")
			state.TaskStates = tt.states

			rule := &MaxWIPPerOwnerRule{Limit: tt.limit}
			got := rule.Validate(plan, state)

			if len(got) != tt.wantViolations {
				t.Fatalf("expected %d violations, got %d: %+v", tt.wantViolations, len(got), got)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got[0].Message, want) {
					t.Errorf("message %q should mention %q", got[0].Message, want)
				}
			}
		})
	}
}

func TestMaxWIPPerOwnerRuleIsDeterministic(t *testing.T) {
	plan := &planning.Plan{Tasks: []planning.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}, {ID: "t4"}}}
	state := planning.NewExecutionState("plan-1")
	state.TaskStates = map[string]planning.TaskResult{
		"t1": {Status: planning.StatusInProgress, Owner: "zoe"},
		"t2": {Status: planning.StatusInProgress, Owner: "zoe"},
		"t3": {Status: planning.StatusInProgress, Owner: "adam"},
		"t4": {Status: planning.StatusInProgress, Owner: "adam"},
	}

	rule := &MaxWIPPerOwnerRule{Limit: 1}

	// Map iteration order must not leak into the output, or the same state
	// produces different reports run to run.
	first := rule.Validate(plan, state)
	for range 20 {
		got := rule.Validate(plan, state)
		if len(got) != len(first) {
			t.Fatalf("violation count varies between runs")
		}
		for i := range got {
			if got[i].Message != first[i].Message {
				t.Fatalf("violation order varies between runs: %q vs %q", got[i].Message, first[i].Message)
			}
		}
	}

	if !strings.Contains(first[0].Message, "adam") {
		t.Errorf("expected violations sorted by owner, got %q first", first[0].Message)
	}
}

func TestMaxWIPPerOwnerRuleNilInputs(t *testing.T) {
	rule := &MaxWIPPerOwnerRule{Limit: 1}

	if got := rule.Validate(nil, planning.NewExecutionState("p")); got != nil {
		t.Errorf("nil plan should yield no violations, got %+v", got)
	}
	if got := rule.Validate(&planning.Plan{}, nil); got != nil {
		t.Errorf("nil state should yield no violations, got %+v", got)
	}
}
