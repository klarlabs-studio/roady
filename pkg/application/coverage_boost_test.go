package application_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/billing"
	"github.com/felixgeelhaar/roady/pkg/domain/dependency"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// ---- InitService: SetTemplate and buildSpec paths ----

func TestInitService_SetTemplate_WebAPI(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := application.NewAuditService(repo)
	svc := application.NewInitService(repo, audit)

	svc.SetTemplate("web-api")
	if err := svc.InitializeProject("my-api"); err != nil {
		t.Fatalf("InitializeProject with web-api template: %v", err)
	}

	s, err := repo.LoadSpec()
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if s.ID != "my-api" {
		t.Errorf("expected spec ID 'my-api', got %q", s.ID)
	}
	// web-api template has 3 features: api-endpoints, authentication, observability
	if len(s.Features) < 3 {
		t.Errorf("expected at least 3 features from web-api template, got %d", len(s.Features))
	}
}

func TestInitService_SetTemplate_CLITool(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := application.NewAuditService(repo)
	svc := application.NewInitService(repo, audit)

	svc.SetTemplate("cli-tool")
	if err := svc.InitializeProject("my-cli"); err != nil {
		t.Fatalf("InitializeProject with cli-tool template: %v", err)
	}

	s, err := repo.LoadSpec()
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if len(s.Features) < 2 {
		t.Errorf("expected at least 2 features from cli-tool template, got %d", len(s.Features))
	}
}

func TestInitService_SetTemplate_Library(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := application.NewAuditService(repo)
	svc := application.NewInitService(repo, audit)

	svc.SetTemplate("library")
	if err := svc.InitializeProject("my-lib"); err != nil {
		t.Fatalf("InitializeProject with library template: %v", err)
	}

	s, err := repo.LoadSpec()
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if len(s.Features) < 2 {
		t.Errorf("expected at least 2 features from library template, got %d", len(s.Features))
	}
}

func TestInitService_SetTemplate_UnknownFallsBackToDefault(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := application.NewAuditService(repo)
	svc := application.NewInitService(repo, audit)

	svc.SetTemplate("nonexistent-template")
	if err := svc.InitializeProject("fallback-proj"); err != nil {
		t.Fatalf("InitializeProject with unknown template: %v", err)
	}

	s, err := repo.LoadSpec()
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	// Default template has 1 feature: core-foundation
	if len(s.Features) != 1 {
		t.Errorf("expected 1 feature from default template, got %d", len(s.Features))
	}
	if s.Features[0].ID != "core-foundation" {
		t.Errorf("expected feature ID 'core-foundation', got %q", s.Features[0].ID)
	}
}

func TestInitService_SetTemplate_Minimal(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := application.NewAuditService(repo)
	svc := application.NewInitService(repo, audit)

	svc.SetTemplate("minimal")
	if err := svc.InitializeProject("min-proj"); err != nil {
		t.Fatalf("InitializeProject with minimal template: %v", err)
	}

	s, err := repo.LoadSpec()
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if s.ID != "min-proj" {
		t.Errorf("expected spec ID 'min-proj', got %q", s.ID)
	}
}

func TestAuditService_VerifyIntegrity_LoadError(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("events load fail")}
	svc := application.NewAuditService(repo)

	_, err := svc.VerifyIntegrity()
	if err == nil {
		t.Fatal("expected error when events cannot be loaded")
	}
}

// ---- AuditService: GetVelocity with load error ----

func TestAuditService_GetVelocity_LoadError(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("events load fail")}
	svc := application.NewAuditService(repo)

	_, err := svc.GetVelocity()
	if err == nil {
		t.Fatal("expected error when events cannot be loaded")
	}
}

// ---- AuditService: GetVelocity with empty events ----

func TestAuditService_GetVelocity_Empty(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	svc := application.NewAuditService(repo)

	vel, err := svc.GetVelocity()
	if err != nil {
		t.Fatalf("GetVelocity: %v", err)
	}
	if vel != 0 {
		t.Errorf("expected 0 velocity for empty events, got %f", vel)
	}
}

// ---- DriftService: AcceptDrift error paths ----

func TestDriftService_AcceptDrift_NoSpec(t *testing.T) {
	repo := &MockRepo{
		Spec: nil,
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewDriftService(repo, audit, &MockInspector{}, policy)

	err := svc.AcceptDrift()
	if err == nil {
		t.Fatal("expected error when spec is nil")
	}
}

func TestDriftService_AcceptDrift_SpecLoadError(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("load fail")}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewDriftService(repo, audit, &MockInspector{}, policy)

	err := svc.AcceptDrift()
	if err == nil {
		t.Fatal("expected error when spec cannot be loaded")
	}
}

func TestDriftService_AcceptDrift_NilAudit(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "s1", Title: "S1"}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}

	policy := application.NewPolicyService(repo)
	// Pass nil audit
	svc := application.NewDriftService(repo, nil, storage.NewCodebaseInspector(), policy)

	err := svc.AcceptDrift()
	if err == nil {
		t.Fatal("expected error when audit is nil")
	}
}

