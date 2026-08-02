package application_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestPlanService_FullCoverage(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-cov-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	// 1. Success Path
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "s1", Features: []spec.Feature{{ID: "f1", Title: "F1"}}})
	p, err := service.GeneratePlan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(p.Tasks))
	}

	// 2. Reconciliation Path
	if err := repo.SaveState(&planning.ExecutionState{
		TaskStates: map[string]planning.TaskResult{
			p.Tasks[0].ID: {Status: planning.StatusDone},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePlan(p); err != nil {
		t.Fatal(err)
	}

	// Add f2
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "s1", Features: []spec.Feature{{ID: "f1", Title: "F1"}, {ID: "f2", Title: "F2"}}})
	p2, _ := service.GeneratePlan(context.Background())
	state, _ := repo.LoadState()
	if len(p2.Tasks) != 2 || state.TaskStates[p2.Tasks[0].ID].Status != planning.StatusDone {
		t.Error("Reconciliation failed")
	}

	// 3. GetPlan
	gp, _ := service.GetPlan()
	if gp.ID != p2.ID {
		t.Error("GetPlan failed")
	}
}

func TestPlanService_FailurePaths(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("fail")}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	// 1. Load Spec Fail
	_, err := service.GeneratePlan(context.Background())
	if err == nil {
		t.Error("Expected error on spec load fail")
	}

	// 2. Reconcile Plan with Cycle
	repo.LoadError = nil
	repo.Spec = &spec.ProductSpec{ID: "s1"}
	tasks := []planning.Task{
		{ID: "t1", Title: "T1", DependsOn: []string{"t1"}},
	}
	_, _, err = service.UpdatePlan(tasks)
	if err == nil {
		t.Error("Expected error for DAG cycle")
	}
}

func TestPlanService_GeneratePlanLocksSpec(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-lock-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:      "s1",
		Title:   "Spec One",
		Version: "1.0.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}

	lock, err := repo.LoadSpecLock()
	if err != nil {
		t.Fatalf("load spec lock: %v", err)
	}
	if lock == nil {
		t.Fatal("expected spec lock to exist after plan generation")
	}
	if lock.Hash() != specDoc.Hash() {
		t.Fatalf("spec lock hash mismatch: got %s want %s", lock.Hash(), specDoc.Hash())
	}
}

func TestPlanService_UpdatePlanLocksSpec(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-update-lock-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:      "s1",
		Title:   "Spec One",
		Version: "1.0.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	_, _, err := service.UpdatePlan([]planning.Task{
		{ID: "task-f1", Title: "Implement Feature 1", FeatureID: "f1"},
	})
	if err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}

	lock, err := repo.LoadSpecLock()
	if err != nil {
		t.Fatalf("load spec lock: %v", err)
	}
	if lock == nil {
		t.Fatal("expected spec lock to exist after plan update")
	}
	if lock.Hash() != specDoc.Hash() {
		t.Fatalf("spec lock hash mismatch: got %s want %s", lock.Hash(), specDoc.Hash())
	}
}

func TestPlanService_ApproveRejectPlan(t *testing.T) {
	repo := &MockRepo{
		Plan: &planning.Plan{
			ID:             "p1",
			ApprovalStatus: planning.ApprovalPending,
		},
	}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("ApprovePlan failed: %v", err)
	}
	if repo.Plan.ApprovalStatus != planning.ApprovalApproved {
		t.Fatalf("expected approved status, got %s", repo.Plan.ApprovalStatus)
	}

	if err := service.RejectPlan(); err != nil {
		t.Fatalf("RejectPlan failed: %v", err)
	}
	if repo.Plan.ApprovalStatus != planning.ApprovalRejected {
		t.Fatalf("expected rejected status, got %s", repo.Plan.ApprovalStatus)
	}
}

