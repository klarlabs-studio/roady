package rules

import (
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
)

// MaxWIPPerOwnerRule caps how much work any one person or agent may have in
// progress at once.
//
// The project-wide MaxWIPRule cannot do this job: a team of five sitting under
// a global limit of ten can still have one person holding nine tasks. Limiting
// per owner is what makes WIP a coordination signal rather than a headcount
// multiplier.
//
// A Limit of zero disables the rule, so existing projects are unaffected until
// they opt in.
type MaxWIPPerOwnerRule struct {
	Limit int `yaml:"limit"`
}

func (r *MaxWIPPerOwnerRule) ID() string {
	return "max-wip-per-owner"
}

func (r *MaxWIPPerOwnerRule) Validate(plan *planning.Plan, state *planning.ExecutionState) []policy.Violation {
	if plan == nil || state == nil || r.Limit <= 0 {
		return nil
	}

	// Unassigned work is deliberately not counted: it belongs to nobody, so
	// attributing it to a phantom owner would produce a violation no one can
	// act on. `roady task unassigned` surfaces it instead.
	counts := map[string]int{}
	display := map[string]string{}

	for _, task := range plan.Tasks {
		res, ok := state.TaskStates[task.ID]
		if !ok || res.Status != planning.StatusInProgress {
			continue
		}

		owner := strings.TrimSpace(res.Owner)
		if owner == "" {
			continue
		}

		key := strings.ToLower(owner)
		counts[key]++
		if _, seen := display[key]; !seen {
			display[key] = owner
		}
	}

	owners := make([]string, 0, len(counts))
	for key := range counts {
		owners = append(owners, key)
	}
	// Sorted so the same state always reports in the same order.
	sort.Strings(owners)

	var violations []policy.Violation
	for _, key := range owners {
		if counts[key] <= r.Limit {
			continue
		}
		violations = append(violations, policy.Violation{
			RuleID: r.ID(),
			Level:  policy.ViolationWarning,
			Message: fmt.Sprintf("Per-owner WIP exceeded: %s has %d tasks in progress (limit: %d).",
				display[key], counts[key], r.Limit),
		})
	}

	return violations
}
