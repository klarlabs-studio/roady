package mcp

import (
	"context"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestServer_HandleTransitionalTools(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(tempDir); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	spec := &spec.ProductSpec{
		ID:    "mcp",
		Title: "MCP Project",
		Features: []spec.Feature{
			{ID: "feature-1", Title: "Feature 1", Requirements: []spec.Requirement{{ID: "req-1", Title: "Req 1"}}},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}
	plan := &planning.Plan{
		ID:             "plan-1",
		SpecID:         spec.ID,
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-req-1", Title: "Task 1", FeatureID: "feature-1"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{AllowAI: true, MaxWIP: 2}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	if err := repo.UpdateUsage(domain.UsageStats{}); err != nil {
		t.Fatalf("update usage: %v", err)
	}

	server, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx := context.Background()

	if _, err := server.handleGetUsage(ctx, GetUsageArgs{}); err != nil {
		t.Fatalf("handleGetUsage failed: %v", err)
	}

	if _, err := server.handleGetPlan(ctx, GetPlanArgs{}); err != nil {
		t.Fatalf("handleGetPlan failed: %v", err)
	}

	if _, err := server.handleGetState(ctx, GetStateArgs{}); err != nil {
		t.Fatalf("handleGetState failed: %v", err)
	}

	if _, err := server.handleTransitionTask(ctx, TransitionTaskArgs{
		TaskID:   "task-req-1",
		Event:    "start",
		Evidence: "test",
	}); err != nil {
		t.Fatalf("handleTransitionTask failed: %v", err)
	}

	if _, err := server.handleCheckPolicy(ctx, CheckPolicyArgs{}); err != nil {
		t.Fatalf("handleCheckPolicy failed: %v", err)
	}

	if _, err := server.handleDetectDrift(ctx, DetectDriftArgs{}); err != nil {
		t.Fatalf("handleDetectDrift failed: %v", err)
	}

	if _, err := server.handleAcceptDrift(ctx, AcceptDriftArgs{}); err != nil {
		t.Fatalf("handleAcceptDrift failed: %v", err)
	}
}

func TestServerHandleStatusCounts(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	spec := &spec.ProductSpec{
		ID:    "status-spec",
		Title: "Status Spec",
		Features: []spec.Feature{
			{ID: "feat-a", Title: "Feature A"},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-status",
		SpecID:         spec.ID,
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-done", FeatureID: "feat-a", Title: "Done Task"},
			{ID: "task-progress", FeatureID: "feat-a", Title: "In Progress Task"},
			{ID: "task-pending", FeatureID: "feat-a", Title: "Pending Task"},
			{ID: "task-blocked", FeatureID: "feat-a", Title: "Blocked Task"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState(plan.ID)
	state.TaskStates["task-done"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["task-progress"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["task-pending"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["task-blocked"] = planning.TaskResult{Status: planning.StatusBlocked}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	result, err := server.handleStatus(context.Background(), StatusArgs{})
	if err != nil {
		t.Fatalf("handleStatus failed: %v", err)
	}

	status, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}

	for _, want := range []string{"- Done: 1", "- In Progress: 1", "- Pending: 1", "- Blocked: 1"} {
		if !strings.Contains(status, want) {
			t.Fatalf("expected %q in status summary, got:\n%s", want, status)
		}
	}
}

func TestServerHandleStatusFiltering(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	spec := &spec.ProductSpec{
		ID:    "filter-spec",
		Title: "Filter Test Spec",
		Features: []spec.Feature{
			{ID: "feat-a", Title: "Feature A"},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-filter",
		SpecID:         spec.ID,
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-high-pending", FeatureID: "feat-a", Title: "High Pending", Priority: planning.PriorityHigh},
			{ID: "task-low-done", FeatureID: "feat-a", Title: "Low Done", Priority: planning.PriorityLow},
			{ID: "task-medium-progress", FeatureID: "feat-a", Title: "Medium Progress", Priority: planning.PriorityMedium},
			{ID: "task-high-blocked", FeatureID: "feat-a", Title: "High Blocked", Priority: planning.PriorityHigh},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState(plan.ID)
	state.TaskStates["task-high-pending"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["task-low-done"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["task-medium-progress"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["task-high-blocked"] = planning.TaskResult{Status: planning.StatusBlocked}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Test filter by status
	t.Run("FilterByStatus", func(t *testing.T) {
		result, err := server.handleStatus(context.Background(), StatusArgs{Status: "pending"})
		if err != nil {
			t.Fatalf("handleStatus failed: %v", err)
		}
		status := result.(string)
		if !strings.Contains(status, "Filtered Tasks: 1") {
			t.Errorf("expected 1 filtered task, got:\n%s", status)
		}
		if !strings.Contains(status, "High Pending") {
			t.Errorf("expected pending task in output, got:\n%s", status)
		}
	})

	// Test filter by priority
	t.Run("FilterByPriority", func(t *testing.T) {
		result, err := server.handleStatus(context.Background(), StatusArgs{Priority: "high"})
		if err != nil {
			t.Fatalf("handleStatus failed: %v", err)
		}
		status := result.(string)
		if !strings.Contains(status, "Filtered Tasks: 2") {
			t.Errorf("expected 2 filtered tasks (high priority), got:\n%s", status)
		}
	})

	// Test blocked flag
	t.Run("BlockedFlag", func(t *testing.T) {
		result, err := server.handleStatus(context.Background(), StatusArgs{Blocked: true})
		if err != nil {
			t.Fatalf("handleStatus failed: %v", err)
		}
		status := result.(string)
		if !strings.Contains(status, "Filtered Tasks: 1") {
			t.Errorf("expected 1 blocked task, got:\n%s", status)
		}
		if !strings.Contains(status, "High Blocked") {
			t.Errorf("expected blocked task in output, got:\n%s", status)
		}
	})

	// Test active flag
	t.Run("ActiveFlag", func(t *testing.T) {
		result, err := server.handleStatus(context.Background(), StatusArgs{Active: true})
		if err != nil {
			t.Fatalf("handleStatus failed: %v", err)
		}
		status := result.(string)
		if !strings.Contains(status, "Filtered Tasks: 1") {
			t.Errorf("expected 1 active task, got:\n%s", status)
		}
		if !strings.Contains(status, "Medium Progress") {
			t.Errorf("expected in_progress task in output, got:\n%s", status)
		}
	})

	// Test limit
	t.Run("Limit", func(t *testing.T) {
		result, err := server.handleStatus(context.Background(), StatusArgs{Status: "pending,blocked", Limit: 1})
		if err != nil {
			t.Fatalf("handleStatus failed: %v", err)
		}
		status := result.(string)
		if !strings.Contains(status, "Filtered Tasks: 1") {
			t.Errorf("expected 1 task with limit, got:\n%s", status)
		}
	})

	// Test JSON output
	t.Run("JSONOutput", func(t *testing.T) {
		result, err := server.handleStatus(context.Background(), StatusArgs{JSON: true, Status: "in_progress"})
		if err != nil {
			t.Fatalf("handleStatus failed: %v", err)
		}
		jsonStr := result.(string)
		if !strings.Contains(jsonStr, `"filtered_count":1`) {
			t.Errorf("expected JSON with 1 filtered task, got:\n%s", jsonStr)
		}
		if !strings.Contains(jsonStr, `"status":"in_progress"`) {
			t.Errorf("expected in_progress status in JSON, got:\n%s", jsonStr)
		}
	})
}

func TestServerHandleCheckPolicyViolations(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	spec := &spec.ProductSpec{
		ID:    "policy-spec",
		Title: "Policy Project",
		Features: []spec.Feature{
			{
				ID:    "feature-pol",
				Title: "Policy Feature",
				Requirements: []spec.Requirement{
					{ID: "req-pol", Title: "Req", Description: "Desc"},
				},
			},
		},
	}
	if err := repo.SaveSpec(spec); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-policy",
		SpecID:         spec.ID,
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-a", FeatureID: "feature-pol", Title: "Task A"},
			{ID: "task-b", FeatureID: "feature-pol", Title: "Task B"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState(plan.ID)
	state.TaskStates["task-a"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["task-b"] = planning.TaskResult{Status: planning.StatusInProgress}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 1, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	result, err := server.handleCheckPolicy(context.Background(), CheckPolicyArgs{})
	if err != nil {
		t.Fatalf("handleCheckPolicy failed: %v", err)
	}

	violations, ok := result.([]policy.Violation)
	if !ok {
		t.Fatalf("expected []policy.Violation, got %T", result)
	}
	if len(violations) == 0 {
		t.Fatalf("expected violations, got none")
	}
}

func TestServerHandleDepsList(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "deps-spec",
		Title: "Deps Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	result, err := server.handleDepsList(context.Background(), GetSpecArgs{})
	if err != nil {
		t.Fatalf("handleDepsList failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestServerHandleDepsScan(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "scan-spec",
		Title: "Scan Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	result, err := server.handleDepsScan(context.Background(), GetSpecArgs{})
	if err != nil {
		t.Fatalf("handleDepsScan failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil scan result")
	}
}

func TestServerHandleDepsGraph(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "graph-spec",
		Title: "Graph Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Test without cycle check
	result, err := server.handleDepsGraph(context.Background(), DepsGraphArgs{CheckCycles: false})
	if err != nil {
		t.Fatalf("handleDepsGraph failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Test with cycle check
	result, err = server.handleDepsGraph(context.Background(), DepsGraphArgs{CheckCycles: true})
	if err != nil {
		t.Fatalf("handleDepsGraph with cycle check failed: %v", err)
	}

	resultMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if _, hasCycle := resultMap["has_cycle"]; !hasCycle {
		t.Fatal("expected has_cycle in result when CheckCycles is true")
	}
}

func TestServerHandleDebtReport(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "debt-spec",
		Title: "Debt Test Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	result, err := server.handleDebtReport(context.Background(), GetSpecArgs{})
	if err != nil {
		t.Fatalf("handleDebtReport failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestServerHandleDebtSummary(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "debt-summary-spec",
		Title: "Debt Summary Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	result, err := server.handleDebtSummary(context.Background(), GetSpecArgs{})
	if err != nil {
		t.Fatalf("handleDebtSummary failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestServerHandleStickyDrift(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "sticky-spec",
		Title: "Sticky Drift Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	result, err := server.handleStickyDrift(context.Background(), GetSpecArgs{})
	if err != nil {
		t.Fatalf("handleStickyDrift failed: %v", err)
	}

	// Result can be nil or empty slice if no sticky items
	_ = result
}

func TestServerHandleDebtTrend(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specData := &spec.ProductSpec{
		ID:    "trend-spec",
		Title: "Trend Analysis Project",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(specData); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 2, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Test with default days
	result, err := server.handleDebtTrend(context.Background(), DebtTrendArgs{})
	if err != nil {
		t.Fatalf("handleDebtTrend failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Test with custom days
	result, err = server.handleDebtTrend(context.Background(), DebtTrendArgs{Days: 14})
	if err != nil {
		t.Fatalf("handleDebtTrend with custom days failed: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result with custom days")
	}
}