func TestPlanService_PrunePlan(t *testing.T) {
	repo := &MockRepo{
		Spec: &spec.ProductSpec{
			ID: "s1",
			Features: []spec.Feature{
				{ID: "f1", Title: "Feature 1", Requirements: []spec.Requirement{{ID: "r1", Title: "Req 1"}}},
			},
		},
		Plan: &planning.Plan{
			ID: "p1",
			Tasks: []planning.Task{
				{ID: "task-r1", FeatureID: "f1", Title: "Valid Task"},
				{ID: "task-r2", FeatureID: "f2", Title: "Invalid Task"},
			},
		},
	}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	if err := service.PrunePlan(); err != nil {
		t.Fatalf("PrunePlan failed: %v", err)
	}
	if len(repo.Plan.Tasks) != 1 {
		t.Fatalf("expected 1 task after prune, got %d", len(repo.Plan.Tasks))
	}
	if repo.Plan.Tasks[0].ID != "task-r1" {
		t.Fatalf("expected task-r1 to remain, got %s", repo.Plan.Tasks[0].ID)
	}
}

func TestPlanService_GovernanceEventsFromManualTransitions(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-events-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	spec := &spec.ProductSpec{
		ID:    "gov-spec",
		Title: "Governance Spec",
		Features: []spec.Feature{
			{
				ID:    "f1",
				Title: "Feature 1",
				Requirements: []spec.Requirement{
					{ID: "r1", Title: "Req 1"},
				},
			},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-gov",
		SpecID:         spec.ID,
		ApprovalStatus: planning.ApprovalPending,
		Tasks: []planning.Task{
			{ID: "task-r1", FeatureID: "f1", Title: "Valid Task"},
			{ID: "task-orphan", FeatureID: "missing", Title: "Orphan Task"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("approve plan failed: %v", err)
	}
	if err := service.RejectPlan(); err != nil {
		t.Fatalf("reject plan failed: %v", err)
	}
	if err := service.PrunePlan(); err != nil {
		t.Fatalf("prune plan failed: %v", err)
	}

	events, err := repo.LoadEvents()
	if err != nil {
		t.Fatalf("load events: %v", err)
	}

	found := map[string]bool{}
	for _, ev := range events {
		found[ev.Action] = true
	}

	for _, want := range []string{"plan.approved", "plan.reject", "plan.prune"} {
		if !found[want] {
			t.Fatalf("expected governance event %s, got events: %+v", want, events)
		}
	}
}

func TestPlanService_GetStateUsage(t *testing.T) {
	repo := &MockRepo{
		State: planning.NewExecutionState("p1"),
	}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	if _, err := service.GetState(); err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if _, err := service.GetUsage(); err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}
}

func TestPlanService_GeneratePlanWithRequirements(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-req-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewPlanService(repo, application.NewAuditService(repo))

	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "spec-req",
		Features: []spec.Feature{
			{
				ID:    "feature-x",
				Title: "Feature X",
				Requirements: []spec.Requirement{
					{ID: "req-alpha", Title: "Alpha", Description: "Desc A"},
					{ID: "req-beta", Title: "Beta", Description: "Desc B", DependsOn: []string{"req-alpha"}},
				},
			},
			{
				ID:    "feature-y",
				Title: "Feature Y",
			},
		},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan, err := service.GeneratePlan(context.Background())
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}

	if len(plan.Tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(plan.Tasks))
	}

	var foundDependency bool
	for _, task := range plan.Tasks {
		if task.ID == "task-req-beta" {
			for _, dep := range task.DependsOn {
				if dep == "task-req-alpha" {
					foundDependency = true
				}
			}
		}
	}
	if !foundDependency {
		t.Fatal("expected req-beta to depend on req-alpha")
	}
}

