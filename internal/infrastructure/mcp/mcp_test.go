package mcp

import (
	"context"
	"os"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestServer_Handlers(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-mcp-handlers-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	// Set env vars for AI config instead of creating .roady directory
	// so handleInit can still create the project
	t.Setenv("ROADY_AI_PROVIDER", "mock")
	t.Setenv("ROADY_AI_MODEL", "test")

	s, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// 1. HandleInit
	resp, err := s.handleInit(context.Background(), InitArgs{Name: "test"})
	if err != nil {
		t.Errorf("handleInit failed: %v", err)
	}
	if resp != "Project test initialized successfully" {
		t.Errorf("unexpected response: %v", resp)
	}

	// 1.1 HandleInit Error (empty name)
	res, err := s.handleInit(context.Background(), InitArgs{Name: ""})
	assertToolError(t, res, err, "")

	// 2. HandleGeneratePlan
	_, err = s.handleGeneratePlan(context.Background(), GeneratePlanArgs{})
	if err != nil {
		t.Errorf("handleGeneratePlan failed: %v", err)
	}

	// 3. HandleGetPlan
	_, err = s.handleGetPlan(context.Background(), GetPlanArgs{})
	if err != nil {
		t.Errorf("handleGetPlan failed: %v", err)
	}

	// 3.1 HandleGetSpec
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "s1", Title: "S1"})
	_, err = s.handleGetSpec(context.Background(), GetSpecArgs{})
	if err != nil {
		t.Errorf("handleGetSpec failed: %v", err)
	}

	// 4. HandleUpdatePlan
	_, err = s.handleUpdatePlan(context.Background(), UpdatePlanArgs{
		Tasks: []planning.Task{
			{ID: "t1", Title: "T1", FeatureID: "f1"},
			{ID: "t2", Title: "T2", FeatureID: "f1"},
		},
	})
	if err != nil {
		t.Errorf("handleUpdatePlan failed: %v", err)
	}

	// 4.1 HandleUpdatePlan empty
	_, err = s.handleUpdatePlan(context.Background(), UpdatePlanArgs{Tasks: []planning.Task{}})
	if err != nil {
		t.Errorf("handleUpdatePlan empty failed: %v", err)
	}

	// 5. HandleDetectDrift
	repo = storage.NewFilesystemRepository(tempDir)
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "s1", Title: "S1", Features: []spec.Feature{{ID: "f1"}}})
	// Provide plan to avoid drift or show it
	_ = repo.SavePlan(&planning.Plan{Tasks: []planning.Task{{ID: "task-f1", FeatureID: "f1"}}})

	_, err = s.handleDetectDrift(context.Background(), DetectDriftArgs{})
	if err != nil {
		t.Errorf("handleDetectDrift failed: %v", err)
	}

	// 6. HandleStatus
	_ = repo.SavePlan(&planning.Plan{Tasks: []planning.Task{
		{ID: "t1"},
		{ID: "t2"},
	}})
	_ = repo.SaveState(&planning.ExecutionState{
		TaskStates: map[string]planning.TaskResult{
			"t1": {Status: planning.StatusInProgress},
			"t2": {Status: planning.StatusDone},
		},
	})
	_, err = s.handleStatus(context.Background(), StatusArgs{})
	if err != nil {
		t.Errorf("handleStatus failed: %v", err)
	}

	// 7. HandleCheckPolicy
	// Success path
	_ = repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 10, AllowAI: true})
	_, err = s.handleCheckPolicy(context.Background(), CheckPolicyArgs{})
	if err != nil {
		t.Errorf("handleCheckPolicy failed: %v", err)
	}

	// Force violation
	_ = repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 1, AllowAI: true})
	_, err = s.handleCheckPolicy(context.Background(), CheckPolicyArgs{})
	if err != nil {
		t.Errorf("handleCheckPolicy failed: %v", err)
	}

	// 8. Error cases (restricted dir)
	_ = os.Chmod(tempDir+"/.roady", 0000)
	defer func() { _ = os.Chmod(tempDir+"/.roady", 0700) }()

	res, err = s.handleGetPlan(context.Background(), GetPlanArgs{})
	assertToolError(t, res, err, "")

	// 8.1 HandleGeneratePlan missing spec
	tempEmpty2, _ := os.MkdirTemp("", "roady-mcp-empty-*")
	defer func() { _ = os.RemoveAll(tempEmpty2) }()
	s2, err := NewServer(tempEmpty2)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	res2, err2 := s2.handleGeneratePlan(context.Background(), GeneratePlanArgs{})
	assertToolError(t, res2, err2, "")

	// 8.1.1 HandleGeneratePlan error (restricted dir)
	res, err = s.handleGeneratePlan(context.Background(), GeneratePlanArgs{})
	assertToolError(t, res, err, "")

	// 8.2.1 HandleGetSpec error (restricted dir)
	res, err = s.handleGetSpec(context.Background(), GetSpecArgs{})
	assertToolError(t, res, err, "")

	// 8.3 HandleUpdatePlan error (cycle)

	res, err = s.handleUpdatePlan(context.Background(), UpdatePlanArgs{

		Tasks: []planning.Task{{ID: "t1", DependsOn: []string{"t1"}}},
	})

	assertToolError(t, res, err, "")

	// 8.3.1 HandleUpdatePlan error (restricted dir)

	res, err = s.handleUpdatePlan(context.Background(), UpdatePlanArgs{

		Tasks: []planning.Task{{ID: "t1"}},
	})

	assertToolError(t, res, err, "")

	// 8.4 HandleStatus error
	res, err = s.handleStatus(context.Background(), StatusArgs{})
	assertToolError(t, res, err, "")

	// 8.5 HandleDetectDrift error
	res, err = s.handleDetectDrift(context.Background(), DetectDriftArgs{})
	assertToolError(t, res, err, "")

	// 8.6 HandleCheckPolicy error
	res, err = s.handleCheckPolicy(context.Background(), CheckPolicyArgs{})
	assertToolError(t, res, err, "")
}
