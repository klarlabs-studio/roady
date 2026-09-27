package application_test

import (
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
)

func TestComputeAdoptionStats(t *testing.T) {
	at := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	n := 0
	ev := func(action string, meta map[string]any) domain.Event {
		n++
		return domain.Event{Action: action, Timestamp: at.Add(time.Duration(n) * time.Minute), Metadata: meta}
	}
	mcp := func(session string, m map[string]any) map[string]any {
		m["session_id"], m["surface"] = session, "mcp"
		return m
	}
	events := []domain.Event{
		// Captured: two hook imports, one by hand, one ordinary capture.
		ev("plan.capture", map[string]any{"via": "plan-import-auto"}),
		ev("plan.capture", map[string]any{"via": "plan-import-auto"}),
		ev("plan.capture", map[string]any{"via": "plan-import"}),
		ev("plan.capture", map[string]any{}),
		// Proven: a verified on a check; b on someone's word; c first without,
		// then re-verified with a check (the latest counts).
		ev("task.transition", map[string]any{"event": "verify", "task_id": "a", "check_kind": "run"}),
		ev("task.transition", map[string]any{"event": "verify", "task_id": "b"}),
		ev("task.transition", map[string]any{"event": "verify", "task_id": "c"}),
		ev("task.transition", map[string]any{"event": "verify", "task_id": "c", "check_kind": "manual"}),
		// Resumed: s1 starts x (nothing in progress before: not counted).
		ev("task.transition", mcp("s1", map[string]any{"event": "start", "task_id": "x"})),
		// s2 comes back to x: resumed.
		ev("task.check", mcp("s2", map[string]any{"task_id": "x"})),
		// s3 ignores x and starts y: not resumed.
		ev("task.transition", mcp("s3", map[string]any{"event": "start", "task_id": "y"})),
		// A one-command CLI session is not a conversation: not counted.
		ev("task.transition", map[string]any{"event": "complete", "task_id": "y", "session_id": "cli-1", "surface": "cli"}),
		// A CLI session the caller named is one: it resumes x.
		ev("task.transition", map[string]any{"event": "complete", "task_id": "x", "session_id": "named", "surface": "cli", "session_given": true}),
	}
	st := application.ComputeAdoptionStats(events)
	if st.PlansCapturedAutomatically != (application.Ratio{Count: 2, Total: 3}) {
		t.Errorf("captured %+v", st.PlansCapturedAutomatically)
	}
	if st.VerifiedWithPassingCheck != (application.Ratio{Count: 2, Total: 3}) {
		t.Errorf("proven %+v", st.VerifiedWithPassingCheck)
	}
	if st.SessionsResumedRightTask != (application.Ratio{Count: 2, Total: 3}) {
		t.Errorf("resumed %+v", st.SessionsResumedRightTask)
	}
	out := st.Render()
	if !strings.Contains(out, "67% (2 of 3)") {
		t.Errorf("render:\n%s", out)
	}
	if empty := application.ComputeAdoptionStats(nil).Render(); strings.Count(empty, "n/a") != 3 {
		t.Errorf("an empty log has nothing to count:\n%s", empty)
	}
}