func TestPlanService_PropagatesSourceFromSpec(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-source-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewPlanService(repo, application.NewAuditService(repo))

	featureSource := spec.Source{Doc: "docs/auth.md", Line: 12}
	requirementSource := spec.Source{Doc: "docs/auth.md", Line: 18}

	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "spec-source",
		Features: []spec.Feature{
			{
				ID:     "auth",
				Title:  "Auth",
				Source: featureSource,
				Requirements: []spec.Requirement{
					// First requirement carries its own source.
					{ID: "auth-signup", Title: "Sign up", Source: requirementSource},
					// Second requirement falls back to feature source.
					{ID: "auth-login", Title: "Log in"},
				},
			},
			// Empty-feature fallback path.
			{ID: "stub", Title: "Stub", Source: spec.Source{Doc: "docs/stub.md", Line: 3}},
		},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan, err := service.GeneratePlan(context.Background())
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	want := map[string]planning.TaskSource{
		"task-auth-signup": {Doc: "docs/auth.md", Line: 18},
		"task-auth-login":  {Doc: "docs/auth.md", Line: 12},
		"task-stub":        {Doc: "docs/stub.md", Line: 3},
	}

	for _, task := range plan.Tasks {
		expected, ok := want[task.ID]
		if !ok {
			continue
		}
		if task.Source != expected {
			t.Errorf("task %q source = %+v, want %+v", task.ID, task.Source, expected)
		}
	}
}

func TestPlanService_UpdatePlanKeepsOrphans(t *testing.T) {
	repo := &MockRepo{
		Spec: &spec.ProductSpec{
			ID: "spec-1",
			Features: []spec.Feature{
				{ID: "feature-1", Title: "Feature"},
			},
		},
		Plan: &planning.Plan{
			ID: "plan-old",
			Tasks: []planning.Task{
				{ID: "task-orphan", Title: "Orphan", FeatureID: "feature-1"},
			},
		},
	}
	service := application.NewPlanService(repo, application.NewAuditService(repo))

	updated, _, err := service.UpdatePlan([]planning.Task{
		{ID: "task-new", Title: "New Task", FeatureID: "feature-1"},
	})
	if err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}

	if len(updated.Tasks) != 2 {
		t.Fatalf("expected 2 tasks after reconciliation, got %d", len(updated.Tasks))
	}
	foundOrphan := false
	for _, task := range updated.Tasks {
		if task.ID == "task-orphan" {
			foundOrphan = true
		}
	}
	if !foundOrphan {
		t.Fatal("orphan task was dropped")
	}
}

func TestPlanService_GovernanceEvents(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-events-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	spec := &spec.ProductSpec{
		ID:    "gov",
		Title: "Governance Project",
		Features: []spec.Feature{
			{
				ID:    "feature-gov",
				Title: "Governance Feature",
				Requirements: []spec.Requirement{
					{ID: "req-gov", Title: "Governance req", Description: "desc"},
				},
			},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("generate plan failed: %v", err)
	}
	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("approve plan failed: %v", err)
	}
	if err := service.RejectPlan(); err != nil {
		t.Fatalf("reject plan failed: %v", err)
	}
	if _, _, err := service.UpdatePlan([]planning.Task{
		{ID: "task-update", Title: "Updated", FeatureID: "feature-gov"},
	}); err != nil {
		t.Fatalf("update plan failed: %v", err)
	}
	if err := service.PrunePlan(); err != nil {
		t.Fatalf("prune plan failed: %v", err)
	}

	events, err := repo.LoadEvents()
	if err != nil {
		t.Fatalf("load events: %v", err)
	}

	actionSet := make(map[string]bool)
	for _, ev := range events {
		actionSet[ev.Action] = true
	}

	for _, want := range []string{"plan.generate", "plan.update_smart", "plan.approved", "plan.reject", "plan.prune"} {
		if !actionSet[want] {
			t.Fatalf("expected event %s recorded, got actions %v", want, events)
		}
	}
}

