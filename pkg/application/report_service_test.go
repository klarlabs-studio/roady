package application

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/debt"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

func TestBuildProgress(t *testing.T) {
	tests := []struct {
		name      string
		summaries []project.TaskSummary
		want      struct {
			total, done, verified, inProgress, blocked, ready, pending int
			percent                                                    float64
		}
	}{
		{
			name:      "empty plan yields zero percent without dividing by zero",
			summaries: nil,
		},
		{
			name: "counts each status bucket once",
			summaries: []project.TaskSummary{
				{ID: "1", Status: planning.StatusDone},
				{ID: "2", Status: planning.StatusVerified},
				{ID: "3", Status: planning.StatusInProgress},
				{ID: "4", Status: planning.StatusBlocked},
				{ID: "5", Status: planning.StatusPending, IsUnlocked: true},
				{ID: "6", Status: planning.StatusPending},
			},
			want: struct {
				total, done, verified, inProgress, blocked, ready, pending int
				percent                                                    float64
			}{total: 6, done: 1, verified: 1, inProgress: 1, blocked: 1, ready: 1, pending: 1, percent: 100.0 / 3},
		},
		{
			name: "verified counts toward completion",
			summaries: []project.TaskSummary{
				{ID: "1", Status: planning.StatusVerified},
				{ID: "2", Status: planning.StatusPending},
			},
			want: struct {
				total, done, verified, inProgress, blocked, ready, pending int
				percent                                                    float64
			}{total: 2, verified: 1, pending: 1, percent: 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildProgress(tt.summaries)

			if got.Total != tt.want.total {
				t.Errorf("Total = %d, want %d", got.Total, tt.want.total)
			}
			if got.Done != tt.want.done {
				t.Errorf("Done = %d, want %d", got.Done, tt.want.done)
			}
			if got.Verified != tt.want.verified {
				t.Errorf("Verified = %d, want %d", got.Verified, tt.want.verified)
			}
			if got.InProgress != tt.want.inProgress {
				t.Errorf("InProgress = %d, want %d", got.InProgress, tt.want.inProgress)
			}
			if got.Blocked != tt.want.blocked {
				t.Errorf("Blocked = %d, want %d", got.Blocked, tt.want.blocked)
			}
			if got.Ready != tt.want.ready {
				t.Errorf("Ready = %d, want %d", got.Ready, tt.want.ready)
			}
			if got.Pending != tt.want.pending {
				t.Errorf("Pending = %d, want %d", got.Pending, tt.want.pending)
			}
			if diff := got.Percent - tt.want.percent; diff > 0.001 || diff < -0.001 {
				t.Errorf("Percent = %f, want %f", got.Percent, tt.want.percent)
			}
		})
	}
}

func TestBuildAssignments(t *testing.T) {
	summaries := []project.TaskSummary{
		{ID: "t1", Title: "Bob active", Owner: "bob", Status: planning.StatusInProgress},
		{ID: "t2", Title: "Alice active", Owner: "alice", Status: planning.StatusInProgress},
		{ID: "t3", Title: "Alice blocked", Owner: "alice", Status: planning.StatusBlocked},
		{ID: "t4", Title: "Alice done", Owner: "alice", Status: planning.StatusDone},
		{ID: "t5", Title: "Nobody", Status: planning.StatusPending},
	}

	got := buildAssignments(summaries)

	if len(got) != 3 {
		t.Fatalf("expected 3 assignment groups, got %d", len(got))
	}

	// Named owners sort alphabetically, unassigned goes last.
	if got[0].Owner != "alice" || got[1].Owner != "bob" {
		t.Errorf("expected alice then bob, got %s then %s", got[0].Owner, got[1].Owner)
	}
	if !got[2].Unassigned {
		t.Errorf("expected the last group to be the unassigned bucket, got %+v", got[2])
	}

	alice := got[0]
	if alice.Active != 1 {
		t.Errorf("alice Active = %d, want 1", alice.Active)
	}
	if alice.Blocked != 1 {
		t.Errorf("alice Blocked = %d, want 1", alice.Blocked)
	}
	if alice.Done != 1 {
		t.Errorf("alice Done = %d, want 1", alice.Done)
	}
	// Completed work is counted but not listed — the report shows outstanding work.
	if len(alice.OpenTasks) != 2 {
		t.Errorf("alice OpenTasks = %d, want 2 (done task excluded)", len(alice.OpenTasks))
	}
	for _, task := range alice.OpenTasks {
		if task.ID == "t4" {
			t.Error("completed task t4 should not appear in OpenTasks")
		}
	}
}

