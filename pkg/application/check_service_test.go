package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// fakeRunner stands in for the shell: it records the command and returns the
// configured exit code.
type fakeRunner struct {
	exit  int
	out   string
	calls []string
}

func (f *fakeRunner) run(_ context.Context, _ string, command string) (int, []byte, error) {
	f.calls = append(f.calls, command)
	return f.exit, []byte(f.out), nil
}

func checkedService(status planning.TaskStatus, check *planning.Check) (*application.TaskService, *MockRepo, *fakeRunner) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1", Title: "T1", Check: check}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{"t1": {Status: status, Owner: "dev"}},
		},
	}
	svc := application.NewTaskService(repo, application.NewAuditService(repo), application.NewPolicyService(repo))
	runner := &fakeRunner{}
	svc.SetCheckRunner(runner.run)
	return svc, repo, runner
}

func TestRunCheckRecordsPassAndFail(t *testing.T) {
	svc, repo, runner := checkedService(planning.StatusInProgress, &planning.Check{Run: "go test ./x"})

	res, err := svc.RunCheck(context.Background(), "t1", "dev", application.CheckOptions{})
	if err != nil || !res.Passed || res.Kind != planning.CheckKindRun {
		t.Fatalf("expected a recorded pass, got %+v, %v", res, err)
	}

	runner.exit, runner.out = 1, "FAIL: TestX"
	res, err = svc.RunCheck(context.Background(), "t1", "dev", application.CheckOptions{})
	if err != nil {
		t.Fatalf("a failing check is a result, not an error: %v", err)
	}
	if res.Passed || res.ExitCode != 1 || !strings.Contains(res.Output, "FAIL: TestX") {
		t.Fatalf("expected a recorded failure with output, got %+v", res)
	}

	checks := repo.State.TaskStates["t1"].Checks
	if len(checks) != 2 || !checks[0].Passed || checks[1].Passed {
		t.Fatalf("both runs must be kept as evidence, newest last: %+v", checks)
	}
	if got := repo.State.TaskStates["t1"].Status; got != planning.StatusInProgress {
		t.Errorf("running a check must not change the status, got %s", got)
	}
}

func TestRunCheckWithoutCheck(t *testing.T) {
	svc, _, _ := checkedService(planning.StatusInProgress, nil)
	if _, err := svc.RunCheck(context.Background(), "t1", "dev", application.CheckOptions{}); !errors.Is(err, application.ErrNoCheck) {
		t.Fatalf("expected ErrNoCheck, got %v", err)
	}
}

func TestManualCheckNeedsConfirmation(t *testing.T) {
	svc, repo, runner := checkedService(planning.StatusDone, &planning.Check{Manual: "Invoice PDF opens in Preview"})

	var manual *application.ManualCheckError
	if _, err := svc.RunCheck(context.Background(), "t1", "agent", application.CheckOptions{}); !errors.As(err, &manual) {
		t.Fatalf("an unconfirmed manual check must be refused, got %v", err)
	}
	if len(repo.State.TaskStates["t1"].Checks) != 0 {
		t.Fatal("a refused manual check must not be recorded as a result")
	}
	res, err := svc.RunCheck(context.Background(), "t1", "felix", application.CheckOptions{ConfirmManual: true})
	if err != nil || !res.Passed || res.By != "felix" {
		t.Fatalf("expected a confirmed manual check by felix, got %+v, %v", res, err)
	}
	if len(runner.calls) != 0 {
		t.Error("a manual check must never execute anything")
	}
}

// verify runs the check against the code as it is and refuses when it fails.
func TestVerifyIsGatedOnCheck(t *testing.T) {
	svc, repo, runner := checkedService(planning.StatusDone, &planning.Check{Run: "make test"})
	runner.exit = 2

	err := svc.TransitionTask("t1", "verify", "agent", "")
	var failed *application.CheckFailedError
	if !errors.As(err, &failed) {
		t.Fatalf("expected verification to be refused, got %v", err)
	}
	if got := repo.State.TaskStates["t1"].Status; got != planning.StatusDone {
		t.Fatalf("a refused verify must leave the task done, got %s", got)
	}

	runner.exit = 0
	if err := svc.TransitionTask("t1", "verify", "agent", ""); err != nil {
		t.Fatalf("a passing check should allow verify: %v", err)
	}
	if got := repo.State.TaskStates["t1"].Status; got != planning.StatusVerified {
		t.Fatalf("expected verified, got %s", got)
	}
	if n := len(runner.calls); n != 2 {
		t.Errorf("verify must run the check each time rather than trust an earlier pass, ran %d times", n)
	}
}

