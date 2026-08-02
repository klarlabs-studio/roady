package project

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

type stubResolver map[string]planning.TaskStatus

func (s stubResolver) ExternalTaskStatus(project, taskID string) (planning.TaskStatus, bool) {
	st, ok := s[project+":"+taskID]
	return st, ok
}

func TestIsUnlockedWithExternalDependencies(t *testing.T) {
	state := planning.NewExecutionState("p")
	state.TaskStates["task-local"] = planning.TaskResult{Status: planning.StatusDone}

	task := planning.Task{ID: "t1", DependsOn: []string{"task-local", "@auth:task-signup"}}

	tests := []struct {
		name     string
		resolver ExternalStatusResolver
		want     bool
	}{
		{
			// Previously the external ref was passed to the local state
			// lookup, which reported "pending" because no such local task
			// exists — pinning the task as blocked forever.
			name:     "no resolver blocks rather than guessing",
			resolver: nil,
			want:     false,
		},
		{
			name:     "unknown external task blocks",
			resolver: stubResolver{},
			want:     false,
		},
		{
			name:     "incomplete external task blocks",
			resolver: stubResolver{"auth:task-signup": planning.StatusInProgress},
			want:     false,
		},
		{
			name:     "complete external task unlocks",
			resolver: stubResolver{"auth:task-signup": planning.StatusDone},
			want:     true,
		},
		{
			name:     "verified counts as complete",
			resolver: stubResolver{"auth:task-signup": planning.StatusVerified},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUnlocked(task, state, tt.resolver); got != tt.want {
				t.Errorf("isUnlocked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsUnlockedLocalStillBlocks(t *testing.T) {
	// A satisfied external dependency must not paper over an unmet local one.
	state := planning.NewExecutionState("p")
	state.TaskStates["task-local"] = planning.TaskResult{Status: planning.StatusInProgress}

	task := planning.Task{ID: "t1", DependsOn: []string{"task-local", "@auth:task-signup"}}
	resolver := stubResolver{"auth:task-signup": planning.StatusDone}

	if isUnlocked(task, state, resolver) {
		t.Error("an incomplete local dependency must still block")
	}
}

func TestIsUnlockedPurelyLocalUnaffected(t *testing.T) {
	state := planning.NewExecutionState("p")
	state.TaskStates["task-a"] = planning.TaskResult{Status: planning.StatusDone}

	task := planning.Task{ID: "t1", DependsOn: []string{"task-a"}}

	if !isUnlocked(task, state, nil) {
		t.Error("a plan with no cross-project edges must not need a resolver")
	}
}