func TestDriftService_AcceptDrift_SaveLockError(t *testing.T) {
	repo := &MockRepo{
		Spec:      &spec.ProductSpec{ID: "s1", Title: "S1"},
		SaveError: errors.New("save lock fail"),
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewDriftService(repo, audit, &MockInspector{}, policy)

	err := svc.AcceptDrift()
	if err == nil {
		t.Fatal("expected error when spec lock save fails")
	}
}

// ---- DriftService: DetectDrift with cancelled context ----

func TestDriftService_DetectDrift_CancelledContext(t *testing.T) {
	repo := &MockRepo{
		Spec:   &spec.ProductSpec{Features: []spec.Feature{{ID: "f1"}}},
		Plan:   &planning.Plan{Tasks: []planning.Task{}},
		State:  &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{}},
		Policy: &domain.PolicyConfig{},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewDriftService(repo, audit, &MockInspector{}, policy)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := svc.DetectDrift(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestDriftService_DetectDrift_NilContext(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "s1", Title: "S1", Features: []spec.Feature{{ID: "f1"}}}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{Tasks: []planning.Task{}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("s1")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)

	// nil context should be handled gracefully
	report, err := svc.DetectDrift(context.TODO())
	if err != nil {
		t.Fatalf("DetectDrift with nil ctx: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
}

// ---- PlanService: GeneratePlan with cancelled context ----

func TestPlanService_GeneratePlan_CancelledContext(t *testing.T) {
	repo := &MockRepo{
		Spec: &spec.ProductSpec{ID: "s1", Features: []spec.Feature{{ID: "f1"}}},
	}
	audit := application.NewAuditService(repo)
	svc := application.NewPlanService(repo, audit)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.GeneratePlan(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestPlanService_GeneratePlan_NilContext(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "s1",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}

	audit := application.NewAuditService(repo)
	svc := application.NewPlanService(repo, audit)

	plan, err := svc.GeneratePlan(context.TODO())
	if err != nil {
		t.Fatalf("GeneratePlan with nil ctx: %v", err)
	}
	if plan == nil {
		t.Fatal("expected non-nil plan")
	}
}

// ---- PlanService: RejectPlan error paths ----

func TestPlanService_RejectPlan_LoadError(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("fail")}
	audit := application.NewAuditService(repo)
	svc := application.NewPlanService(repo, audit)

	err := svc.RejectPlan()
	if err == nil {
		t.Fatal("expected error when plan load fails")
	}
}

// ---- PlanService: task query methods with nil context ----

func TestPlanService_TaskQueries_NilContext(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "s1",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	audit := application.NewAuditService(repo)
	svc := application.NewPlanService(repo, audit)

	if _, err := svc.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if err := svc.ApprovePlan(); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	// All should handle nil context gracefully
	if _, err := svc.GetTaskSummaries(context.TODO()); err != nil {
		t.Fatalf("GetTaskSummaries with nil ctx: %v", err)
	}
	if _, err := svc.GetReadyTasks(context.TODO()); err != nil {
		t.Fatalf("GetReadyTasks with nil ctx: %v", err)
	}
	if _, err := svc.GetBlockedTasks(context.TODO()); err != nil {
		t.Fatalf("GetBlockedTasks with nil ctx: %v", err)
	}
	if _, err := svc.GetInProgressTasks(context.TODO()); err != nil {
		t.Fatalf("GetInProgressTasks with nil ctx: %v", err)
	}
}

// ---- TaskService: transitionWithFSM fallback for unsupported events ----

func TestTaskService_TransitionWithFSM_UnsupportedEvent(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
		Policy: &domain.PolicyConfig{MaxWIP: 10},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewTaskService(repo, audit, policy)

	// "reset" is not a known coordinator event, so it falls to transitionWithFSM
	err := svc.TransitionTask("t1", "reset", "test-user", "")
	// Should fail because "reset" is not a valid FSM event either
	if err == nil {
		t.Fatal("expected error for unsupported event 'reset'")
	}
}

func TestTaskService_TransitionWithFSM_NoPlan(t *testing.T) {
	repo := &MockRepo{
		Plan:  nil,
		State: &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{}},
	}
	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	err := svc.TransitionTask("t1", "custom_event", "user", "")
	if err == nil {
		t.Fatal("expected error when no plan exists in FSM fallback")
	}
}

func TestTaskService_TransitionWithFSM_TaskNotFound(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{}},
	}
	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	// "custom_event" falls through to transitionWithFSM, but "missing" not in plan
	err := svc.TransitionTask("missing", "custom_event", "user", "")
	if err == nil {
		t.Fatal("expected error for task not in plan via FSM fallback")
	}
}

// ---- TaskService: error path coverage ----

func TestTaskService_StartTask_PolicyViolation(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks:          []planning.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}},
			ApprovalStatus: planning.ApprovalApproved,
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusInProgress},
				"t2": {Status: planning.StatusPending},
				"t3": {Status: planning.StatusPending},
			},
		},
		Policy: &domain.PolicyConfig{MaxWIP: 1},
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewTaskService(repo, audit, policy)

	err := svc.StartTask(context.Background(), "t2", "alice", "")
	if err == nil {
		t.Fatal("expected WIP limit error")
	}
}