func TestPlanService_GettersReturnStoredValues(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-getters-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	spec := &spec.ProductSpec{
		ID:    "getter-spec",
		Title: "Getter Project",
		Features: []spec.Feature{
			{ID: "feature-x", Title: "Feature X"},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-get",
		SpecID:         spec.ID,
		ApprovalStatus: planning.ApprovalPending,
		Tasks: []planning.Task{
			{ID: "task-get-1", FeatureID: "feature-x", Title: "Getter Task"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState(plan.ID)
	state.TaskStates["task-get-1"] = planning.TaskResult{Status: planning.StatusPending}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	usage := domain.UsageStats{TotalCommands: 7}
	if err := repo.UpdateUsage(usage); err != nil {
		t.Fatalf("update usage: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	gotPlan, err := service.GetPlan()
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	if gotPlan == nil || gotPlan.ID != plan.ID {
		t.Fatalf("unexpected plan: %+v", gotPlan)
	}

	gotState, err := service.GetState()
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if gotState == nil || len(gotState.TaskStates) != 1 {
		t.Fatalf("unexpected state: %+v", gotState)
	}

	gotUsage, err := service.GetUsage()
	if err != nil {
		t.Fatalf("GetUsage failed: %v", err)
	}
	if gotUsage.TotalCommands != usage.TotalCommands {
		t.Fatalf("unexpected usage: %+v", gotUsage)
	}
}

func TestPlanService_ApproveRejectErrorsWithoutPlan(t *testing.T) {
	repo := &MockRepo{}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	if err := service.ApprovePlan(); err == nil || !strings.Contains(err.Error(), "no plan found to approve") {
		t.Fatalf("expected approval error when plan missing, got %v", err)
	}

	if err := service.RejectPlan(); err == nil || !strings.Contains(err.Error(), "no plan found to reject") {
		t.Fatalf("expected rejection error when plan missing, got %v", err)
	}
}

func TestPlanService_GettersLoadErrors(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("boom")}
	service := application.NewPlanService(repo, application.NewAuditService(repo))

	if _, err := service.GetPlan(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected plan load error, got %v", err)
	}
	if _, err := service.GetState(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected state load error, got %v", err)
	}
	if _, err := service.GetUsage(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected usage load error, got %v", err)
	}
}

func TestPlanService_PrunePlanSpecLoadFails(t *testing.T) {
	repo := &MockRepo{LoadError: errors.New("boom")}
	service := application.NewPlanService(repo, application.NewAuditService(repo))

	if err := service.PrunePlan(); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected prune error when spec load fails, got %v", err)
	}
}

func TestPlanService_UpdatePlanFiltersInvalidTasks(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-filter-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:    "filter-spec",
		Title: "Filter",
		Features: []spec.Feature{
			{ID: "feature-filter", Title: "Filter Feature"},
		},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	tasks := []planning.Task{
		{ID: "", FeatureID: "feature-filter", Title: "Missing ID"},
		{ID: "task-valid", FeatureID: "feature-filter", Title: "Valid"},
		{ID: "task-nofeature", Title: "No Feature"},
	}
	plan, _, err := service.UpdatePlan(tasks)
	if err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}
	if len(plan.Tasks) != 2 {
		t.Fatalf("expected 2 tasks after filtering, got %d", len(plan.Tasks))
	}
	for _, task := range plan.Tasks {
		if task.ID == "" || task.Title == "" {
			t.Fatalf("unexpected invalid task in plan: %+v", task)
		}
	}
}

func TestPlanService_GetCoordinator(t *testing.T) {
	repo := &MockRepo{}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	coord := service.GetCoordinator()
	if coord == nil {
		t.Fatal("expected GetCoordinator to return non-nil coordinator")
	}
}

func TestPlanService_GetProjectSnapshot(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-snapshot-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	// Save spec, generate plan, approve it, then get snapshot
	specDoc := &spec.ProductSpec{
		ID:    "snap-spec",
		Title: "Snapshot Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
			{ID: "f2", Title: "Feature 2"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	snapshot, err := service.GetProjectSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetProjectSnapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snapshot.Plan == nil {
		t.Fatal("expected snapshot to contain plan")
	}
}

func TestPlanService_GetProjectSnapshot_NilContext(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-snap-nilctx-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:    "nilctx-spec",
		Title: "Nil Context",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}
	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}

	// Pass nil context; the method should handle it gracefully
	snapshot, err := service.GetProjectSnapshot(context.TODO())
	if err != nil {
		t.Fatalf("GetProjectSnapshot with nil ctx: %v", err)
	}
	if snapshot == nil {
		t.Fatal("expected non-nil snapshot with nil context")
	}
}

func TestPlanService_GetTaskSummaries(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-summaries-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:    "sum-spec",
		Title: "Summary Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
			{ID: "f2", Title: "Feature 2"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	summaries, err := service.GetTaskSummaries(context.Background())
	if err != nil {
		t.Fatalf("GetTaskSummaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 task summaries, got %d", len(summaries))
	}
}

func TestPlanService_GetReadyTasks(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-ready-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:    "ready-spec",
		Title: "Ready Tasks Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	ready, err := service.GetReadyTasks(context.Background())
	if err != nil {
		t.Fatalf("GetReadyTasks: %v", err)
	}
	// After approval with no dependencies, all tasks should be ready
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready task, got %d", len(ready))
	}
}

func TestPlanService_GetBlockedTasks(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-blocked-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:    "blocked-spec",
		Title: "Blocked Tasks Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if _, err := service.GeneratePlan(context.Background()); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	blocked, err := service.GetBlockedTasks(context.Background())
	if err != nil {
		t.Fatalf("GetBlockedTasks: %v", err)
	}
	// With no dependencies, no tasks should be blocked
	if len(blocked) != 0 {
		t.Fatalf("expected 0 blocked tasks, got %d", len(blocked))
	}
}

func TestPlanService_GetInProgressTasks(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-inprog-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	specDoc := &spec.ProductSpec{
		ID:    "inprog-spec",
		Title: "InProgress Tasks Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specDoc); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan, err := service.GeneratePlan(context.Background())
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if err := service.ApprovePlan(); err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}

	// Start a task through the coordinator to move it to in_progress
	coord := service.GetCoordinator()
	if err := coord.StartTask(context.Background(), plan.Tasks[0].ID, "alice", ""); err != nil {
		t.Fatalf("StartTask: %v", err)
	}

	inProgress, err := service.GetInProgressTasks(context.Background())
	if err != nil {
		t.Fatalf("GetInProgressTasks: %v", err)
	}
	if len(inProgress) != 1 {
		t.Fatalf("expected 1 in-progress task, got %d", len(inProgress))
	}
	if inProgress[0].ID != plan.Tasks[0].ID {
		t.Fatalf("expected in-progress task ID %s, got %s", plan.Tasks[0].ID, inProgress[0].ID)
	}
}

func TestPlanService_ReconcilePlanKeepsOrphans(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-plan-orphans-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	audit := application.NewAuditService(repo)
	service := application.NewPlanService(repo, audit)

	spec := &spec.ProductSpec{
		ID:    "orphan-spec",
		Title: "Orphan",
		Features: []spec.Feature{
			{ID: "f1", Title: "F1"},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:     "orphan-plan",
		SpecID: spec.ID,
		Tasks: []planning.Task{
			{ID: "task-orphan", FeatureID: "legacy", Title: "Orphan Task"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if _, _, err := service.UpdatePlan([]planning.Task{
		{ID: "task-new", FeatureID: "f1", Title: "New Task"},
	}); err != nil {
		t.Fatalf("UpdatePlan failed: %v", err)
	}

	updated, err := repo.LoadPlan()
	if err != nil {
		t.Fatalf("load plan: %v", err)
	}
	foundOrphan := false
	for _, tk := range updated.Tasks {
		if tk.ID == "task-orphan" {
			foundOrphan = true
			break
		}
	}
	if !foundOrphan {
		t.Fatalf("orphan task was dropped from plan: %+v", updated.Tasks)
	}
}
