package mcp

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

func TestFilterByAssignee(t *testing.T) {
	tasks := []project.TaskSummary{
		{ID: "task-1", Owner: "alice"},
		{ID: "task-2", Owner: "bob"},
		{ID: "task-3", Owner: "Alice"},
		{ID: "task-4"},
	}

	tests := []struct {
		name     string
		assignee string
		wantIDs  []string
	}{
		{
			name:     "empty assignee means no filter",
			assignee: "",
			wantIDs:  []string{"task-1", "task-2", "task-3", "task-4"},
		},
		{
			name:     "whitespace-only assignee means no filter",
			assignee: "   ",
			wantIDs:  []string{"task-1", "task-2", "task-3", "task-4"},
		},
		{
			name:     "matches case-insensitively",
			assignee: "ALICE",
			wantIDs:  []string{"task-1", "task-3"},
		},
		{
			name:     "trims surrounding whitespace",
			assignee: "  bob ",
			wantIDs:  []string{"task-2"},
		},
		{
			name:     "unknown assignee yields none",
			assignee: "carol",
			wantIDs:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterByAssignee(tasks, tt.assignee)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("expected %d tasks %v, got %d", len(tt.wantIDs), tt.wantIDs, len(got))
			}
			for i, want := range tt.wantIDs {
				if got[i].ID != want {
					t.Errorf("expected %s at index %d, got %s", want, i, got[i].ID)
				}
			}
		})
	}
}
