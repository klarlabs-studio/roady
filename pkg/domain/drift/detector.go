package drift

import (
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// DriftDetector is a domain service that detects various types of drift
// between spec, plan, code, and policy.
type DriftDetector struct{}

// NewDriftDetector creates a new DriftDetector instance.
func NewDriftDetector() *DriftDetector {
	return &DriftDetector{}
}

// DetectIntentDrift checks if the current spec differs from the locked spec snapshot.
func (d *DriftDetector) DetectIntentDrift(current, locked *spec.ProductSpec) []Issue {
	if locked == nil {
		return nil
	}

	if current.Hash() != locked.Hash() {
		return []Issue{{
			ID:       "intent-drift",
			Type:     DriftTypeSpec,
			Category: CategoryMismatch,
			Severity: SeverityMedium,
			Message:  "The Specification has changed since the Plan was last updated. Your intent and plan may be out of sync.",
			Hint:     "Review the changes and run 'roady plan generate' to align your plan with the new Spec.",
		}}
	}

	return nil
}

// DetectPlanDrift checks for mismatches between the spec and plan.
// Returns issues for missing tasks (requirements without tasks) and orphan tasks
// (tasks without corresponding features/requirements).
func (d *DriftDetector) DetectPlanDrift(s *spec.ProductSpec, plan *planning.Plan) []Issue {
	issues := make([]Issue, 0)

	taskIDMap := make(map[string]bool)
	specRequirementIDs := make(map[string]bool)
	specFeatureIDs := make(map[string]bool)

	if plan != nil {
		for _, t := range plan.Tasks {
			taskIDMap[t.ID] = true
		}
	}

	// Detect missing tasks for requirements
	for _, f := range s.Features {
		specFeatureIDs[f.ID] = true
		for _, r := range f.Requirements {
			taskID := fmt.Sprintf("task-%s", r.ID)
			specRequirementIDs[taskID] = true
			if !taskIDMap[taskID] {
				issues = append(issues, Issue{
					ID:          fmt.Sprintf("missing-task-%s", r.ID),
					Type:        DriftTypePlan,
					Category:    CategoryMissing,
					Severity:    SeverityHigh,
					ComponentID: r.ID,
					Message:     fmt.Sprintf("Requirement '%s' (Feature: %s) is missing from Plan.", r.Title, f.Title),
					Hint:        "Run 'roady plan generate' to update your plan.",
				})
			}
		}
	}

	// Detect orphan tasks
	if plan != nil {
		for _, t := range plan.Tasks {
			// A task with no feature was added on purpose outside the spec —
			// a quick fix, a chore. That is worth knowing, not pruning.
			if t.FeatureID == "" && !specRequirementIDs[t.ID] {
				where := "the inbox"
				if t.Goal != "" {
					where = "goal " + t.Goal
				}
				issues = append(issues, Issue{
					ID:          fmt.Sprintf("unplanned-task-%s", t.ID),
					Type:        DriftTypePlan,
					Category:    CategoryUnplanned,
					Severity:    SeverityInfo,
					ComponentID: t.ID,
					Message:     fmt.Sprintf("Task '%s' (ID: %s) is unplanned work in %s: it serves no feature or requirement.", t.Title, t.ID, where),
					Hint:        fmt.Sprintf("Fine as it is. To trace it to intent, `roady move %s --req <requirement>` or `--feature <feature>`.", t.ID),
				})
				continue
			}
			// A task is an orphan only if it doesn't match a Requirement AND doesn't match a Feature
			if !specRequirementIDs[t.ID] && !specFeatureIDs[t.FeatureID] {
				issues = append(issues, Issue{
					ID:          fmt.Sprintf("orphan-task-%s", t.ID),
					Type:        DriftTypePlan,
					Category:    CategoryOrphan,
					Severity:    SeverityMedium,
					ComponentID: t.ID,
					Message:     fmt.Sprintf("Task '%s' (ID: %s) exists in Plan but corresponds to no active Feature or Requirement in Spec.", t.Title, t.ID),
					Hint:        "Run 'roady plan prune' to remove orphan tasks or update your Spec to include this intent.",
				})
			}
		}
	}

	return issues
}

// DetectCodeDrift checks for mismatches between the plan/state and actual code.
// Returns issues for missing files, empty files, and uncommitted changes.
func (d *DriftDetector) DetectCodeDrift(plan *planning.Plan, state *planning.ExecutionState, inspector CodeInspector) []Issue {
	issues := make([]Issue, 0)

	if plan == nil || state == nil {
		return issues
	}

	for _, task := range plan.Tasks {
		result := state.TaskStates[task.ID]
		if result.Path == "" {
			continue
		}

		exists, _ := inspector.FileExists(result.Path)

		if result.Status == planning.StatusDone {
			if !exists {
				issues = append(issues, Issue{
					ID:          fmt.Sprintf("missing-code-%s", task.ID),
					Type:        DriftTypeCode,
					Category:    CategoryImplementation,
					Severity:    SeverityCritical,
					ComponentID: task.ID,
					Message:     fmt.Sprintf("Task '%s' is DONE but path '%s' is missing.", task.Title, result.Path),
					Hint:        "Restore the missing file or mark the task as incomplete using 'roady task reopen'.",
				})
			} else {
				// Check if empty
				notEmpty, _ := inspector.FileNotEmpty(result.Path)
				if !notEmpty {
					issues = append(issues, Issue{
						ID:          fmt.Sprintf("empty-code-%s", task.ID),
						Type:        DriftTypeCode,
						Category:    CategoryImplementation,
						Severity:    SeverityHigh,
						ComponentID: task.ID,
						Message:     fmt.Sprintf("Task '%s' is DONE but file '%s' is empty.", task.Title, result.Path),
						Hint:        "Ensure the task implementation is committed to the file.",
					})
				}

				// Check Git Status
				gitStatus, _ := inspector.GitStatus(result.Path)
				if gitStatus == "modified" || gitStatus == "untracked" {
					issues = append(issues, Issue{
						ID:          fmt.Sprintf("uncommitted-code-%s", task.ID),
						Type:        DriftTypeCode,
						Category:    CategoryImplementation,
						Severity:    SeverityMedium,
						ComponentID: task.ID,
						Message:     fmt.Sprintf("Task '%s' is DONE but file '%s' is %s (not committed).", task.Title, result.Path, gitStatus),
						Hint:        "Commit your changes to verify the task completion.",
					})
				}
			}
		}
	}

	return issues
}

// DetectPolicyDrift converts policy violations to drift issues.
func (d *DriftDetector) DetectPolicyDrift(violations []policy.Violation) []Issue {
	issues := make([]Issue, 0, len(violations))

	for _, v := range violations {
		severity := SeverityHigh
		if v.Level == "warning" {
			severity = SeverityMedium
		}

		issues = append(issues, Issue{
			ID:       fmt.Sprintf("policy-%s", v.RuleID),
			Type:     DriftTypePolicy,
			Category: CategoryViolation,
			Severity: severity,
			Message:  v.Message,
			Hint:     "Adjust your execution state or update policy.yaml to resolve this violation.",
		})
	}

	return issues
}
