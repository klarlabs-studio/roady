package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// claimsProject writes an approved plan with independent tasks to dir.
func claimsProject(t *testing.T, lease string, tasks ...string) string {
	t.Helper()
	dir := t.TempDir()
	repo := storage.NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	sp := &spec.ProductSpec{ID: "s", Title: "S", Features: []spec.Feature{{ID: "f", Title: "F"}}}
	_ = repo.SaveSpec(sp)
	_ = repo.SaveSpecLock(sp)
	plan := &planning.Plan{ID: "p", SpecID: "s", ApprovalStatus: planning.ApprovalApproved}
	for _, id := range tasks {
		plan.Tasks = append(plan.Tasks, planning.Task{ID: id, Title: id, FeatureID: "f"})
	}
	_ = repo.SavePlan(plan)
	_ = repo.SaveState(planning.NewExecutionState("p"))
	_ = repo.SavePolicy(&domain.PolicyConfig{ClaimLease: lease})
	return dir
}

// agent is one roady process: its own repository and services over dir.
func agent(dir string) (*application.TaskService, *storage.FilesystemRepository) {
	repo := storage.NewFilesystemRepository(dir)
	audit := application.NewAuditService(repo)
	return application.NewTaskService(repo, audit, application.NewPolicyService(repo)), repo
}

// Two agents, as two processes over one .roady/, start the same task at the
// same moment. Exactly one gets it, every time, and the loser is told who.
func TestTwoAgentsNeverDoubleClaim(t *testing.T) {
	for round := 0; round < 25; round++ {
		dir := claimsProject(t, "1h", "task-a")
		names := []string{"claude", "codex"}
		errs := make([]error, len(names))
		var start, done sync.WaitGroup
		start.Add(1)
		for i, name := range names {
			done.Add(1)
			go func(i int, name string) {
				defer done.Done()
				svc, _ := agent(dir)
				start.Wait()
				errs[i] = svc.TransitionTask("task-a", "start", name, "")
			}(i, name)
		}
		start.Done()
		done.Wait()

		won := 0
		for _, err := range errs {
			if err == nil {
				won++
			} else if !strings.Contains(err.Error(), "claimed by") && !strings.Contains(err.Error(), "invalid transition") {
				t.Fatalf("round %d: unexpected error %v", round, err)
			}
		}
		if won != 1 {
			t.Fatalf("round %d: %d agents claimed the task (errors %v)", round, won, errs)
		}
		_, repo := agent(dir)
		st, _ := repo.LoadState()
		r := st.TaskStates["task-a"]
		if r.Status != planning.StatusInProgress || r.Lease == nil || r.Lease.Holder != r.Owner {
			t.Fatalf("round %d: state %+v", round, r)
		}
	}
}

func TestClaimIsHeldAndRenewed(t *testing.T) {
	dir := claimsProject(t, "1h", "task-a")
	claude, repo := agent(dir)
	codex, _ := agent(dir)
	if err := claude.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatal(err)
	}
	err := codex.StartTask(t.Context(), "task-a", "codex")
	if err == nil || !strings.Contains(err.Error(), "claimed by claude") {
		t.Fatalf("a live claim must hold: %v", err)
	}
	st, _ := repo.LoadState()
	first := st.TaskStates["task-a"].Lease.ExpiresAt
	if d := time.Until(first); d < 55*time.Minute || d > time.Hour {
		t.Errorf("lease length %v, want the policy's 1h", d)
	}

	time.Sleep(10 * time.Millisecond)
	lease, err := claude.RenewClaim("task-a", "claude")
	if err != nil || !lease.ExpiresAt.After(first) {
		t.Fatalf("renew: %+v, %v", lease, err)
	}
	if _, err := codex.RenewClaim("task-a", "codex"); err == nil || !strings.Contains(err.Error(), "claimed by claude") {
		t.Errorf("someone else's claim must not be renewable: %v", err)
	}
	if _, err := claude.RenewClaim("task-missing", "claude"); err == nil {
		t.Error("renewing a task that is not in progress should fail")
	}

	// Completing the task ends the claim.
	if err := claude.TransitionTask("task-a", "complete", "claude", "abc"); err != nil {
		t.Fatal(err)
	}
	st, _ = repo.LoadState()
	if st.TaskStates["task-a"].Lease != nil {
		t.Error("a completed task kept its claim")
	}
}

