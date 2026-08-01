package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/prompt"

	"github.com/felixgeelhaar/roady/internal/infrastructure/config"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestServerForecastAndExplainDrift(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}

	product := &spec.ProductSpec{
		ID:    "project-forecast",
		Title: "Forecast Project",
		Features: []spec.Feature{
			{
				ID:          "feature-a",
				Title:       "Feature A",
				Description: "Description A",
				Requirements: []spec.Requirement{
					{ID: "req-a", Title: "Requirement A", Description: "Req A", Priority: "high"},
				},
			},
			{
				ID:          "feature-b",
				Title:       "Feature B",
				Description: "Description B",
				Requirements: []spec.Requirement{
					{ID: "req-b", Title: "Requirement B", Description: "Req B", Priority: "medium"},
				},
			},
		},
	}
	if err := repo.SaveSpec(product); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-1",
		SpecID:         product.ID,
		ApprovalStatus: planning.ApprovalPending,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Tasks: []planning.Task{
			{ID: "task-req-a", Title: "Task A", FeatureID: "feature-a"},
			{ID: "task-extra", Title: "Extra Task", FeatureID: "feature-a"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := &planning.ExecutionState{
		ProjectID: product.ID,
		TaskStates: map[string]planning.TaskResult{
			"task-req-a": {Status: planning.StatusVerified},
			"task-extra": {Status: planning.StatusInProgress},
		},
		UpdatedAt: time.Now(),
	}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	if err := config.SaveAIConfig(tempDir, &config.AIConfig{Provider: "mock", Model: "test"}); err != nil {
		t.Fatalf("save ai config: %v", err)
	}

	audit := application.NewAuditService(repo)
	if err := audit.Log("task.transition", "cli", map[string]interface{}{"status": string(planning.StatusVerified)}); err != nil {
		t.Fatalf("log event: %v", err)
	}

	server, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx := context.Background()

	forecastResult, err := server.handleForecast(ctx, ForecastArgs{})
	if err != nil {
		t.Fatalf("forecast failed: %v", err)
	}
	// Result is now a struct; marshal to JSON to verify content
	forecastJSON, err := json.Marshal(forecastResult)
	if err != nil {
		t.Fatalf("marshal forecast: %v", err)
	}
	if !strings.Contains(string(forecastJSON), `"remaining":1`) {
		t.Fatalf("unexpected forecast: %s", string(forecastJSON))
	}

	// Roady no longer explains drift itself; it hands back the prompt for
	// the caller's own model to run.
	explanation, err := server.handleExplainDrift(ctx, ExplainDriftArgs{})
	if err != nil {
		t.Fatalf("explain drift failed: %v", err)
	}
	req, ok := explanation.(*prompt.Request)
	if !ok {
		t.Fatalf("expected a prompt request, got %T", explanation)
	}
	if req.Operation != prompt.OpExplainDrift {
		t.Errorf("operation = %q, want %q", req.Operation, prompt.OpExplainDrift)
	}
	if req.Prompt == "" {
		t.Error("expected an assembled prompt")
	}
}
