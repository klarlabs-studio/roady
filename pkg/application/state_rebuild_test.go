package application

import (
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func evt(min int, action, actor string, meta map[string]any) domain.Event {
	return domain.Event{
		ID:        action + "-" + actor + "-" + time.Duration(min).String(),
		Timestamp: time.Date(2026, 8, 1, 9, min, 0, 0, time.UTC),
		Action:    action,
		Actor:     actor,
		Metadata:  meta,
	}
}

func TestApplyEventToState(t *testing.T) {
	tests := []struct {
		name       string
		event      domain.Event
		wantStatus planning.TaskStatus
		wantOwner  string
		wantApply  bool
	}{
		{
			name:       "started sets in_progress and owner",
			event:      evt(1, "task.started", "alice", map[string]any{"task_id": "t1"}),
			wantStatus: planning.StatusInProgress,
			wantOwner:  "alice",
			wantApply:  true,
		},
		{
			name:       "system actor does not become the owner",
			event:      evt(1, "task.started", "system", map[string]any{"task_id": "t1"}),
			wantStatus: planning.StatusInProgress,
			wantApply:  true,
		},
		{
			name:       "completed sets done",
			event:      evt(2, "task.completed", "system", map[string]any{"task_id": "t1"}),
			wantStatus: planning.StatusDone,
			wantApply:  true,
		},
		{
			name:       "blocked",
			event:      evt(3, "task.blocked", "system", map[string]any{"task_id": "t1"}),
			wantStatus: planning.StatusBlocked,
			wantApply:  true,
		},
		{
			name:       "unblocked returns to pending",
			event:      evt(4, "task.unblocked", "system", map[string]any{"task_id": "t1"}),
			wantStatus: planning.StatusPending,
			wantApply:  true,
		},
		{
			name:       "transition trusts the recorded status",
			event:      evt(5, "task.transition", "alice", map[string]any{"task_id": "t1", "status": "verified"}),
			wantStatus: planning.StatusVerified,
			wantApply:  true,
		},
		{
			name:      "assignment sets owner without touching status",
			event:     evt(6, "task.assign", "cli", map[string]any{"task_id": "t1", "assignee": "bob"}),
			wantOwner: "bob",
			wantApply: true,
		},
		{
			name:      "event without a task id is ignored",
			event:     evt(7, "plan.approved", "cli", map[string]any{"plan_id": "p1"}),
			wantApply: false,
		},
		{
			name:      "unknown task action is ignored",
			event:     evt(8, "task.something_new", "cli", map[string]any{"task_id": "t1"}),
			wantApply: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := planning.NewExecutionState("p")

			applied := applyEventToState(state, &tt.event)

			if applied != tt.wantApply {
				t.Fatalf("applied = %v, want %v", applied, tt.wantApply)
			}
			if !tt.wantApply {
				return
			}
			if tt.wantStatus != "" && state.GetTaskStatus("t1") != tt.wantStatus {
				t.Errorf("status = %q, want %q", state.GetTaskStatus("t1"), tt.wantStatus)
			}
			if tt.wantOwner != "" {
				result, _ := state.GetTaskResult("t1")
				if result.Owner != tt.wantOwner {
					t.Errorf("owner = %q, want %q", result.Owner, tt.wantOwner)
				}
			}
		})
	}
}

// rebuildRepo serves events and state for the replay.
type rebuildRepo struct {
	domainWorkspaceRepo
	events []domain.Event
	state  *planning.ExecutionState
	saved  *planning.ExecutionState
}

func (r *rebuildRepo) LoadEvents() ([]domain.Event, error)          { return r.events, nil }
func (r *rebuildRepo) LoadState() (*planning.ExecutionState, error) { return r.state, nil }
func (r *rebuildRepo) SaveState(s *planning.ExecutionState) error   { r.saved = s; return nil }

func TestRebuildReplaysInTimestampOrder(t *testing.T) {
	// A union merge can interleave branches, so file order is not history
	// order. Feeding them out of order must still land on the right state.
	repo := &rebuildRepo{
		state: planning.NewExecutionState("p"),
		events: []domain.Event{
			evt(3, "task.completed", "system", map[string]any{"task_id": "t1", "evidence": "abc"}),
			evt(1, "task.started", "alice", map[string]any{"task_id": "t1"}),
			evt(2, "task.blocked", "system", map[string]any{"task_id": "t2"}),
		},
	}

	rebuilt, result, err := NewStateRebuildService(repo).Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	if got := rebuilt.GetTaskStatus("t1"); got != planning.StatusDone {
		t.Errorf("t1 = %q, want done (completed is later than started)", got)
	}
	if got := rebuilt.GetTaskStatus("t2"); got != planning.StatusBlocked {
		t.Errorf("t2 = %q, want blocked", got)
	}
	if result.EventsReplayed != 3 {
		t.Errorf("EventsReplayed = %d, want 3", result.EventsReplayed)
	}

	r1, _ := rebuilt.GetTaskResult("t1")
	if len(r1.Evidence) != 1 || r1.Evidence[0] != "abc" {
		t.Errorf("evidence = %v, want [abc]", r1.Evidence)
	}
}

func TestRebuildIsIdempotent(t *testing.T) {
	events := []domain.Event{
		evt(1, "task.started", "alice", map[string]any{"task_id": "t1"}),
		evt(2, "task.completed", "system", map[string]any{"task_id": "t1", "evidence": "abc"}),
	}
	repo := &rebuildRepo{state: planning.NewExecutionState("p"), events: events}
	svc := NewStateRebuildService(repo)

	first, _, _ := svc.Rebuild()
	second, _, _ := svc.Rebuild()

	if first.GetTaskStatus("t1") != second.GetTaskStatus("t1") {
		t.Error("replaying twice must produce the same state")
	}
	r, _ := second.GetTaskResult("t1")
	if len(r.Evidence) != 1 {
		t.Errorf("evidence must not accumulate on repeat replay: %v", r.Evidence)
	}
}

func TestRebuildReportsDivergence(t *testing.T) {
	current := planning.NewExecutionState("p")
	current.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusPending}

	repo := &rebuildRepo{
		state: current,
		events: []domain.Event{
			evt(1, "task.started", "alice", map[string]any{"task_id": "t1"}),
			evt(2, "task.completed", "system", map[string]any{"task_id": "t1"}),
		},
	}

	_, result, err := NewStateRebuildService(repo).Rebuild()
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	if len(result.Changed) != 1 {
		t.Fatalf("expected 1 divergence, got %v", result.Changed)
	}
	if result.Changed[0].From != planning.StatusPending || result.Changed[0].To != planning.StatusDone {
		t.Errorf("unexpected change: %+v", result.Changed[0])
	}
}

func TestSaveCarriesDiskVersionForward(t *testing.T) {
	// SaveState requires the incoming version to match disk and increments
	// it itself; pre-incrementing produces a spurious conflict.
	current := planning.NewExecutionState("p")
	current.Version = 7

	repo := &rebuildRepo{
		state:  current,
		events: []domain.Event{evt(1, "task.started", "alice", map[string]any{"task_id": "t1"})},
	}

	if _, err := NewStateRebuildService(repo).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if repo.saved == nil {
		t.Fatal("nothing was saved")
	}
	if repo.saved.Version != 7 {
		t.Errorf("saved version = %d, want 7 (disk version carried forward)", repo.saved.Version)
	}
}
