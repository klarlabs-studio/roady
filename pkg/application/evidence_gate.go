package application

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// EvidenceRequiredError is returned when verify_requires_evidence is on and a
// task lacks what it takes to be verified. Missing lists what is absent.
type EvidenceRequiredError struct {
	TaskID  string
	Missing []string
}

func (e *EvidenceRequiredError) Error() string {
	return fmt.Sprintf("cannot verify %s: policy verify_requires_evidence is on and the task has no %s. "+
		"Add what is missing, or have a person verify it with `roady task verify %s --override \"<reason>\"`",
		e.TaskID, strings.Join(e.Missing, " and no "), e.TaskID)
}

// commitRef matches the commit evidence roady records: "Commit: <sha>" from
// `roady git sync`, or a bare hash passed with --evidence.
var commitRef = regexp.MustCompile(`(?i)^(commit:\s*)?[0-9a-f]{7,40}$`)

func hasLinkedCommit(result planning.TaskResult) bool {
	for _, e := range result.Evidence {
		if commitRef.MatchString(strings.TrimSpace(e)) {
			return true
		}
	}
	return false
}

// requiresEvidence reports whether the project's policy gates verification on
// evidence.
func (s *TaskService) requiresEvidence() bool {
	cfg, err := s.repo.LoadPolicy()
	return err == nil && cfg != nil && cfg.VerifyRequiresEvidence
}

// ensureEvidence enforces verify_requires_evidence before any check runs, so
// a verify that would be refused for a missing commit does not first spend a
// test suite's worth of time finding out. Whether the check passes is decided
// afterwards by ensureCheckPassed, which no override lifts.
func (s *TaskService) ensureEvidence(taskID string) error {
	if !s.requiresEvidence() {
		return nil
	}
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return nil // the coordinator reports a missing state in its own terms
	}
	// An invalid transition is the coordinator's error to report.
	if !state.GetTaskStatus(taskID).CanTransitionWith("verify") {
		return nil
	}
	var missing []string
	if plan, err := s.repo.LoadPlan(); err == nil && plan != nil {
		if task, ok := findTask(plan, taskID); ok && task.Check.IsZero() {
			missing = append(missing, "acceptance check")
		}
	}
	if !hasLinkedCommit(state.TaskStates[taskID]) {
		missing = append(missing, "linked commit (commit with [roady:"+taskID+"] and run `roady git sync`, or complete it with --evidence <sha>)")
	}
	if len(missing) == 0 {
		return nil
	}
	return &EvidenceRequiredError{TaskID: taskID, Missing: missing}
}

// VerifyWithOverride verifies a task on a person's say-so where the evidence
// policy would refuse it, and records the override and its reason in the
// audit log so the trail shows the task was not verified on evidence.
//
// It lifts only missing evidence. A task whose check runs and fails is not
// done, and an override would record something the check has just shown to be
// false. Deliberately not reachable over MCP.
func (s *TaskService) VerifyWithOverride(ctx context.Context, taskID, actor, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("an override needs a reason; it is recorded in the audit trail")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if s.policy != nil {
		if err := s.policy.ValidateTransitionForOwner(taskID, "verify", actor); err != nil {
			return err
		}
	}
	check, err := s.ensureCheckPassed(ctx, taskID, actor)
	if err != nil {
		if _, manual := err.(*ManualCheckError); !manual {
			return err
		}
	}
	if err := s.coordinator.VerifyTask(ctx, taskID, actor); err != nil {
		return s.mapCoordinatorError(err, "verify")
	}
	meta := map[string]interface{}{
		"task_id":         taskID,
		"event":           "verify",
		"status":          string(planning.StatusVerified),
		"verifier":        actor,
		"override":        true,
		"override_reason": reason,
	}
	if check != nil {
		meta["check_kind"] = check.Kind
		meta["check_commit"] = check.Commit
	}
	return s.audit.Log("task.transition", actor, meta)
}
