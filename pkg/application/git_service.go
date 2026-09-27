package application

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

type GitService struct {
	repo    domain.WorkspaceRepository
	taskSvc *TaskService
}

func NewGitService(repo domain.WorkspaceRepository, taskSvc *TaskService) *GitService {
	return &GitService{repo: repo, taskSvc: taskSvc}
}

// SyncMarkers scans the last n commits for [roady:task-id] markers and completes tasks.
func (s *GitService) SyncMarkers(n int) ([]string, error) {
	// Validate input bounds to prevent abuse
	if n < 1 {
		n = 1
	}
	if n > 1000 {
		n = 1000 // Cap at reasonable maximum
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// #nosec G204 -- n is bounds-checked integer, safe for command line
	cmd := exec.CommandContext(ctx, "git", "log", "-n", fmt.Sprintf("%d", n), "--pretty=format:%H|%s")
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("git log timed out after 30 seconds")
		}
		return nil, fmt.Errorf("failed to read git log: %w", err)
	}

	lines := strings.Split(string(out), "\n")
	results := []string{}

	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		hash, message := parts[0], parts[1]

		if strings.Contains(message, "[roady:") {
			start := strings.Index(message, "[roady:") + 7
			end := strings.Index(message[start:], "]")
			if end != -1 {
				taskID := message[start : start+end]

				// A task completed before its commit existed still needs the
				// commit as evidence: skipping it left completed work with no
				// linked commit, which verify_requires_evidence then refused.
				if linked, done := s.linkCommitToFinishedTask(taskID, hash); done {
					if linked {
						results = append(results, fmt.Sprintf("Task %s: linked %s as evidence (already %s)", taskID, hash[:8], s.statusOf(taskID)))
					} else {
						results = append(results, fmt.Sprintf("Task %s: %s already linked", taskID, hash[:8]))
					}
					continue
				}

				// A commit naming a task nobody started is still the work:
				// start it first rather than skip it. Dependencies, approval
				// and claims guard the start as they would any other.
				started := false
				if st := s.statusOf(taskID); st == string(planning.StatusPending) {
					if err := s.taskSvc.TransitionTask(taskID, "start", "git-automation", ""); err != nil {
						results = append(results, fmt.Sprintf("Task %s: skip (never started, and cannot start: %v)", taskID, err))
						continue
					}
					started = true
				}
				err := s.taskSvc.TransitionTask(taskID, "complete", "git-automation", "Commit: "+hash)
				switch {
				case err != nil:
					results = append(results, fmt.Sprintf("Task %s: skip (%v)", taskID, err))
				case started:
					results = append(results, fmt.Sprintf("Task %s: started and completed via %s (it was never started)", taskID, hash[:8]))
				default:
					results = append(results, fmt.Sprintf("Task %s: completed via %s", taskID, hash[:8]))
				}
			}
		}
	}

	return results, nil
}

// linkCommitToFinishedTask records hash as evidence on a task that is already
// done or verified. done reports whether the task was finished (so the caller
// should not try to complete it); linked reports whether evidence was added,
// false when this commit was already recorded.
func (s *GitService) linkCommitToFinishedTask(taskID, hash string) (linked, done bool) {
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return false, false
	}
	result, ok := state.TaskStates[taskID]
	if !ok || (result.Status != planning.StatusDone && result.Status != planning.StatusVerified) {
		return false, false
	}
	evidence := "Commit: " + hash
	for _, e := range result.Evidence {
		if e == evidence {
			return false, true
		}
	}
	state.AddEvidence(taskID, evidence)
	if err := s.repo.SaveState(state); err != nil {
		return false, false
	}
	if s.taskSvc != nil && s.taskSvc.audit != nil {
		_ = s.taskSvc.audit.Log("task.evidence", "git-automation", map[string]any{
			"task_id":  taskID,
			"evidence": evidence,
		})
	}
	return true, true
}

func (s *GitService) statusOf(taskID string) string {
	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return "finished"
	}
	return string(state.GetTaskStatus(taskID))
}
