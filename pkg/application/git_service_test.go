package application_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func TestGitService_SyncMarkers_BoundsCheck(t *testing.T) {
	repo := &MockRepo{
		Spec: &spec.ProductSpec{ID: "s1"},
		Plan: &planning.Plan{
			ID:             "p1",
			ApprovalStatus: planning.ApprovalApproved,
			Tasks:          []planning.Task{{ID: "t1", Title: "Task 1"}},
		},
		State:  planning.NewExecutionState("p1"),
		Policy: &domain.PolicyConfig{MaxWIP: 5, AllowAI: true},
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	taskSvc := application.NewTaskService(repo, audit, policy)
	gitSvc := application.NewGitService(repo, taskSvc)

	// SyncMarkers requires git — this tests that n bounds are handled
	// and that the service doesn't panic with small or large n values.
	// In a non-git directory, it will return an error which is fine.
	_, _ = gitSvc.SyncMarkers(0)   // Should clamp to 1
	_, _ = gitSvc.SyncMarkers(-5)  // Should clamp to 1
	_, _ = gitSvc.SyncMarkers(999) // Should work (under 1000)
}

// A task completed by hand before its commit existed used to be skipped by
// git sync ("invalid transition from done"), so it never got the commit as
// evidence and could not satisfy verify_requires_evidence.
func TestGitService_SyncLinksCommitToFinishedTask(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("commit", "-q", "--allow-empty", "-m", "implement it [roady:t1]")
	t.Chdir(dir)

	state := planning.NewExecutionState("p1")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusDone}
	repo := &MockRepo{
		Plan: &planning.Plan{
			ID: "p1", ApprovalStatus: planning.ApprovalApproved,
			Tasks: []planning.Task{{ID: "t1", Title: "Task 1"}},
		},
		State:  state,
		Policy: &domain.PolicyConfig{MaxWIP: 5},
	}
	audit := application.NewAuditService(repo)
	gitSvc := application.NewGitService(repo, application.NewTaskService(repo, audit, application.NewPolicyService(repo)))

	for i := 0; i < 2; i++ {
		if _, err := gitSvc.SyncMarkers(5); err != nil {
			t.Fatalf("sync: %v", err)
		}
	}
	got := repo.State.TaskStates["t1"]
	if got.Status != planning.StatusDone {
		t.Errorf("status changed to %s", got.Status)
	}
	if len(got.Evidence) != 1 || !strings.HasPrefix(got.Evidence[0], "Commit: ") {
		t.Fatalf("expected the commit linked exactly once, got %v", got.Evidence)
	}
}

// A commit naming a task nobody started used to be skipped ("invalid
// transition from pending"): the work was in the history and the plan did
// not know. Sync starts and completes it, unless the start itself is not
// allowed — then it says why.
func TestSyncCompletesUnstartedTask(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("commit", "-q", "--allow-empty", "-m", "do the first thing [roady:t1]")
	run("commit", "-q", "--allow-empty", "-m", "jump ahead [roady:t2]")
	t.Chdir(dir)

	repo := &MockRepo{
		Plan: &planning.Plan{
			ID: "p1", ApprovalStatus: planning.ApprovalApproved,
			Tasks: []planning.Task{{ID: "t1", Title: "One"}, {ID: "t2", Title: "Two", DependsOn: []string{"t3"}}, {ID: "t3", Title: "Three"}},
		},
		State:  planning.NewExecutionState("p1"),
		Policy: &domain.PolicyConfig{MaxWIP: 5},
	}
	audit := application.NewAuditService(repo)
	gitSvc := application.NewGitService(repo, application.NewTaskService(repo, audit, application.NewPolicyService(repo)))
	results, err := gitSvc.SyncMarkers(5)
	if err != nil {
		t.Fatal(err)
	}
	out := strings.Join(results, "\n")
	if got := repo.State.TaskStates["t1"]; got.Status != planning.StatusDone || len(got.Evidence) == 0 {
		t.Errorf("t1 %+v\n%s", got, out)
	}
	if !strings.Contains(out, "Task t1: started and completed via") {
		t.Errorf("results:\n%s", out)
	}
	if repo.State.GetTaskStatus("t2") != planning.StatusPending || !strings.Contains(out, "Task t2: skip (never started, and cannot start:") {
		t.Errorf("t2 must wait for its dependency:\n%s", out)
	}
}