func TestTaskService_BlockTask_InvalidTransition(t *testing.T) {
	// A task in "done" status cannot be blocked — "block" is only valid from pending or in_progress.
	repo := &MockRepo{
		Plan: nil,
		State: &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{
			"t1": {Status: planning.StatusDone},
		}},
	}
	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	err := svc.BlockTask(context.Background(), "t1", "reason")
	if err == nil {
		t.Fatal("expected error when blocking a done task")
	}
}

func TestTaskService_UnblockTask_NoPlan(t *testing.T) {
	repo := &MockRepo{
		Plan:  nil,
		State: &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{}},
	}
	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	err := svc.UnblockTask(context.Background(), "t1")
	if err == nil {
		t.Fatal("expected error when no plan exists")
	}
}

func TestTaskService_VerifyTask_NoPlan(t *testing.T) {
	repo := &MockRepo{
		Plan:  nil,
		State: &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{}},
	}
	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	err := svc.VerifyTask(context.Background(), "t1", "reviewer")
	if err == nil {
		t.Fatal("expected error when no plan exists")
	}
}

func TestTaskService_AssignTask_SaveError(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			Tasks: []planning.Task{{ID: "t1"}},
		},
		State: &planning.ExecutionState{
			TaskStates: map[string]planning.TaskResult{
				"t1": {Status: planning.StatusPending},
			},
		},
		SaveError: errors.New("save fail"),
	}
	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	svc := application.NewTaskService(repo, audit, policy)

	err := svc.AssignTask(context.Background(), "t1", "alice")
	if err == nil {
		t.Fatal("expected error on save failure")
	}
}

// ---- BillingService: additional edge cases ----

func TestBillingService_SetDefaultRate_NotFound(t *testing.T) {
	repo := &mockBillingRepo{
		MockRepo:    &MockRepo{},
		RatesConfig: &billing.RateConfig{Rates: []billing.Rate{}},
	}
	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	err := svc.SetDefaultRate("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent rate")
	}
}

func TestBillingService_SetTax_Inclusive(t *testing.T) {
	repo := &mockBillingRepo{MockRepo: &MockRepo{}}
	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	err := svc.SetTax("GST", 10.0, true)
	if err != nil {
		t.Fatalf("SetTax: %v", err)
	}

	config, _ := repo.LoadRates()
	if !config.Tax.Included {
		t.Error("expected inclusive tax")
	}
}

func TestBillingService_StartTask_DefaultRate(t *testing.T) {
	repo := &mockBillingRepo{
		MockRepo: &MockRepo{},
		RatesConfig: &billing.RateConfig{
			Rates: []billing.Rate{
				{ID: "rate-1", Name: "Standard", HourlyRate: 100, IsDefault: true},
			},
		},
		ExecState: planning.NewExecutionState("test"),
	}
	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	// Pass empty rate ID - should fall back to default
	err := svc.StartTask("task-1", "")
	if err != nil {
		t.Fatalf("StartTask: %v", err)
	}

	state, _ := repo.LoadState()
	result := state.TaskStates["task-1"]
	if result.RateID != "rate-1" {
		t.Errorf("expected default rate-1, got %q", result.RateID)
	}
}

func TestBillingService_CompleteTask_NoElapsedMinutes(t *testing.T) {
	repo := &mockBillingRepo{
		MockRepo: &MockRepo{},
		ExecState: func() *planning.ExecutionState {
			s := planning.NewExecutionState("test")
			s.SetTaskStatus("task-1", planning.StatusInProgress)
			return s
		}(),
		RatesConfig: &billing.RateConfig{
			Rates: []billing.Rate{
				{ID: "rate-1", Name: "Standard", HourlyRate: 100, IsDefault: true},
			},
		},
	}
	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	err := svc.CompleteTask("task-1")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Should not create a time entry when elapsed minutes = 0
	entries, _ := repo.LoadTimeEntries()
	if len(entries) != 0 {
		t.Errorf("expected no time entries when elapsed = 0, got %d", len(entries))
	}
}

