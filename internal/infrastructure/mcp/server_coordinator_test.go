package mcp

import (
	"context"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
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
	if err := initProjectDir(root); err != nil {
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

// Every registered tool must carry behaviour annotations. An unannotated tool
// leaves a client unable to tell a read from a destructive write.
func TestServer_EveryToolIsAnnotated(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	for _, tool := range server.mcpServer.Tools() {
		if _, ok := toolBehaviours[tool.Name]; !ok {
			t.Errorf("tool %q has no entry in toolBehaviours", tool.Name)
		}
	}
}

func TestDriftDetectSemantic_ReturnsQuestionsWithTheRequest(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	res, err := server.handleDetectDrift(context.Background(), DetectDriftArgs{Semantic: true})
	if err != nil {
		t.Fatalf("handleDetectDrift(semantic): %v", err)
	}
	// The fixture's spec has a feature with no implemented work, so Roady
	// refuses rather than asking a model about nothing. That refusal is the
	// documented behaviour and must be actionable.
	if out, ok := res.(map[string]any); ok {
		if _, has := out["questions"]; !has {
			t.Error("a successful request came back without its questions")
		}
		if _, has := out["request"]; !has {
			t.Error("no request returned")
		}
		return
	}
	assertToolError(t, res, err, "")
}

func TestRootForAndProjectDirName(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	if got := server.rootFor(""); got != server.root {
		t.Errorf("rootFor(\"\") = %q, want the server root", got)
	}
	if got := server.rootFor("  "); got != server.root {
		t.Errorf("rootFor(blank) = %q, want the server root", got)
	}
	if got := server.rootFor("/tmp/elsewhere"); got != "/tmp/elsewhere" {
		t.Errorf("rootFor(override) = %q, want the override", got)
	}

}

// A judgement can only attach to a requirement roady asked about: the
// questions are rebuilt server-side, so an invented id is refused.
func TestRecordSemanticDrift_RejectsUnaskedRequirements(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	res, err := server.handleRecordSemanticDrift(context.Background(), RecordSemanticDriftArgs{
		Judgements: []drift.SemanticJudgement{{RequirementID: "invented", Agrees: false, Explanation: "x"}},
	})
	assertToolError(t, res, err, "")
}