func TestVerifyManualCheckUsesRecordedConfirmation(t *testing.T) {
	svc, repo, _ := checkedService(planning.StatusDone, &planning.Check{Manual: "Reviewed by accounting"})

	var manual *application.ManualCheckError
	if err := svc.TransitionTask("t1", "verify", "agent", ""); !errors.As(err, &manual) {
		t.Fatalf("verify without a confirmation must be refused, got %v", err)
	}
	if _, err := svc.RunCheck(context.Background(), "t1", "felix", application.CheckOptions{ConfirmManual: true}); err != nil {
		t.Fatal(err)
	}
	if err := svc.TransitionTask("t1", "verify", "felix", ""); err != nil {
		t.Fatalf("verify after confirmation: %v", err)
	}
	if got := repo.State.TaskStates["t1"].Status; got != planning.StatusVerified {
		t.Fatalf("expected verified, got %s", got)
	}
}

// Verifying a task in the wrong state reports the transition error; it does not
// run the check first.
func TestVerifyWrongStateDoesNotRunCheck(t *testing.T) {
	svc, _, runner := checkedService(planning.StatusPending, &planning.Check{Run: "make test"})
	if err := svc.TransitionTask("t1", "verify", "agent", ""); err == nil {
		t.Fatal("verifying a pending task must fail")
	}
	if len(runner.calls) != 0 {
		t.Errorf("check ran for an invalid transition: %v", runner.calls)
	}
}

// Without a check, verify behaves as before; requiring evidence is a separate
// policy.
func TestVerifyWithoutCheckIsUnchanged(t *testing.T) {
	svc, repo, _ := checkedService(planning.StatusDone, nil)
	if err := svc.TransitionTask("t1", "verify", "agent", ""); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got := repo.State.TaskStates["t1"].Status; got != planning.StatusVerified {
		t.Fatalf("expected verified, got %s", got)
	}
}

func evidenceService(check *planning.Check, evidence ...string) (*application.TaskService, *MockRepo, *fakeRunner) {
	svc, repo, runner := checkedService(planning.StatusDone, check)
	repo.Policy = &domain.PolicyConfig{VerifyRequiresEvidence: true}
	tr := repo.State.TaskStates["t1"]
	tr.Evidence = evidence
	repo.State.TaskStates["t1"] = tr
	return svc, repo, runner
}

func TestEvidencePolicyRequiresCheckAndCommit(t *testing.T) {
	svc, repo, _ := evidenceService(nil)
	err := svc.TransitionTask("t1", "verify", "agent", "")
	var ev *application.EvidenceRequiredError
	if !errors.As(err, &ev) || len(ev.Missing) != 2 {
		t.Fatalf("expected both check and commit reported missing, got %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusDone {
		t.Fatal("a refused verify must leave the task done")
	}
}

// A missing commit is found before the check runs, so a refusal does not cost
// a test suite.
func TestEvidencePolicyRefusesBeforeRunningCheck(t *testing.T) {
	svc, _, runner := evidenceService(&planning.Check{Run: "make test"})
	var ev *application.EvidenceRequiredError
	if err := svc.TransitionTask("t1", "verify", "agent", ""); !errors.As(err, &ev) {
		t.Fatalf("expected a missing-commit refusal, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("check ran although verification was already refused: %v", runner.calls)
	}
}

func TestEvidencePolicyAcceptsCommitAndPassingCheck(t *testing.T) {
	for _, commit := range []string{"Commit: 84924d29fd01dc3660fd8c7a9b5a00b67d984479", "8cc57d2"} {
		svc, repo, _ := evidenceService(&planning.Check{Run: "make test"}, commit)
		if err := svc.TransitionTask("t1", "verify", "agent", ""); err != nil {
			t.Fatalf("evidence %q: %v", commit, err)
		}
		if repo.State.TaskStates["t1"].Status != planning.StatusVerified {
			t.Fatalf("evidence %q: expected verified", commit)
		}
	}
}

func TestOverrideLiftsMissingEvidenceButNotAFailingCheck(t *testing.T) {
	svc, _, runner := evidenceService(&planning.Check{Run: "make test"}, "8cc57d2")
	runner.exit = 1
	var failed *application.CheckFailedError
	if err := svc.VerifyWithOverride(context.Background(), "t1", "felix", "shipped anyway"); !errors.As(err, &failed) {
		t.Fatalf("an override must not verify over a failing check, got %v", err)
	}

	svc, repo, _ := evidenceService(nil)
	if err := svc.VerifyWithOverride(context.Background(), "t1", "felix", ""); err == nil {
		t.Fatal("an override without a reason must be refused")
	}
	if err := svc.VerifyWithOverride(context.Background(), "t1", "felix", "verified by hand in staging"); err != nil {
		t.Fatalf("override: %v", err)
	}
	if repo.State.TaskStates["t1"].Status != planning.StatusVerified {
		t.Fatal("expected verified after override")
	}
}
