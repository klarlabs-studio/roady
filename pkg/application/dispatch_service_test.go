package application

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func newDispatchFixture(t *testing.T) (*DispatchService, *storage.FilesystemRepository) {
	t.Helper()
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "spec-1", Title: "Checkout",
		Features: []spec.Feature{{
			ID: "f1", Title: "Signup",
			Requirements: []spec.Requirement{
				{ID: "signup", Title: "Email signup", Description: "A user registers with an email and password."},
			},
		}},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{
		ID: "plan-1", SpecID: "spec-1", ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-signup", Title: "Wire signup", FeatureID: "f1", Priority: planning.PriorityHigh,
				Estimate: "4h", Source: planning.TaskSource{Doc: "docs/spec.md", Line: 42}},
			{ID: "task-later", Title: "Later", FeatureID: "f1", DependsOn: []string{"task-signup"}},
		},
	}); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("plan-1")); err != nil {
		t.Fatalf("save state: %v", err)
	}

	audit := NewAuditService(repo)
	plan := NewPlanService(repo, audit)
	policy := NewPolicyService(repo)
	tasks := NewTaskService(repo, audit, policy)
	return NewDispatchService(repo, plan, tasks), repo
}

func TestDispatchBuildsABriefFromIntent(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	brief, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "claude-code", Session: "run-7"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if brief.Feature != "Signup" {
		t.Errorf("feature = %q, want Signup", brief.Feature)
	}
	if !strings.Contains(brief.Requirement, "registers with an email") {
		t.Errorf("the originating requirement should be carried: %q", brief.Requirement)
	}
	if brief.Citation != "docs/spec.md:42" {
		t.Errorf("citation = %q", brief.Citation)
	}
	if brief.Acceptance == "" {
		t.Error("acceptance should be populated when the spec says what the requirement is")
	}
}

// TestDispatchCompletionContractAttributesTheSubagent is the point of the
// feature: work that never lands as a recorded transition is invisible to the
// audit trail, so the brief must name the exact call with the agent filled in.
func TestDispatchCompletionContractAttributesTheSubagent(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	brief, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "codex", Session: "run-9"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	c := brief.Completion
	if c.Tool != "roady_transition_task" {
		t.Errorf("tool = %q", c.Tool)
	}
	if c.Arguments["agent"] != "codex" || c.Arguments["session_id"] != "run-9" {
		t.Errorf("agent/session not carried into the contract: %+v", c.Arguments)
	}
	if c.Arguments["task_id"] != "task-signup" || c.Arguments["event"] != "complete" {
		t.Errorf("unexpected arguments: %+v", c.Arguments)
	}
	if !c.EvidenceRequired {
		t.Error("evidence should be requested up front; a done task with none is the classic audit finding")
	}
	if !strings.Contains(c.CLI, "ROADY_SESSION_ID=run-9") || !strings.Contains(c.CLI, "ROADY_AGENT=codex") {
		t.Errorf("CLI form should carry the identity: %q", c.CLI)
	}
}

func TestDispatchClaimsTheTask(t *testing.T) {
	svc, repo := newDispatchFixture(t)

	if _, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "claude-code", Start: true}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	state, _ := repo.LoadState()
	result, _ := state.GetTaskResult("task-signup")
	if result.Status != planning.StatusInProgress {
		t.Errorf("status = %q, want in_progress", result.Status)
	}
	if result.Owner != "claude-code" {
		t.Errorf("owner = %q, want claude-code", result.Owner)
	}
}

func TestDispatchDryRunDoesNotClaim(t *testing.T) {
	svc, repo := newDispatchFixture(t)

	if _, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "claude-code", Start: false}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	state, _ := repo.LoadState()
	if state.GetTaskStatus("task-signup") == planning.StatusInProgress {
		t.Error("a dry run must not claim the task")
	}
}

// TestDispatchRefusesUnreadyWork guards the failure that produces a wedged
// agent: handing out a task whose prerequisites do not exist yet.
func TestDispatchRefusesUnreadyWork(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	_, err := svc.Dispatch(t.Context(), "task-later", DispatchOptions{Agent: "claude-code"})
	if err == nil {
		t.Fatal("expected a blocked task to be refused")
	}
	if !strings.Contains(err.Error(), "task-signup") {
		t.Errorf("the error should name the unmet dependency: %v", err)
	}
}

func TestDispatchRefusesAlreadyClaimed(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	if _, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "a", Start: true}); err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	_, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "b", Start: true})
	if err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("a claimed task should not be dispatched twice, got %v", err)
	}
}

func TestDispatchRequiresAnAgent(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	_, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{})
	if err == nil || !strings.Contains(err.Error(), "agent name is required") {
		t.Fatalf("an unattributed dispatch defeats the audit trail, got %v", err)
	}
}

func TestDispatchUnknownTask(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	if _, err := svc.Dispatch(t.Context(), "task-nope", DispatchOptions{Agent: "a"}); err == nil {
		t.Error("expected an error for a task not in the plan")
	}
}

func TestBriefWarningsSurfaceGaps(t *testing.T) {
	svc, _ := newDispatchFixture(t)

	brief, err := svc.Dispatch(t.Context(), "task-signup", DispatchOptions{Agent: "a"})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	// This fixture has a citation and a requirement, so there is nothing to
	// warn about — the warning path is exercised below.
	if len(brief.Warnings()) != 0 {
		t.Errorf("expected no warnings for a complete task, got %v", brief.Warnings())
	}

	bare := brief
	bare.Citation, bare.Acceptance, bare.Description, bare.Requirement = "", "", "", ""
	if len(bare.Warnings()) != 3 {
		t.Errorf("expected all three gaps reported, got %v", bare.Warnings())
	}
}
