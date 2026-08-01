package main

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestLinearPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority planning.TaskPriority
		want     int
	}{
		{name: "high", priority: planning.PriorityHigh, want: 2},
		{name: "medium", priority: planning.PriorityMedium, want: 3},
		{name: "low", priority: planning.PriorityLow, want: 4},
		// -1 means "leave it alone". Returning 0 would be Linear's
		// "no priority" and would clear a value a human set.
		{name: "unset leaves the field untouched", priority: "", want: -1},
		{name: "unknown leaves the field untouched", priority: "urgent", want: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := linearPriority(tt.priority); got != tt.want {
				t.Errorf("linearPriority(%q) = %d, want %d", tt.priority, got, tt.want)
			}
		})
	}
}