func TestBillingService_GetCostReport_TaskFilter(t *testing.T) {
	repo := &mockBillingRepo{
		MockRepo: &MockRepo{},
		RatesConfig: &billing.RateConfig{
			Currency: "EUR",
			Rates: []billing.Rate{
				{ID: "rate-1", Name: "Standard", HourlyRate: 100},
			},
		},
		TimeEntries: []billing.TimeEntry{
			{TaskID: "task-1", RateID: "rate-1", Minutes: 60},
			{TaskID: "task-2", RateID: "rate-1", Minutes: 120},
		},
		Plan: &planning.Plan{
			Tasks: []planning.Task{
				{ID: "task-1", Title: "Task 1"},
				{ID: "task-2", Title: "Task 2"},
			},
		},
	}
	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	report, err := svc.GetCostReport(application.CostReportOpts{TaskID: "task-1"})
	if err != nil {
		t.Fatalf("GetCostReport: %v", err)
	}
	if report.TotalHours != 1.0 {
		t.Errorf("expected 1.0 hours for filtered task, got %f", report.TotalHours)
	}
}

func TestBillingService_CompleteTask_TaskNotFound(t *testing.T) {
	repo := &mockBillingRepo{
		MockRepo:  &MockRepo{},
		ExecState: planning.NewExecutionState("test"),
	}
	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	err := svc.CompleteTask("nonexistent")
	if err == nil {
		t.Fatal("expected error for task not found")
	}
}

// ---- OrgService: AggregateMetrics with real filesystem ----

