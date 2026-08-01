package application

import (
	"fmt"
	"sort"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// StateRebuildService reconstructs execution state by replaying the event log.
//
// state.json is a whole-file JSON document, so two collaborators who both
// moved a task conflict on it in git even when their work does not actually
// disagree. events.jsonl does not have that problem: it is append-only and
// union-merges cleanly. Rebuilding makes the conflict trivial to resolve —
// take either side, replay, and the result is the same either way.
type StateRebuildService struct {
	repo domain.WorkspaceRepository
}

func NewStateRebuildService(repo domain.WorkspaceRepository) *StateRebuildService {
	return &StateRebuildService{repo: repo}
}

// RebuildResult reports what a replay produced.
type RebuildResult struct {
	EventsReplayed int
	TasksAffected  int
	Changed        []StateChange
}

// StateChange is one task whose rebuilt status differs from what was on disk.
type StateChange struct {
	TaskID string
	From   planning.TaskStatus
	To     planning.TaskStatus
}

// Rebuild replays the log and returns the resulting state without saving.
func (s *StateRebuildService) Rebuild() (*planning.ExecutionState, *RebuildResult, error) {
	events, err := s.repo.LoadEvents()
	if err != nil {
		return nil, nil, fmt.Errorf("load events: %w", err)
	}

	current, err := s.repo.LoadState()
	if err != nil {
		return nil, nil, fmt.Errorf("load state: %w", err)
	}

	planID := ""
	if current != nil {
		planID = current.ProjectID
	}

	rebuilt := planning.NewExecutionState(planID)
	result := &RebuildResult{}

	// Replay in recorded order. Events carry their own timestamps, and a
	// union merge can interleave branches, so sort rather than trusting
	// file order.
	ordered := make([]domain.Event, len(events))
	copy(ordered, events)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Timestamp.Before(ordered[j].Timestamp)
	})

	for i := range ordered {
		if applyEventToState(rebuilt, &ordered[i]) {
			result.EventsReplayed++
		}
	}

	result.TasksAffected = len(rebuilt.TaskStates)

	// Report divergence so an operator can see what a rebuild would change
	// before committing to it.
	if current != nil {
		ids := map[string]bool{}
		for id := range current.TaskStates {
			ids[id] = true
		}
		for id := range rebuilt.TaskStates {
			ids[id] = true
		}

		sorted := make([]string, 0, len(ids))
		for id := range ids {
			sorted = append(sorted, id)
		}
		sort.Strings(sorted)

		for _, id := range sorted {
			was := current.GetTaskStatus(id)
			now := rebuilt.GetTaskStatus(id)
			if was != now {
				result.Changed = append(result.Changed, StateChange{TaskID: id, From: was, To: now})
			}
		}
	}

	return rebuilt, result, nil
}

// Save replays and persists the result.
func (s *StateRebuildService) Save() (*RebuildResult, error) {
	rebuilt, result, err := s.Rebuild()
	if err != nil {
		return nil, err
	}

	// SaveState implements optimistic locking by requiring the incoming
	// version to match what is on disk, then incrementing it itself. A
	// rebuild is authoritative over whatever is there, so it carries the
	// on-disk version forward rather than pre-incrementing.
	if current, cErr := s.repo.LoadState(); cErr == nil && current != nil {
		rebuilt.Version = current.Version
	}

	if err := s.repo.SaveState(rebuilt); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}

	return result, nil
}

// applyEventToState folds one event into the state, returning whether it was
// a task event the replay understood.
//
// Ownership is taken from task.started rather than from a dedicated
// assignment event, because that is what actually records who picked the work
// up. Later assignment events override it.
func applyEventToState(state *planning.ExecutionState, e *domain.Event) bool {
	taskID, _ := e.Metadata["task_id"].(string)
	if taskID == "" {
		return false
	}

	action := e.Action

	switch action {
	case "task.started":
		setStatus(state, taskID, planning.StatusInProgress)
		if e.Actor != "" && e.Actor != "system" {
			state.SetTaskOwner(taskID, e.Actor)
		}
		return true

	case "task.completed":
		setStatus(state, taskID, planning.StatusDone)
		if ev, ok := e.Metadata["evidence"].(string); ok && ev != "" {
			appendEvidence(state, taskID, ev)
		}
		return true

	case "task.blocked":
		setStatus(state, taskID, planning.StatusBlocked)
		return true

	case "task.unblocked":
		setStatus(state, taskID, planning.StatusPending)
		return true

	case "task.verified":
		setStatus(state, taskID, planning.StatusVerified)
		return true

	case "task.assign":
		if assignee, ok := e.Metadata["assignee"].(string); ok && assignee != "" {
			state.SetTaskOwner(taskID, assignee)
		}
		return true

	case "task.transition":
		// The transition event carries the resulting status directly, which
		// is more reliable than re-deriving it from the event name.
		if status, ok := e.Metadata["status"].(string); ok && status != "" {
			setStatus(state, taskID, planning.TaskStatus(status))
		}
		if ev, ok := e.Metadata["evidence"].(string); ok && ev != "" {
			appendEvidence(state, taskID, ev)
		}
		return true
	}

	return false
}

func setStatus(state *planning.ExecutionState, taskID string, status planning.TaskStatus) {
	result, _ := state.GetTaskResult(taskID)
	result.Status = status
	state.TaskStates[taskID] = result
}

func appendEvidence(state *planning.ExecutionState, taskID, evidence string) {
	result, _ := state.GetTaskResult(taskID)
	for _, existing := range result.Evidence {
		if existing == evidence {
			return
		}
	}
	result.Evidence = append(result.Evidence, evidence)
	state.TaskStates[taskID] = result
}
