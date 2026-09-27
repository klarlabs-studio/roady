package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// AcceptFinished records a person's acceptance of finished tasks as done
// without verification: the named tasks, or with allDone every done task not
// yet accepted. It returns the tasks it accepted.
//
// Adopting roady in a project with history leaves every earlier completion
// awaiting a verification nothing can give it. Verifying them all on a
// person's say-so would empty "verified" of its meaning, so acceptance is its
// own record: who, why and when, in the state and the audit log. The task
// stays done and can still be verified.
func (s *TaskService) AcceptFinished(ctx context.Context, taskIDs []string, allDone bool, actor, reason string) ([]string, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("accepting without verification needs a reason; it is recorded in the audit trail")
	}
	if allDone == (len(taskIDs) > 0) {
		return nil, fmt.Errorf("name the tasks to accept, or pass all-done for every finished task awaiting verification, not both")
	}
	if allDone {
		ids, err := s.awaitingVerification()
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf("no finished task is awaiting verification")
		}
		taskIDs = ids
	}
	if ctx == nil {
		ctx = context.Background()
	}
	a := planning.Acceptance{By: actor, Reason: reason, At: time.Now()}
	if err := s.coordinator.AcceptTasks(ctx, taskIDs, a); err != nil {
		return nil, err
	}
	for _, id := range taskIDs {
		if err := s.audit.Log("task.accepted", actor, map[string]interface{}{
			"task_id": id,
			"status":  string(planning.StatusDone),
			"reason":  reason,
		}); err != nil {
			return taskIDs, fmt.Errorf("accepted, but the audit log was not written: %w", err)
		}
	}
	return taskIDs, nil
}

// awaitingVerification lists the done tasks that are neither verified nor
// accepted, sorted.
func (s *TaskService) awaitingVerification() ([]string, error) {
	state, err := s.repo.LoadState()
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	if state == nil {
		return nil, nil
	}
	var ids []string
	for id, r := range state.TaskStates {
		if r.Status == planning.StatusDone && r.Accepted == nil {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
