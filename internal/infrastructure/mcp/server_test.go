package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/internal/infrastructure/config"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestServerHandlersExercise(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}

	if err := config.SaveAIConfig(root, &config.AIConfig{Provider: "mock", Model: "coverage-test"}); err != nil {
		t.Fatalf("save ai config: %v", err)
	}

	specFile := &spec.ProductSpec{
		ID:    "project",
		Title: "Project",
		Features: []spec.Feature{
			{
				ID:    "feature",
				Title: "Feature",
				Requirements: []spec.Requirement{
					{ID: "req-alpha", Title: "Alpha", Description: "Desc"},
					{ID: "req-beta", Title: "Beta", Description: "Desc"},
				},
			},
		},
	}
	if err := repo.SaveSpec(specFile); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "plan-1",
		SpecID:         specFile.ID,
		ApprovalStatus: planning.ApprovalPending,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
		Tasks: []planning.Task{
			{ID: "task-req-alpha", Title: "Alpha Task", FeatureID: "feature"},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState(plan.ID)
	state.TaskStates["task-req-alpha"] = planning.TaskResult{Status: planning.StatusPending}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	if err := repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 3, AllowAI: true}); err != nil {
		t.Fatalf("save policy: %v", err)
	}

	if err := repo.UpdateUsage(domain.UsageStats{ProviderStats: map[string]int{}}); err != nil {
		t.Fatalf("update usage: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o700); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(original)
	})
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	// Initialize dummy git repo
	_ = exec.Command("git", "init").Run()
	_ = exec.Command("git", "config", "user.email", "test@example.com").Run()
	_ = exec.Command("git", "config", "user.name", "Test").Run()
	_ = exec.Command("git", "config", "commit.gpgsign", "false").Run()
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("test"), 0644)
	_ = exec.Command("git", "add", ".").Run()
	_ = exec.Command("git", "commit", "-m", "Initial commit").Run()

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx := context.Background()

	if _, err := server.handleGetSpec(ctx, GetSpecArgs{}); err != nil {
		t.Fatalf("get spec: %v", err)
	}
	if _, err := server.handleGetPlan(ctx, GetPlanArgs{}); err != nil {
		t.Fatalf("get plan: %v", err)
	}
	if _, err := server.handleGetState(ctx, GetStateArgs{}); err != nil {
		t.Fatalf("get state: %v", err)
	}

	if _, err := server.handleGeneratePlan(ctx, GeneratePlanArgs{}); err != nil {
		t.Fatalf("generate plan: %v", err)
	}

	if _, err := server.handleUpdatePlan(ctx, UpdatePlanArgs{
		Tasks: []planning.Task{
			{ID: "task-req-alpha", Title: "Alpha Task", FeatureID: "feature"},
		},
	}); err != nil {
		t.Fatalf("update plan: %v", err)
	}

	if _, err := server.handleDetectDrift(ctx, DetectDriftArgs{}); err != nil {
		t.Fatalf("detect drift: %v", err)
	}

	if _, err := server.handleStatus(ctx, StatusArgs{}); err != nil {
		t.Fatalf("status: %v", err)
	}

	if _, err := server.handleCheckPolicy(ctx, CheckPolicyArgs{}); err != nil {
		t.Fatalf("check policy: %v", err)
	}

	if _, err := server.handleGetUsage(ctx, GetUsageArgs{}); err != nil {
		t.Fatalf("get usage: %v", err)
	}

	if _, err := server.handleExplainSpec(ctx, ExplainSpecArgs{}); err != nil {
		t.Fatalf("explain spec: %v", err)
	}

	// A project with no drift legitimately has nothing to explain; the
	// handler says so rather than assembling an empty prompt.
	if _, err := server.handleExplainDrift(ctx, ExplainDriftArgs{}); err != nil &&
		!strings.Contains(err.Error(), "no drift issues") {
		t.Fatalf("explain drift: %v", err)
	}

	if _, err := server.handleAcceptDrift(ctx, AcceptDriftArgs{}); err != nil {
		t.Fatalf("accept drift: %v", err)
	}

	if _, err := server.handleForecast(ctx, ForecastArgs{}); err != nil {
		t.Fatalf("forecast: %v", err)
	}

	if _, err := server.handleOrgStatus(ctx, GetSpecArgs{}); err != nil {
		t.Fatalf("org status: %v", err)
	}

	if _, err := server.handleGitSync(ctx, GitSyncArgs{}); err != nil {
		t.Fatalf("git sync: %v", err)
	}

	if _, err := server.handleApprovePlan(ctx, ApprovePlanArgs{}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}

	if _, err := server.handleTransitionTask(ctx, TransitionTaskArgs{
		TaskID:   "task-req-alpha",
		Event:    "start",
		Evidence: "coverage",
	}); err != nil {
		t.Fatalf("transition task: %v", err)
	}

	if _, err := server.handleAddFeature(ctx, AddFeatureArgs{
		Title:       "extra",
		Description: "details",
	}); err != nil {
		t.Fatalf("add feature: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(root, "docs", "backlog.md"))
	if err != nil {
		t.Fatalf("read backlog: %v", err)
	}
	if !strings.Contains(string(content), "extra") {
		t.Fatalf("backlog missing feature: %s", content)
	}

	if _, err := server.handleDetectDrift(ctx, DetectDriftArgs{}); err != nil {
		t.Fatalf("detect drift after feature: %v", err)
	}

	if _, err := server.handleStatus(ctx, StatusArgs{}); err != nil {
		t.Fatalf("status after transition: %v", err)
	}

	if _, err := server.handleExplainSpec(ctx, ExplainSpecArgs{}); err != nil {
		t.Fatalf("explain spec post-change: %v", err)
	}
}

func TestServerPlanEventsLogged(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initMockAIConfig(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specFile := &spec.ProductSpec{
		ID:    "event-spec",
		Title: "Eventful Project",
		Features: []spec.Feature{
			{ID: "feature-a", Title: "Feature A"},
		},
	}
	if err := repo.SaveSpec(specFile); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx := context.Background()

	if _, err := server.handleGeneratePlan(ctx, GeneratePlanArgs{}); err != nil {
		t.Fatalf("generate plan failed: %v", err)
	}
	if _, err := server.handleApprovePlan(ctx, ApprovePlanArgs{}); err != nil {
		t.Fatalf("approve plan failed: %v", err)
	}

	events, err := repo.LoadEvents()
	if err != nil {
		t.Fatalf("load events: %v", err)
	}

	found := map[string]bool{}
	for _, ev := range events {
		found[ev.Action] = true
	}

	for _, want := range []string{"plan.generate", "plan.approved"} {
		if !found[want] {
			t.Fatalf("expected event %s, got %v", want, events)
		}
	}
}

func TestInitHandlerCreatesProject(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ROADY_AI_PROVIDER", "mock")
	t.Setenv("ROADY_AI_MODEL", "test")
	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if _, err := server.handleInit(context.Background(), InitArgs{Name: "demo"}); err != nil {
		t.Fatalf("init handler failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".roady", "spec.yaml")); err != nil {
		t.Fatalf("spec was not created: %v", err)
	}
}

func TestServicesForPath_DefaultRoot(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initMockAIConfig(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "test", Title: "Test"}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Empty override returns cached services
	svc, err := server.servicesForPath("", "")
	if err != nil {
		t.Fatalf("servicesForPath empty: %v", err)
	}
	if svc != server.services {
		t.Fatal("expected same services instance for empty path")
	}

	// Same root returns cached services
	svc, err = server.servicesForPath(root, "")
	if err != nil {
		t.Fatalf("servicesForPath same root: %v", err)
	}
	if svc != server.services {
		t.Fatal("expected same services instance for same root")
	}
}

func TestServicesForPath_Override(t *testing.T) {
	rootA := t.TempDir()
	repoA := storage.NewFilesystemRepository(rootA)
	if err := repoA.Initialize(); err != nil {
		t.Fatalf("initialize repo A: %v", err)
	}
	if err := initMockAIConfig(rootA); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}
	if err := repoA.SaveSpec(&spec.ProductSpec{ID: "project-a", Title: "Project A"}); err != nil {
		t.Fatalf("save spec A: %v", err)
	}

	rootB := t.TempDir()
	repoB := storage.NewFilesystemRepository(rootB)
	if err := repoB.Initialize(); err != nil {
		t.Fatalf("initialize repo B: %v", err)
	}
	if err := initMockAIConfig(rootB); err != nil {
		t.Fatalf("init mock AI config B: %v", err)
	}
	if err := repoB.SaveSpec(&spec.ProductSpec{ID: "project-b", Title: "Project B"}); err != nil {
		t.Fatalf("save spec B: %v", err)
	}

	server, err := NewServer(rootA)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Override path builds fresh services
	svc, err := server.servicesForPath(rootB, "")
	if err != nil {
		t.Fatalf("servicesForPath override: %v", err)
	}
	if svc == server.services {
		t.Fatal("expected different services instance for override path")
	}
}

func TestServicesForPath_CacheHitAndEviction(t *testing.T) {
	// Set up the "home" project.
	rootA := t.TempDir()
	repoA := storage.NewFilesystemRepository(rootA)
	if err := repoA.Initialize(); err != nil {
		t.Fatalf("init repo A: %v", err)
	}
	if err := initMockAIConfig(rootA); err != nil {
		t.Fatalf("mock AI config A: %v", err)
	}
	if err := repoA.SaveSpec(&spec.ProductSpec{ID: "a", Title: "A"}); err != nil {
		t.Fatalf("save spec A: %v", err)
	}

	srv, err := NewServer(rootA)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	// Create enough cross-project dirs to exceed maxCachedServices.
	dirs := make([]string, maxCachedServices+2)
	for i := range dirs {
		d := t.TempDir()
		repo := storage.NewFilesystemRepository(d)
		if err := repo.Initialize(); err != nil {
			t.Fatalf("init repo %d: %v", i, err)
		}
		if err := initMockAIConfig(d); err != nil {
			t.Fatalf("mock AI config %d: %v", i, err)
		}
		if err := repo.SaveSpec(&spec.ProductSpec{ID: "p", Title: "P"}); err != nil {
			t.Fatalf("save spec %d: %v", i, err)
		}
		dirs[i] = d
	}

	// First call builds, second call should return the cached instance.
	svc1, err := srv.servicesForPath(dirs[0], "")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	svc2, err := srv.servicesForPath(dirs[0], "")
	if err != nil {
		t.Fatalf("cached call: %v", err)
	}
	if svc1 != svc2 {
		t.Fatal("expected cache hit for same override path")
	}

	// Fill the cache beyond the cap.
	for _, d := range dirs[1:] {
		if _, err := srv.servicesForPath(d, ""); err != nil {
			t.Fatalf("fill cache: %v", err)
		}
	}

	// The first entry should have been evicted.
	svc3, err := srv.servicesForPath(dirs[0], "")
	if err != nil {
		t.Fatalf("post-eviction call: %v", err)
	}
	if svc3 == svc1 {
		t.Fatal("expected cache miss after eviction (got same pointer)")
	}
}

func TestHandleGetSpec_WithProjectPath(t *testing.T) {
	rootA := t.TempDir()
	repoA := storage.NewFilesystemRepository(rootA)
	if err := repoA.Initialize(); err != nil {
		t.Fatalf("initialize repo A: %v", err)
	}
	if err := initMockAIConfig(rootA); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}
	if err := repoA.SaveSpec(&spec.ProductSpec{ID: "project-a", Title: "Project A"}); err != nil {
		t.Fatalf("save spec A: %v", err)
	}

	rootB := t.TempDir()
	repoB := storage.NewFilesystemRepository(rootB)
	if err := repoB.Initialize(); err != nil {
		t.Fatalf("initialize repo B: %v", err)
	}
	if err := initMockAIConfig(rootB); err != nil {
		t.Fatalf("init mock AI config B: %v", err)
	}
	if err := repoB.SaveSpec(&spec.ProductSpec{ID: "project-b", Title: "Project B"}); err != nil {
		t.Fatalf("save spec B: %v", err)
	}

	// Server created at root A
	server, err := NewServer(rootA)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	ctx := context.Background()

	// Default: returns A's spec
	result, err := server.handleGetSpec(ctx, GetSpecArgs{})
	if err != nil {
		t.Fatalf("get spec default: %v", err)
	}
	specA := result.(*spec.ProductSpec)
	if specA.Title != "Project A" {
		t.Fatalf("expected Project A, got %s", specA.Title)
	}

	// Override: returns B's spec
	result, err = server.handleGetSpec(ctx, GetSpecArgs{ProjectPath: rootB})
	if err != nil {
		t.Fatalf("get spec with override: %v", err)
	}
	specB := result.(*spec.ProductSpec)
	if specB.Title != "Project B" {
		t.Fatalf("expected Project B, got %s", specB.Title)
	}
}

func TestGRPCServerStartsAndStops(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initMockAIConfig(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	specFile := &spec.ProductSpec{
		ID:    "grpc-test",
		Title: "gRPC Test Project",
	}
	if err := repo.SaveSpec(specFile); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- server.ServeGRPC(ctx, ":0") // Use :0 for random available port
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down in time")
	}
}
