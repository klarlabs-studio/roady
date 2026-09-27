package planning

import (
	"testing"
	"time"
)

func TestLeaseLifecycle(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	s := NewExecutionState("p")
	s.SetTaskStatus("t", StatusInProgress)
	s.Claim("t", "Claude", "s1", time.Hour, now)
	l := s.TaskStates["t"].Lease
	if l == nil || !l.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("claim: %+v", l)
	}
	if !l.HeldBy("claude", "s1") || !l.HeldBy("claude", "") || l.HeldBy("claude", "s2") || l.HeldBy("codex", "s1") {
		t.Error("HeldBy: holder is case-insensitive; sessions must match when both are known")
	}
	if l.Expired(now.Add(59*time.Minute)) || !l.Expired(now.Add(time.Hour)) {
		t.Error("Expired")
	}
	if !s.RenewClaim("t", "claude", "s1", time.Hour, now.Add(30*time.Minute)) {
		t.Fatal("renew failed")
	}
	if got := s.TaskStates["t"].Lease.ExpiresAt; !got.Equal(now.Add(90 * time.Minute)) {
		t.Errorf("renewed to %v", got)
	}
	if s.RenewClaim("t", "codex", "", time.Hour, now) || s.RenewClaim("t", "claude", "", 0, now) || s.RenewClaim("x", "claude", "", time.Hour, now) {
		t.Error("renewed what it should not")
	}
	if s.RenewClaim("t", "claude", "s1", time.Hour, now.Add(3*time.Hour)) {
		t.Error("an expired lease was revived")
	}
	if ids := s.ExpiredClaims(now.Add(3 * time.Hour)); len(ids) != 1 || ids[0] != "t" {
		t.Errorf("ExpiredClaims = %v", ids)
	}
	s.ReleaseClaim("t")
	if r := s.TaskStates["t"]; r.Status != StatusPending || r.Lease != nil || r.Owner != "" {
		t.Errorf("released: %+v", r)
	}
	s.ReleaseClaim("missing") // no panic, no entry
	if _, ok := s.TaskStates["missing"]; ok {
		t.Error("released a task that had no state")
	}

	s.Claim("t", "claude", "", 0, now)
	if s.TaskStates["t"].Lease != nil {
		t.Error("a zero ttl took a lease")
	}
	s.SetTaskStatus("t", StatusInProgress)
	s.Claim("t", "claude", "", time.Hour, now)
	s.SetTaskStatus("t", StatusDone)
	if s.TaskStates["t"].Lease != nil {
		t.Error("leaving in_progress kept the lease")
	}
}

func TestPruneKeepsUnplannedWork(t *testing.T) {
	tasks := []Task{{ID: "task-r", FeatureID: "f"}, {ID: "task-chore"}, {ID: "task-gone", FeatureID: "deleted"}}
	got := (&PlanReconciler{}).FilterValidTasks(tasks, map[string]bool{"task-r": true}, map[string]bool{"f": true})
	if len(got) != 2 || got[1].ID != "task-chore" {
		t.Errorf("kept %+v", got)
	}
}
