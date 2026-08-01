package planning

import (
	"strings"
	"testing"
)

func TestEventForTransition(t *testing.T) {
	tests := []struct {
		name      string
		from, to  TaskStatus
		wantEvent string
		wantOK    bool
	}{
		{name: "pending to in_progress", from: StatusPending, to: StatusInProgress, wantEvent: "start", wantOK: true},
		{name: "pending to blocked", from: StatusPending, to: StatusBlocked, wantEvent: "block", wantOK: true},
		{name: "in_progress to done", from: StatusInProgress, to: StatusDone, wantEvent: "complete", wantOK: true},
		// The same target reached by different events depending on origin —
		// the reason a reverse lookup needs both ends.
		{name: "in_progress to pending is stop", from: StatusInProgress, to: StatusPending, wantEvent: "stop", wantOK: true},
		{name: "blocked to pending is unblock", from: StatusBlocked, to: StatusPending, wantEvent: "unblock", wantOK: true},
		{name: "done to verified", from: StatusDone, to: StatusVerified, wantEvent: "verify", wantOK: true},
		{name: "done to pending is reopen", from: StatusDone, to: StatusPending, wantEvent: "reopen", wantOK: true},
		{name: "no single step blocked to done", from: StatusBlocked, to: StatusDone, wantOK: false},
		{name: "same status is not a transition", from: StatusDone, to: StatusDone, wantOK: false},
		{name: "unknown status", from: TaskStatus("bogus"), to: StatusDone, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, ok := EventForTransition(tt.from, tt.to)

			if ok != tt.wantOK {
				t.Fatalf("EventForTransition(%s, %s) ok = %v, want %v", tt.from, tt.to, ok, tt.wantOK)
			}
			if ok && event != tt.wantEvent {
				t.Errorf("event = %q, want %q", event, tt.wantEvent)
			}
		})
	}
}

func TestPathToStatus(t *testing.T) {
	tests := []struct {
		name     string
		from, to TaskStatus
		want     []string
		wantOK   bool
	}{
		{name: "same status needs no events", from: StatusDone, to: StatusDone, want: nil, wantOK: true},
		{name: "single step", from: StatusPending, to: StatusInProgress, want: []string{"start"}, wantOK: true},
		{
			name:   "blocked to done walks every intermediate state",
			from:   StatusBlocked,
			to:     StatusDone,
			want:   []string{"unblock", "start", "complete"},
			wantOK: true,
		},
		{
			name:   "pending to verified",
			from:   StatusPending,
			to:     StatusVerified,
			want:   []string{"start", "complete", "verify"},
			wantOK: true,
		},
		{
			name:   "verified back to blocked",
			from:   StatusVerified,
			to:     StatusBlocked,
			want:   []string{"reopen", "block"},
			wantOK: true,
		},
		{name: "unreachable from an unknown status", from: TaskStatus("bogus"), to: StatusDone, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PathToStatus(tt.from, tt.to)

			if ok != tt.wantOK {
				t.Fatalf("PathToStatus(%s, %s) ok = %v, want %v", tt.from, tt.to, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("path = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPathToStatusIsWalkable is the property that matters: every path the
// planner returns must actually be applicable by the FSM, step by step.
func TestPathToStatusIsWalkable(t *testing.T) {
	for _, from := range AllTaskStatuses() {
		for _, to := range AllTaskStatuses() {
			path, ok := PathToStatus(from, to)
			if !ok {
				t.Errorf("no path from %s to %s; every status pair should be reachable", from, to)
				continue
			}

			current := from
			for i, event := range path {
				next, err := current.TransitionWith(event)
				if err != nil {
					t.Fatalf("%s -> %s: step %d (%q) is not a legal transition from %s: %v",
						from, to, i, event, current, err)
				}
				current = next
			}
			if current != to {
				t.Errorf("%s -> %s: walking %v ended at %s", from, to, path, current)
			}
		}
	}
}

func TestPathToStatusIsDeterministic(t *testing.T) {
	// Map iteration order must not leak into the chosen path, or the same
	// sync produces different transitions run to run.
	first, _ := PathToStatus(StatusBlocked, StatusVerified)
	for range 50 {
		got, _ := PathToStatus(StatusBlocked, StatusVerified)
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("path varies between runs: %v vs %v", got, first)
		}
	}
}
