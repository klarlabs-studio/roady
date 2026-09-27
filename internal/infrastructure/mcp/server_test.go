package mcp

import (
	"context"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

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
	if err := initProjectDir(root); err != nil {
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
	if err := initProjectDir(rootA); err != nil {
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
	if err := initProjectDir(rootB); err != nil {
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
	if err := initProjectDir(rootA); err != nil {
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
		if err := initProjectDir(d); err != nil {
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

func TestGRPCServerStartsAndStops(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}
	if err := initProjectDir(root); err != nil {
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

func TestServerHandlersExercise(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
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

	if _, err := server.handleDetectDrift(ctx, DetectDriftArgs{}); err != nil {
		t.Fatalf("detect drift: %v", err)
	}

	if _, err := server.handleStatus(ctx, StatusArgs{}); err != nil {
		t.Fatalf("status: %v", err)
	}

	if _, err := server.handleTransitionTask(ctx, TransitionTaskArgs{
		TaskID:   "task-req-alpha",
		Event:    "start",
		Evidence: "coverage",
	}); err != nil {
		t.Fatalf("transition task: %v", err)
	}

	if _, err := server.handleDetectDrift(ctx, DetectDriftArgs{}); err != nil {
		t.Fatalf("detect drift after transition: %v", err)
	}

	if _, err := server.handleStatus(ctx, StatusArgs{}); err != nil {
		t.Fatalf("status after transition: %v", err)
	}

}
