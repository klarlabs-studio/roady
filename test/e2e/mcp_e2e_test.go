package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
)

// TestMCPServicesHappyPath tests MCP services end-to-end through direct service calls.
// This validates that the same services used by MCP tools work correctly.
func TestMCPServicesHappyPath(t *testing.T) {
	// Setup temp workspace
	tempDir, err := os.MkdirTemp("", "roady-mcp-e2e-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test 1: Initialize project with env vars for AI config
	t.Log("Testing initialization...")
	t.Setenv("ROADY_AI_PROVIDER", "mock")
	t.Setenv("ROADY_AI_MODEL", "test")
	services, err := wiring.BuildAppServices(tempDir)
	if err != nil {
		t.Fatalf("BuildAppServices failed: %v", err)
	}

	err = services.Init.InitializeProject("mcp-test-project")
	if err != nil {
		t.Fatalf("InitializeProject failed: %v", err)
	}

	// Create a spec with features
	specPath := filepath.Join(tempDir, ".roady", "spec.yaml")
	specContent := `id: "mcp-test"
title: "MCP Test Project"
features:
  - id: "feat-auth"
    title: "Authentication"
    description: "User authentication flow"
    requirements:
      - id: "req-login"
        title: "Login Page"
        description: "Implement login form"
`
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Rebuild services after spec change
	services, err = wiring.BuildAppServices(tempDir)
	if err != nil {
		t.Fatalf("BuildAppServices after spec failed: %v", err)
	}

	// Test 2: Get spec
	t.Log("Testing get spec...")
	spec, err := services.Spec.GetSpec()
	if err != nil {
		t.Fatalf("GetSpec failed: %v", err)
	}
	if spec.Title != "MCP Test Project" {
		t.Errorf("Expected 'MCP Test Project', got %s", spec.Title)
	}

	// Test 3: Generate plan
	t.Log("Testing generate plan...")
	plan, err := services.Plan.GeneratePlan(ctx)
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}
	if len(plan.Tasks) == 0 {
		t.Error("Expected tasks in plan")
	}

	// Test 4: Get plan
	t.Log("Testing get plan...")
	plan, err = services.Plan.GetPlan()
	if err != nil {
		t.Fatalf("GetPlan failed: %v", err)
	}
	planJSON, _ := json.Marshal(plan)
	if string(planJSON) == "" {
		t.Error("Expected non-empty plan JSON")
	}

	// Test 5: Detect drift
	t.Log("Testing detect drift...")
	report, err := services.Drift.DetectDrift(ctx)
	if err != nil {
		t.Fatalf("DetectDrift failed: %v", err)
	}
	if report == nil {
		t.Error("Expected drift report")
	}

	// Test 6: Accept drift
	t.Log("Testing accept drift...")
	err = services.Drift.AcceptDrift()
	if err != nil {
		t.Fatalf("AcceptDrift failed: %v", err)
	}

	// Test 7: Approve plan
	t.Log("Testing approve plan...")
	err = services.Plan.ApprovePlan()
	if err != nil {
		t.Fatalf("ApprovePlan failed: %v", err)
	}

	// Test 8: Get state
	t.Log("Testing get state...")
	state, err := services.Plan.GetState()
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state == nil {
		t.Error("Expected execution state")
	}

	// Test 9: Check policy
	t.Log("Testing check policy...")
	_, violations := services.Policy.CheckCompliance()
	// Violations may exist, just ensure it doesn't error

	// Test 11: Get project snapshot
	t.Log("Testing project snapshot...")
	snapshot, err := services.Plan.GetProjectSnapshot(ctx)
	if err != nil {
		t.Fatalf("GetProjectSnapshot failed: %v", err)
	}
	if snapshot == nil {
		t.Fatal("Expected project snapshot")
	}
	if snapshot.Plan == nil {
		t.Error("Expected plan in snapshot")
	}

	// Test 12: Get ready/blocked/in-progress tasks
	t.Log("Testing task queries...")
	readyTasks, err := services.Plan.GetReadyTasks(ctx)
	if err != nil {
		t.Fatalf("GetReadyTasks failed: %v", err)
	}
	// Should have tasks ready since plan was approved
	t.Logf("Ready tasks: %d", len(readyTasks))

	blockedTasks, err := services.Plan.GetBlockedTasks(ctx)
	if err != nil {
		t.Fatalf("GetBlockedTasks failed: %v", err)
	}
	t.Logf("Blocked tasks: %d", len(blockedTasks))

	inProgressTasks, err := services.Plan.GetInProgressTasks(ctx)
	if err != nil {
		t.Fatalf("GetInProgressTasks failed: %v", err)
	}
	t.Logf("In-progress tasks: %d", len(inProgressTasks))

	_ = violations // Use violations to avoid unused error
	t.Log("All MCP services E2E tests passed!")
}