// A holder that stops renewing loses the task: the claim is released, the
// release is on the record, and someone else can start it.
func TestExpiredClaimIsReleased(t *testing.T) {
	dir := claimsProject(t, "40ms", "task-a", "task-b")
	claude, repo := agent(dir)
	codex, _ := agent(dir)
	if err := claude.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := claude.StartTask(t.Context(), "task-b", "claude"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)

	// The brief releases what ran out.
	brief, err := codex.Brief(t.Context(), "codex")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := repo.LoadState()
	for _, id := range []string{"task-a", "task-b"} {
		if r := st.TaskStates[id]; r.Status != planning.StatusPending || r.Owner != "" || r.Lease != nil {
			t.Errorf("%s was not released: %+v", id, r)
		}
	}
	if brief.Mode != "suggested" {
		t.Errorf("a released task should be offered again, got %s", brief.Mode)
	}
	events, _ := repo.LoadEvents()
	released := 0
	for _, e := range events {
		if e.Action == "task.claim_expired" && e.Metadata["holder"] == "claude" {
			released++
		}
	}
	if released != 2 {
		t.Errorf("expected two recorded releases, got %d", released)
	}

	if err := codex.StartTask(t.Context(), "task-a", "codex"); err != nil {
		t.Fatalf("a released task should be startable: %v", err)
	}
	if _, err := claude.RenewClaim("task-a", "claude"); err == nil {
		t.Error("the old holder renewed a claim it lost")
	}
}

// Before anything reaps it, an expired claim is taken over directly by the
// next start, and that is recorded too.
func TestExpiredClaimIsTakenOver(t *testing.T) {
	dir := claimsProject(t, "30ms", "task-a")
	claude, repo := agent(dir)
	if err := claude.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	// Straight at the coordinator: no reaping first.
	if _, err := claude.GetCoordinator().ClaimTask(t.Context(), "task-a", "codex", projectClaim(time.Hour)); err != nil {
		t.Fatalf("takeover: %v", err)
	}
	st, _ := repo.LoadState()
	if r := st.TaskStates["task-a"]; r.Owner != "codex" || r.Lease.Holder != "codex" {
		t.Errorf("after takeover: %+v", r)
	}
}

// The brief is the heartbeat: it renews what the owner holds and says so.
func TestBriefRenewsTheClaim(t *testing.T) {
	dir := claimsProject(t, "200ms", "task-a")
	claude, repo := agent(dir)
	if err := claude.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatal(err)
	}
	st, _ := repo.LoadState()
	before := st.TaskStates["task-a"].Lease.ExpiresAt
	time.Sleep(120 * time.Millisecond) // past half the lease
	brief, err := claude.Brief(t.Context(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if brief.Claim == nil || !brief.Claim.ExpiresAt.After(before) {
		t.Fatalf("brief did not renew: %+v", brief.Claim)
	}
	if !strings.Contains(brief.Render(), "Claim: yours until") {
		t.Errorf("brief does not show the claim:\n%s", brief.Render())
	}
}

func TestClaimsOff(t *testing.T) {
	dir := claimsProject(t, "off", "task-a")
	claude, repo := agent(dir)
	if err := claude.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatal(err)
	}
	st, _ := repo.LoadState()
	if st.TaskStates["task-a"].Lease != nil {
		t.Error("claim_lease: off took a lease")
	}
	if _, err := claude.RenewClaim("task-a", "claude"); err == nil || !strings.Contains(err.Error(), "no claim") {
		t.Errorf("renew with claims off: %v", err)
	}
}

// A lock file left behind by a process that died does not block forever.
func TestStaleStateLockIsBroken(t *testing.T) {
	dir := claimsProject(t, "1h", "task-a")
	lock := filepath.Join(dir, ".roady", "state.json.lock")
	_ = os.WriteFile(lock, []byte("999999\n"), 0o600)
	old := time.Now().Add(-time.Minute)
	_ = os.Chtimes(lock, old, old)
	claude, _ := agent(dir)
	if err := claude.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatalf("a stale lock blocked the start: %v", err)
	}
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Error("the lock file was left behind")
	}
}

func projectClaim(ttl time.Duration) project.ClaimOptions { return project.ClaimOptions{TTL: ttl} }
