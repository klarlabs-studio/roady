package mcp

import (
	"context"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func setupCoordinatorTestServer(t *testing.T) *Server {
	t.Helper()

	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := initMockAIConfig(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	sp := &spec.ProductSpec{
		ID:    "coord-spec",
		Title: "Coordinator Test",
		Features: []spec.Feature{
			{ID: "feat-1", Title: "Feature 1"},
		},
	}
	if err := repo.SaveSpec(sp); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-coord",
		SpecID:         sp.ID,
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "t1", Title: "Ready Task", FeatureID: "feat-1", Priority: planning.PriorityHigh},
			{ID: "t2", Title: "Blocked Task", FeatureID: "feat-1", Priority: planning.PriorityMedium, DependsOn: []string{"t1"}},
			{ID: "t3", Title: "In Progress Task", FeatureID: "feat-1", Priority: planning.PriorityLow},
			{ID: "t4", Title: "Done Task", FeatureID: "feat-1"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState(plan.ID)
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusBlocked}
	state.TaskStates["t3"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["t4"] = planning.TaskResult{Status: planning.StatusDone}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 5, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	return server
}

func TestHandleGetSnapshot(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	result, err := server.handleGetSnapshot(ctx, GetSnapshotArgs{})
	if err != nil {
		t.Fatalf("handleGetSnapshot: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleGetReadyTasks(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	result, err := server.handleGetReadyTasks(ctx, GetReadyTasksArgs{})
	if err != nil {
		t.Fatalf("handleGetReadyTasks: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleGetBlockedTasks(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	result, err := server.handleGetBlockedTasks(ctx, GetBlockedTasksArgs{})
	if err != nil {
		t.Fatalf("handleGetBlockedTasks: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleGetInProgressTasks(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	result, err := server.handleGetInProgressTasks(ctx, GetInProgressTasksArgs{})
	if err != nil {
		t.Fatalf("handleGetInProgressTasks: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestHandleTasks_StatusDispatch(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	cases := []string{"", "ready", "in_progress", "blocked"}
	for _, status := range cases {
		t.Run("status="+status, func(t *testing.T) {
			result, err := server.handleTasks(ctx, TasksArgs{Status: status})
			if err != nil {
				t.Fatalf("handleTasks(%q): %v", status, err)
			}
			if result == nil {
				t.Fatalf("handleTasks(%q): nil result", status)
			}
		})
	}
}

func TestHandleTasks_All(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	result, err := server.handleTasks(ctx, TasksArgs{Status: "all"})
	if err != nil {
		t.Fatalf("handleTasks(all): %v", err)
	}
	bucket, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	for _, key := range []string{"ready", "in_progress", "blocked"} {
		if _, ok := bucket[key]; !ok {
			t.Errorf("expected %q key in result, missing", key)
		}
	}
}

func TestHandleTasks_InvalidStatus(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	if _, err := server.handleTasks(context.Background(), TasksArgs{Status: "bogus"}); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestServer_RegistersCanonicalAndDeprecatedToolNames(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	tools := server.mcpServer.Tools()

	registered := make(map[string]bool, len(tools))
	for _, tool := range tools {
		registered[tool.Name] = true
	}

	expected := []string{
		// Canonical task-listing tool plus its three deprecation aliases.
		"roady_tasks",
		// Canonical decompose + deprecation alias.
		"roady_plan_decompose",
		// Canonical recurring-drift + deprecation alias.
		"roady_drift_recurring",
		// Cost estimator (new in v0.10).
		"roady_cost_estimate",
	}

	for _, name := range expected {
		if !registered[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

func TestHandleCostEstimate_DefaultsAndDispatch(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	t.Run("default_operation", func(t *testing.T) {
		result, err := server.handleCostEstimate(ctx, CostEstimateArgs{})
		if err != nil {
			t.Fatalf("handleCostEstimate: %v", err)
		}
		if result == nil {
			t.Fatal("expected non-nil estimate")
		}
	})

	t.Run("explicit_operation", func(t *testing.T) {
		for _, op := range []string{"generate_plan", "smart_decompose", "review_spec", "explain_drift", "query"} {
			if _, err := server.handleCostEstimate(ctx, CostEstimateArgs{Operation: op}); err != nil {
				t.Errorf("handleCostEstimate(%q): %v", op, err)
			}
		}
	})

	t.Run("invalid_operation", func(t *testing.T) {
		if _, err := server.handleCostEstimate(ctx, CostEstimateArgs{Operation: "bogus"}); err == nil {
			t.Fatal("expected error for unknown operation")
		}
	})
}
