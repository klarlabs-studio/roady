package rules

import (
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
)

type DependencyRule struct{}

func (r *DependencyRule) ID() string {
	return "dependency-check"
}

func (r *DependencyRule) Validate(plan *planning.Plan, state *planning.ExecutionState) []policy.Violation {
	if plan == nil || state == nil {
		return nil
	}
	var violations []policy.Violation

	// Create a map for quick status lookup
	statusMap := make(map[string]planning.TaskStatus)
	for id, res := range state.TaskStates {
		statusMap[id] = res.Status
	}

	for _, task := range plan.Tasks {
		// Only check tasks that are In Progress
		if statusMap[task.ID] != planning.StatusInProgress {
			continue
		}

		// Verified is complete too; external (@project:task) dependencies are
		// resolved by the coordinator, not from this project's state.
		for _, depID := range task.LocalDependencies() {
			if !statusMap[depID].IsComplete() {
				violations = append(violations, policy.Violation{
					RuleID:  r.ID(),
					Level:   policy.ViolationError,
					Message: fmt.Sprintf("Task '%s' is in progress but depends on '%s' which is not done.", task.ID, depID),
				})
			}
		}
	}

	return violations
}
