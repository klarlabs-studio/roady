package main

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestJiraPriorityName(t *testing.T) {
	tests := []struct {
		name     string
		priority planning.TaskPriority
		want     string
	}{
		{name: "high", priority: planning.PriorityHigh, want: "High"},
		{name: "medium", priority: planning.PriorityMedium, want: "Medium"},
		{name: "low", priority: planning.PriorityLow, want: "Low"},
		// "" means "leave it alone" rather than clearing a human's value.
		{name: "unset leaves the field untouched", priority: "", want: ""},
		{name: "unknown leaves the field untouched", priority: "blocker", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jiraPriorityName(tt.priority); got != tt.want {
				t.Errorf("jiraPriorityName(%q) = %q, want %q", tt.priority, got, tt.want)
			}
		})
	}
}