func TestStickySeverityEscalatesWithAge(t *testing.T) {
	tests := []struct {
		name        string
		daysPending int
		want        string
	}{
		{name: "fresh debt is low", daysPending: 3, want: "low"},
		{name: "two weeks is medium", daysPending: 14, want: "medium"},
		{name: "a month is high", daysPending: 30, want: "high"},
		{name: "well past a month stays high", daysPending: 120, want: "high"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stickySeverity(&debt.DebtItem{DaysPending: tt.daysPending})
			if got != tt.want {
				t.Errorf("stickySeverity(%d) = %q, want %q", tt.daysPending, got, tt.want)
			}
		})
	}
}

func TestSeverityRankOrdersMostSevereFirst(t *testing.T) {
	if severityRank("critical") >= severityRank("high") {
		t.Error("critical should rank before high")
	}
	if severityRank("high") >= severityRank("medium") {
		t.Error("high should rank before medium")
	}
	if severityRank("medium") >= severityRank("low") {
		t.Error("medium should rank before low")
	}
	if severityRank("HIGH") != severityRank("high") {
		t.Error("severity ranking should be case-insensitive")
	}
}

func TestSummariseEvent(t *testing.T) {
	tests := []struct {
		name   string
		action string
		target string
		want   string
	}{
		{name: "task event reads as a sentence", action: "task.completed", target: "task-3", want: "task-3 completed"},
		{name: "task event without a target", action: "task.blocked", want: "was blocked"},
		{name: "non-task event prefixes the phrase", action: "drift.detected", target: "auth", want: "drift detected: auth"},
		{name: "unknown action falls back to itself", action: "custom.thing", target: "x", want: "custom.thing: x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summariseEvent(tt.action, tt.target); got != tt.want {
				t.Errorf("summariseEvent(%q, %q) = %q, want %q", tt.action, tt.target, got, tt.want)
			}
		})
	}
}

func TestEventTargetPrefersTaskID(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     string
	}{
		{name: "task_id", metadata: map[string]any{"task_id": "task-1"}, want: "task-1"},
		{name: "camelCase fallback", metadata: map[string]any{"taskID": "task-2"}, want: "task-2"},
		{name: "component fallback", metadata: map[string]any{"component_id": "auth"}, want: "auth"},
		{name: "no usable key", metadata: map[string]any{"other": "x"}, want: ""},
		{name: "non-string value ignored", metadata: map[string]any{"task_id": 42}, want: ""},
		{name: "nil metadata", metadata: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventTarget(tt.metadata); got != tt.want {
				t.Errorf("eventTarget() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsReportableActionExcludesBookkeeping(t *testing.T) {
	reportable := []string{"task.completed", "task.blocked", "plan.approved", "drift.detected"}
	for _, action := range reportable {
		if !isReportableAction(action) {
			t.Errorf("%s should be reportable", action)
		}
	}

	// Routine bookkeeping must not clutter a stakeholder report.
	noise := []string{"task.assign", "external_ref.linked", "billing.logged", "sync.completed"}
	for _, action := range noise {
		if isReportableAction(action) {
			t.Errorf("%s should not appear in a stakeholder report", action)
		}
	}
}