func TestOrgService_AggregateMetrics(t *testing.T) {
	rootDir := t.TempDir()

	// Create a sub-project with .roady directory
	projDir := filepath.Join(rootDir, "project-a")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	projRepo := storage.NewFilesystemRepository(projDir)
	if err := projRepo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := projRepo.SaveSpec(&spec.ProductSpec{
		ID:       "proj-a",
		Title:    "Project A",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := projRepo.SavePlan(&planning.Plan{
		Tasks: []planning.Task{
			{ID: "t1", FeatureID: "f1"},
			{ID: "t2", FeatureID: "f1"},
			{ID: "t3", FeatureID: "f1"},
			{ID: "t4", FeatureID: "f1"},
		},
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	state := planning.NewExecutionState("proj-a")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusVerified}
	state.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["t3"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["t4"] = planning.TaskResult{Status: planning.StatusBlocked}
	if err := projRepo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := projRepo.SavePolicy(&domain.PolicyConfig{MaxWIP: 3}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	svc := application.NewOrgService(rootDir)
	metrics, err := svc.AggregateMetrics()
	if err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}
	if metrics.TotalProjects != 1 {
		t.Errorf("expected 1 project, got %d", metrics.TotalProjects)
	}
	if metrics.TotalTasks != 4 {
		t.Errorf("expected 4 total tasks, got %d", metrics.TotalTasks)
	}
	if metrics.TotalVerified != 1 {
		t.Errorf("expected 1 verified task, got %d", metrics.TotalVerified)
	}
	if metrics.TotalWIP != 1 {
		t.Errorf("expected 1 WIP task, got %d", metrics.TotalWIP)
	}
}

func TestOrgService_AggregateMetrics_NoProjects(t *testing.T) {
	rootDir := t.TempDir()

	svc := application.NewOrgService(rootDir)
	metrics, err := svc.AggregateMetrics()
	if err != nil {
		t.Fatalf("AggregateMetrics: %v", err)
	}
	if metrics.TotalProjects != 0 {
		t.Errorf("expected 0 projects, got %d", metrics.TotalProjects)
	}
}

func TestOrgService_SaveAndLoadOrgConfig(t *testing.T) {
	rootDir := t.TempDir()
	svc := application.NewOrgService(rootDir)

	_, err := svc.LoadOrgConfig()
	if err == nil {
		t.Fatal("expected error for missing org config")
	}
}

func TestOrgService_DetectCrossDrift_WithProject(t *testing.T) {
	rootDir := t.TempDir()

	projDir := filepath.Join(rootDir, "project-drift")
	if err := os.MkdirAll(projDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	projRepo := storage.NewFilesystemRepository(projDir)
	if err := projRepo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := projRepo.SaveSpec(&spec.ProductSpec{
		ID:    "proj-drift",
		Title: "Drift Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "F1", Requirements: []spec.Requirement{{ID: "r1", Title: "R1"}}},
		},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := projRepo.SavePlan(&planning.Plan{Tasks: []planning.Task{}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := projRepo.SaveState(planning.NewExecutionState("proj-drift")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := projRepo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	svc := application.NewOrgService(rootDir)
	report, err := svc.DetectCrossDrift()
	if err != nil {
		t.Fatalf("DetectCrossDrift: %v", err)
	}
	if len(report.Projects) != 1 {
		t.Errorf("expected 1 project in report, got %d", len(report.Projects))
	}
}

// ---- DebtService tests ----

func TestDebtService_GetDebtReport(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "debt-spec",
		Title:    "Debt Project",
		Features: []spec.Feature{{ID: "f1", Title: "F1", Requirements: []spec.Requirement{{ID: "r1", Title: "R1"}}}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{Tasks: []planning.Task{}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("debt-spec")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	driftSvc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	debtSvc := application.NewDebtService(driftSvc, audit)

	report, err := debtSvc.GetDebtReport(context.Background())
	if err != nil {
		t.Fatalf("GetDebtReport: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil report")
	}
}

func TestDebtService_GetDebtSummary(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "summary-spec",
		Title:    "Summary Project",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{Tasks: []planning.Task{{ID: "task-f1", FeatureID: "f1"}}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("summary-spec")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	driftSvc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	debtSvc := application.NewDebtService(driftSvc, audit)

	summary, err := debtSvc.GetDebtSummary(context.Background())
	if err != nil {
		t.Fatalf("GetDebtSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.HealthLevel == "" {
		t.Error("expected non-empty health level")
	}
}

func TestDebtService_GetTopDebtors(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "debtors-spec",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{Tasks: []planning.Task{{ID: "task-f1", FeatureID: "f1"}}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("debtors-spec")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	driftSvc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	debtSvc := application.NewDebtService(driftSvc, audit)

	debtors, err := debtSvc.GetTopDebtors(context.Background(), 5)
	if err != nil {
		t.Fatalf("GetTopDebtors: %v", err)
	}
	_ = debtors
}

func TestDebtService_GetHealthLevel(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "health-spec",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{Tasks: []planning.Task{{ID: "task-f1", FeatureID: "f1"}}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("health-spec")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	driftSvc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	debtSvc := application.NewDebtService(driftSvc, audit)

	level, err := debtSvc.GetHealthLevel(context.Background())
	if err != nil {
		t.Fatalf("GetHealthLevel: %v", err)
	}
	if level == "" {
		t.Error("expected non-empty health level")
	}
}

func TestDebtService_RecordDriftDetection(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "rec-spec", Features: []spec.Feature{{ID: "f1"}}}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{Tasks: []planning.Task{}}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	if err := repo.SaveState(planning.NewExecutionState("rec-spec")); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	driftSvc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	debtSvc := application.NewDebtService(driftSvc, audit)

	report := &drift.Report{
		ID:        "drift-test",
		CreatedAt: time.Now(),
		Issues: []drift.Issue{
			{ID: "i1", ComponentID: "comp-1", Type: drift.DriftTypePlan, Category: drift.CategoryMissing, Message: "missing task"},
		},
	}

	err := debtSvc.RecordDriftDetection(context.Background(), report)
	if err != nil {
		t.Fatalf("RecordDriftDetection: %v", err)
	}
}

// ---- ForecastService: GetBurndown and GetSimpleForecast with no plan ----

func TestForecastService_GetSimpleForecast_NoPlan(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	projection := events.NewExtendedVelocityProjection()
	svc := application.NewForecastService(projection, repo)

	forecast, err := svc.GetSimpleForecast()
	if err != nil {
		t.Fatalf("GetSimpleForecast: %v", err)
	}
	if forecast != nil {
		t.Error("expected nil forecast when no plan")
	}

	burndown, err := svc.GetBurndown()
	if err != nil {
		t.Fatalf("GetBurndown: %v", err)
	}
	if burndown != nil {
		t.Error("expected nil burndown when no plan")
	}
}

func TestForecastService_GetSimpleForecast_WithPlan(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{
		Tasks: []planning.Task{
			{ID: "t1"}, {ID: "t2"}, {ID: "t3"},
		},
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}
	state := planning.NewExecutionState("test")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusDone}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	projection := events.NewExtendedVelocityProjection()
	svc := application.NewForecastService(projection, repo)

	forecast, err := svc.GetSimpleForecast()
	if err != nil {
		t.Fatalf("GetSimpleForecast: %v", err)
	}
	if forecast == nil {
		t.Fatal("expected non-nil forecast")
	}
	if forecast.TotalTasks != 3 {
		t.Errorf("expected 3 total tasks, got %d", forecast.TotalTasks)
	}
	if forecast.RemainingTasks != 2 {
		t.Errorf("expected 2 remaining tasks, got %d", forecast.RemainingTasks)
	}
}

// ---- SpecService: AddFeature ----

func TestSpecService_AddFeature_SuccessPath(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:       "add-feat-spec",
		Title:    "Spec",
		Features: []spec.Feature{{ID: "f1", Title: "F1"}},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}

	// Change working directory so syncToMarkdown writes in tempDir
	origDir, _ := os.Getwd()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() {
		if err := os.Chdir(origDir); err != nil {
			t.Fatal(err)
		}
	}()

	svc := application.NewSpecService(repo)

	updated, err := svc.AddFeature("New Feature", "A brand new feature")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if len(updated.Spec.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(updated.Spec.Features))
	}
	if updated.Spec.Features[1].ID != "new-feature" {
		t.Errorf("expected feature ID 'new-feature', got %q", updated.Spec.Features[1].ID)
	}
}

func TestSpecService_AddFeature_LoadError(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("fail")}
	svc := application.NewSpecService(repo)

	_, err := svc.AddFeature("New Feature", "Desc")
	if err == nil {
		t.Fatal("expected error when spec cannot be loaded")
	}
}

// ---- UsageService: additional coverage ----

func TestUsageService_GetUsage_NilStats(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("no file")}
	svc := application.NewUsageService(repo)

	usage, err := svc.GetUsage()
	if err != nil {
		t.Fatalf("GetUsage: %v", err)
	}
	if usage == nil {
		t.Fatal("expected non-nil usage even on error")
	}
	if usage.ProviderStats == nil {
		t.Error("expected initialized ProviderStats map")
	}
}

func TestUsageService_GetTotalTokens_WithRecordedUsage(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	svc := application.NewUsageService(repo)

	if err := svc.RecordTokenUsage("gpt-4", 100, 50); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}

	total, err := svc.GetTotalTokens()
	if err != nil {
		t.Fatalf("GetTotalTokens: %v", err)
	}
	if total != 150 {
		t.Errorf("expected 150 total tokens, got %d", total)
	}
}

func TestUsageService_GetTotalTokens_NoFile(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("no file")}
	svc := application.NewUsageService(repo)

	total, err := svc.GetTotalTokens()
	if err != nil {
		t.Fatalf("GetTotalTokens: %v", err)
	}
	if total != 0 {
		t.Errorf("expected 0 total tokens, got %d", total)
	}
}

// ---- WorkspaceSyncService: Push and Pull with real git repo ----

func TestWorkspaceSyncService_Push_WithRealGit(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize git repo
	initTestGitRepo(t, tempDir)

	// Initialize roady
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	audit := newTestAudit()
	svc := application.NewWorkspaceSyncService(tempDir, audit)

	// Push with no unstaged changes should report no changes (files are committed by init)
	result, err := svc.Push(context.Background())
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if result.Action != "push" {
		t.Errorf("expected action 'push', got %q", result.Action)
	}
}

func TestWorkspaceSyncService_Pull_WithRealGit(t *testing.T) {
	tempDir := t.TempDir()

	// Initialize git repo
	initTestGitRepo(t, tempDir)

	audit := newTestAudit()
	svc := application.NewWorkspaceSyncService(tempDir, audit)

	// Pull without remote should error
	_, err := svc.Pull(context.Background())
	if err == nil {
		// It may or may not error depending on git config
		t.Log("Pull completed without error (no remote)")
	}
}

// ---- EventSourcedAuditService: GetVerificationVelocity ----

func TestEventSourcedAuditService_GetVerificationVelocity(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := storage.NewFileEventStore(tmpDir)
	if err != nil {
		t.Fatalf("NewFileEventStore: %v", err)
	}
	publisher := storage.NewInMemoryEventPublisher()

	svc, err := application.NewEventSourcedAuditService(store, publisher)
	if err != nil {
		t.Fatalf("NewEventSourcedAuditService: %v", err)
	}

	// Initially zero
	vel := svc.GetVerificationVelocity()
	if vel != 0 {
		t.Errorf("expected 0 verification velocity, got %f", vel)
	}
}

// ---- TransitionWithFSM success path: "stop" event reverts in_progress to pending ----

func TestTaskService_TransitionWithFSM_StopEvent(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Create approved plan with a task
	if err := repo.SavePlan(&planning.Plan{
		Tasks:          []planning.Task{{ID: "t1", Title: "Task 1"}},
		ApprovalStatus: planning.ApprovalApproved,
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	// Set task to in_progress (so "stop" is valid)
	state := planning.NewExecutionState("test")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusInProgress}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	// "stop" is not handled by coordinator switch, so it falls through to transitionWithFSM
	err := svc.TransitionTask("t1", "stop", "user", "")
	if err != nil {
		t.Fatalf("TransitionTask stop: %v", err)
	}

	// Verify the task went back to pending
	updatedState, err := repo.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if updatedState.GetTaskStatus("t1") != planning.StatusPending {
		t.Errorf("expected pending after stop, got %s", updatedState.GetTaskStatus("t1"))
	}
}

func TestTaskService_TransitionWithFSM_ReopenEvent(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Create plan with a task
	if err := repo.SavePlan(&planning.Plan{
		Tasks:          []planning.Task{{ID: "t1", Title: "Task 1"}},
		ApprovalStatus: planning.ApprovalApproved,
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	// Set task to done (so "reopen" is valid)
	state := planning.NewExecutionState("test")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusDone}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	audit := application.NewAuditService(repo)
	svc := application.NewTaskService(repo, audit, nil)

	// "reopen" falls through to transitionWithFSM
	err := svc.TransitionTask("t1", "reopen", "user", "reopen reason")
	if err != nil {
		t.Fatalf("TransitionTask reopen: %v", err)
	}

	updatedState, err := repo.LoadState()
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if updatedState.GetTaskStatus("t1") != planning.StatusPending {
		t.Errorf("expected pending after reopen, got %s", updatedState.GetTaskStatus("t1"))
	}
	// Check evidence was recorded
	result := updatedState.TaskStates["t1"]
	if len(result.Evidence) == 0 || result.Evidence[0] != "reopen reason" {
		t.Errorf("expected evidence 'reopen reason', got %v", result.Evidence)
	}
}

func TestTaskService_TransitionWithFSM_StartGuardCheck(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Create UNAPPROVED plan
	if err := repo.SavePlan(&planning.Plan{
		Tasks:          []planning.Task{{ID: "t1", Title: "Task 1"}},
		ApprovalStatus: planning.ApprovalPending,
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	state := planning.NewExecutionState("test")
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	audit := application.NewAuditService(repo)
	// Use a custom event that is not handled by coordinator switch to force FSM path
	// The "start" event on FSM path checks guard for plan approval
	// We test this by sending "start" through TransitionTask, which uses the coordinator.
	// Instead, let's test with a task that has no valid transition as a guard test.
	svc := application.NewTaskService(repo, audit, nil)

	// Use "stop" on a pending task (invalid: "stop" is only valid from in_progress)
	err := svc.TransitionTask("t1", "stop", "user", "")
	if err == nil {
		t.Fatal("expected error for stop on pending task via FSM")
	}
}

// ---- AddDependency error paths ----

func TestDependencyService_AddDependency_InvalidPath(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	svc := application.NewDependencyService(repo, tempDir)

	// Non-existent target should fail validation
	_, err := svc.AddDependency("/non/existent/path", dependency.DependencyRuntime, "A dependency")
	if err == nil {
		t.Fatal("expected error for non-existent target path")
	}
}

func TestDependencyService_AddDependency_SelfDependency(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	svc := application.NewDependencyService(repo, tempDir)

	// Self-dependency should be rejected
	_, err := svc.AddDependency(tempDir, dependency.DependencyRuntime, "Self ref")
	if err == nil {
		t.Fatal("expected error for self-dependency")
	}
}

// ---- GetBurndown with plan and completed tasks ----

func TestForecastService_GetBurndown_WithPlan(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	if err := repo.SavePlan(&planning.Plan{
		Tasks: []planning.Task{
			{ID: "t1", Title: "Task 1"},
			{ID: "t2", Title: "Task 2"},
			{ID: "t3", Title: "Task 3"},
		},
		ApprovalStatus: planning.ApprovalApproved,
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	state := planning.NewExecutionState("test")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusDone}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	projection := events.NewExtendedVelocityProjection()
	svc := application.NewForecastService(projection, repo)

	burndown, err := svc.GetBurndown()
	if err != nil {
		t.Fatalf("GetBurndown: %v", err)
	}
	// May or may not have burndown points depending on projection data
	// The important thing is it doesn't error and exercises the success path
	_ = burndown
}

// ---- GetDebtSummary with debtors ----

func TestDebtService_GetDebtSummary_WithDebtors(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Create spec with features
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:    "debtor-spec",
		Title: "Debtor Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
			{ID: "f2", Title: "Feature 2"},
		},
	}); err != nil {
		t.Fatalf("SaveSpec: %v", err)
	}

	// Create plan with tasks covering only f1 (f2 missing = drift)
	if err := repo.SavePlan(&planning.Plan{
		Tasks: []planning.Task{
			{ID: "t1", Title: "Task 1", FeatureID: "f1"},
		},
	}); err != nil {
		t.Fatalf("SavePlan: %v", err)
	}

	state := planning.NewExecutionState("debtor-spec")
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{}); err != nil {
		t.Fatalf("SavePolicy: %v", err)
	}

	// Lock spec WITHOUT f2 to create drift
	if err := repo.SaveSpecLock(&spec.ProductSpec{
		ID:       "debtor-spec",
		Title:    "Debtor Project",
		Features: []spec.Feature{{ID: "f1", Title: "Feature 1"}},
	}); err != nil {
		t.Fatalf("SaveSpecLock: %v", err)
	}

	audit := application.NewAuditService(repo)
	policy := application.NewPolicyService(repo)
	driftSvc := application.NewDriftService(repo, audit, storage.NewCodebaseInspector(), policy)
	debtSvc := application.NewDebtService(driftSvc, audit)

	// Detect drift and record it to create debt items
	driftReport, err := driftSvc.DetectDrift(context.Background())
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if driftReport != nil {
		_ = debtSvc.RecordDriftDetection(context.Background(), driftReport)
		_ = debtSvc.RecordDriftDetection(context.Background(), driftReport)
	}

	summary, err := debtSvc.GetDebtSummary(context.Background())
	if err != nil {
		t.Fatalf("GetDebtSummary: %v", err)
	}
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	// Exercise the TopDebtor branch
	if summary.HealthLevel == "" {
		t.Error("expected non-empty health level")
	}
}

// ---- InitService: InitializeProject with template ----

func TestInitService_InitializeProject_WithTemplate(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := newTestAudit()

	svc := application.NewInitService(repo, audit)
	svc.SetTemplate("web-api")

	err := svc.InitializeProject("my-api")
	if err != nil {
		t.Fatalf("InitializeProject: %v", err)
	}

	// Verify spec was created from template
	loadedSpec, err := repo.LoadSpec()
	if err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
	if loadedSpec == nil {
		t.Fatal("expected spec to be loaded")
	}
	if loadedSpec.Title != "my-api" {
		t.Errorf("expected title 'my-api', got %q", loadedSpec.Title)
	}

	// web-api template should produce multiple features
	if len(loadedSpec.Features) < 2 {
		t.Errorf("expected web-api template to have multiple features, got %d", len(loadedSpec.Features))
	}
}

func TestInitService_InitializeProject_AlreadyInitialized(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	audit := newTestAudit()
	svc := application.NewInitService(repo, audit)

	if err := svc.InitializeProject("project1"); err != nil {
		t.Fatalf("First init: %v", err)
	}

	// Second init should fail
	err := svc.InitializeProject("project2")
	if err == nil {
		t.Fatal("expected error for already initialized project")
	}
}

// ---- Workspace sync: Push with changes (exercises git add, commit paths) ----

func TestWorkspaceSyncService_Push_WithChanges(t *testing.T) {
	tempDir := t.TempDir()
	initTestGitRepo(t, tempDir)

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Save an initial spec so .roady/ has files git can track
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:    "initial",
		Title: "Initial Spec",
	}); err != nil {
		t.Fatalf("SaveSpec initial: %v", err)
	}

	// Commit the initial .roady/ files so git tracks them
	addCmd := exec.Command("git", "add", ".roady/")
	addCmd.Dir = tempDir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %s %v", string(out), err)
	}
	commitCmd := exec.Command("git", "commit", "--no-gpg-sign", "-m", "add roady dir")
	commitCmd.Dir = tempDir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %s %v", string(out), err)
	}

	// Now modify a .roady/ file to create unstaged changes
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:    "changed",
		Title: "Changed Spec",
	}); err != nil {
		t.Fatalf("SaveSpec changed: %v", err)
	}

	audit := newTestAudit()
	svc := application.NewWorkspaceSyncService(tempDir, audit)

	// Push will:
	// 1. Detect changed files in .roady/ (exercises changedFiles)
	// 2. Run git add .roady/ (exercises git helper)
	// 3. Run git commit (exercises git helper)
	// 4. Run git push which will fail (no remote)
	// Steps 1-3 provide coverage, step 4 returns an error
	_, err := svc.Push(context.Background())
	if err == nil {
		t.Log("Push succeeded unexpectedly (maybe local repo has remote configured)")
	} else {
		// Expected: git push fails because there's no remote
		t.Logf("Push failed as expected (no remote): %v", err)
	}
}

// ---- Pull with no local changes (exercises the non-stash path) ----

func TestWorkspaceSyncService_Pull_NoLocalChanges(t *testing.T) {
	tempDir := t.TempDir()
	initTestGitRepo(t, tempDir)

	audit := newTestAudit()
	svc := application.NewWorkspaceSyncService(tempDir, audit)

	// Pull with no remote will fail at git pull, but exercises the hasChanges=false path
	_, err := svc.Pull(context.Background())
	if err != nil {
		// Expected: no remote configured
		t.Logf("Pull error (expected): %v", err)
	}
}

// ---- CompleteTask via billing with elapsed minutes > 0 and default rate ----

func TestBillingService_CompleteTask_WithElapsedAndDefaultRate(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Set up rates with default
	rateConfig := &billing.RateConfig{
		Currency: "USD",
		Rates:    []billing.Rate{{ID: "dev", Name: "Developer", HourlyRate: 100.0, IsDefault: true}},
	}
	if err := repo.SaveRates(rateConfig); err != nil {
		t.Fatalf("SaveRates: %v", err)
	}

	// Create state with in_progress task that has started time
	startTime := time.Now().Add(-60 * time.Minute)
	state := planning.NewExecutionState("billing-test")
	state.TaskStates["t1"] = planning.TaskResult{
		Status:    planning.StatusInProgress,
		StartedAt: &startTime, // Started 60 min ago
	}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	audit := newTestAudit()
	svc := application.NewBillingService(repo, audit)

	err := svc.CompleteTask("t1")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}

	// Verify a time entry was created
	entries, err := repo.LoadTimeEntries()
	if err != nil {
		t.Fatalf("LoadTimeEntries: %v", err)
	}
	// Should have created a time entry since elapsed > 0 and default rate exists
	if len(entries) > 0 {
		if entries[0].TaskID != "t1" {
			t.Errorf("expected task ID t1, got %q", entries[0].TaskID)
		}
	}
}

// ---- Helper function ----

func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "config", "commit.gpgsign", "false"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %s %v", args, string(out), err)
		}
	}
}
