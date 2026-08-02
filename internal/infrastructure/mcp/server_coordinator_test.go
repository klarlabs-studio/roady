package mcp

import (
	"context"
	"os"
	"path/filepath"
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

	// status=all returns one paged list rather than three buckets: a single
	// set of counts describes the whole answer, and each task still carries
	// the status that used to be implied by which bucket it sat in.
	page, ok := result.(taskPage)
	if !ok {
		t.Fatalf("expected a taskPage result, got %T", result)
	}
	if page.Status != "all" {
		t.Errorf("Status = %q, want %q", page.Status, "all")
	}
	if page.Limit == 0 {
		t.Error("page carries no limit, so a caller cannot tell whether it was truncated")
	}
	if page.Returned != len(page.Tasks) {
		t.Errorf("Returned = %d but %d tasks present", page.Returned, len(page.Tasks))
	}
	for _, task := range page.Tasks {
		if task.Status == "" {
			t.Errorf("task %s carries no status", task.ID)
		}
	}
}

func TestHandleTasks_InvalidStatus(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	res, err := server.handleTasks(context.Background(), TasksArgs{Status: "bogus"})

	assertToolError(t, res, err, "")
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
	}

	for _, name := range expected {
		if !registered[name] {
			t.Errorf("expected tool %q to be registered", name)
		}
	}
}

// roady_report and roady_spec_analyze existed only as CLI commands, so an
// agent could neither produce the stakeholder report the reporting work was
// built for nor create a spec from source documents — the entry point to the
// whole workflow. Same gap roady_audit_trail closed in 0.17.0.
func TestServer_RegistersReportAndSpecAnalyze(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	registered := make(map[string]bool)
	for _, tool := range server.mcpServer.Tools() {
		registered[tool.Name] = true
	}

	for _, name := range []string{"roady_report", "roady_spec_analyze"} {
		if !registered[name] {
			t.Errorf("%s is not registered; the capability is CLI-only", name)
		}
	}
}

func TestHandleReport_Formats(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	for _, format := range []string{"", "markdown", "html", "json"} {
		res, err := server.handleReport(ctx, ReportArgs{Format: format})
		if err != nil {
			t.Fatalf("handleReport(%q): %v", format, err)
		}
		if res == nil {
			t.Fatalf("handleReport(%q): nil result", format)
		}
	}
}

// A bad format and a malformed since window are caller mistakes: they must
// come back as an actionable tool error, not a protocol error or a silent
// default.
func TestHandleReport_RejectsBadInputActionably(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()

	res, err := server.handleReport(ctx, ReportArgs{Format: "bogus"})
	assertToolError(t, res, err, "bogus")

	// 7xd used to be accepted as "7 days" by the CLI's parser while the MCP
	// one rejected it; both now share application.ParseSince.
	res, err = server.handleReport(ctx, ReportArgs{Since: "7xd"})
	assertToolError(t, res, err, "7xd")
}

func TestHandleSpecAnalyze_RequiresADirectory(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	res, err := server.handleSpecAnalyze(context.Background(), SpecAnalyzeArgs{})
	assertToolError(t, res, err, "dir")
}

// The success path: an agent points the tool at a directory of documents and
// gets a spec, which is the entry point the CLI-only version denied it.
func TestHandleSpecAnalyze_BuildsSpecFromDocuments(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	docs := filepath.Join(server.root, "docs")
	if err := os.MkdirAll(docs, 0o750); err != nil {
		t.Fatal(err)
	}
	md := "# Payments\n\n## Card Payments (Stripe / Adyen)\nTake cards.\n\n## Refunds & Disputes\nHandle refunds.\n"
	if err := os.WriteFile(filepath.Join(docs, "spec.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := server.handleSpecAnalyze(context.Background(), SpecAnalyzeArgs{Dir: "docs"})
	if err != nil {
		t.Fatalf("handleSpecAnalyze: %v", err)
	}
	out, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("got %T, want a summary map", res)
	}
	if out["count"] != 2 {
		t.Errorf("count = %v, want 2", out["count"])
	}
	// Ids must be the slugified form, not the raw heading.
	feats, _ := out["features"].([]map[string]any)
	if len(feats) != 2 || feats[0]["id"] != "card-payments-stripe-adyen" {
		t.Errorf("features = %v, want slugified ids", feats)
	}
	if out["hint"] == "" {
		t.Error("no hint telling the caller what to do next")
	}
}

// A directory that does not exist is a caller mistake and must say so.
func TestHandleSpecAnalyze_MissingDirectoryIsActionable(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	res, err := server.handleSpecAnalyze(context.Background(), SpecAnalyzeArgs{Dir: "no-such-dir"})
	assertToolError(t, res, err, "no-such-dir")
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

	// A relative path must not title a report "." — resolve, then take the base.
	if got := projectDirName("."); got == "." || got == "" {
		t.Errorf("projectDirName(\".\") = %q, want the resolved directory name", got)
	}
	if got := projectDirName(server.root); got != filepath.Base(server.root) {
		t.Errorf("projectDirName(root) = %q, want %q", got, filepath.Base(server.root))
	}
}
