package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestAuditVerifyCmd_NoViolations(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	audit := application.NewAuditService(repo)
	if err := audit.Log("spec.update", "tester", nil); err != nil {
		t.Fatalf("log event: %v", err)
	}

	output := captureStdout(t, func() {
		if err := auditVerifyCmd.RunE(auditVerifyCmd, []string{}); err != nil {
			t.Fatalf("audit verify failed: %v", err)
		}
	})

	if !strings.Contains(output, "Audit trail is intact") {
		t.Fatalf("expected audit verify output, got:\n%s", output)
	}
}

func TestPolicyCheckCmd_Violations(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePolicy(&domain.PolicyConfig{MaxWIP: 1})
	_ = repo.SavePlan(&planning.Plan{
		Tasks: []planning.Task{
			{ID: "t1"},
			{ID: "t2"},
		},
	})
	_ = repo.SaveState(&planning.ExecutionState{
		TaskStates: map[string]planning.TaskResult{
			"t1": {Status: planning.StatusInProgress},
			"t2": {Status: planning.StatusInProgress},
		},
	})

	if err := policyCheckCmd.RunE(policyCheckCmd, []string{}); err == nil {
		t.Fatal("expected policy violations")
	}
}

func TestPolicyCheckCmd_NoViolations(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	output := captureStdout(t, func() {
		if err := policyCheckCmd.RunE(policyCheckCmd, []string{}); err != nil {
			t.Fatalf("policy check failed: %v", err)
		}
	})
	if !strings.Contains(output, "No policy violations") {
		t.Fatalf("expected no violations output, got:\n%s", output)
	}
}

func TestSpecCommands(t *testing.T) {
	root, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()

	mdPath := filepath.Join(root, "spec.md")
	_ = os.WriteFile(mdPath, []byte("# Project\n\n## Feature 1\nDesc"), 0600)

	output := captureStdout(t, func() {
		if err := specImportCmd.RunE(specImportCmd, []string{mdPath}); err != nil {
			t.Fatalf("spec import failed: %v", err)
		}
	})
	if !strings.Contains(output, "Successfully imported spec") {
		t.Fatalf("expected import output, got:\n%s", output)
	}

	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-1",
		Title:    "Project",
		Version:  "0.1.0",
		Features: []spec.Feature{{ID: "f1", Title: "Feature"}},
	})
	output = captureStdout(t, func() {
		if err := specAddCmd.RunE(specAddCmd, []string{"Feature 2", "Desc 2"}); err != nil {
			t.Fatalf("spec add failed: %v", err)
		}
	})
	if !strings.Contains(output, "Successfully added feature") {
		t.Fatalf("expected add output, got:\n%s", output)
	}

	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-2",
		Title:    "Project",
		Features: []spec.Feature{{ID: "f1"}, {ID: "f1"}},
	})
	if err := specValidateCmd.RunE(specValidateCmd, []string{}); err == nil {
		t.Fatal("expected spec validation error")
	}
}

func TestPlanCommands(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-1",
		Title:    "Project",
		Features: []spec.Feature{{ID: "f1", Title: "Feature"}},
	})
	_ = repo.SavePlan(&planning.Plan{
		ID:             "p1",
		ApprovalStatus: planning.ApprovalPending,
		Tasks: []planning.Task{
			{ID: "task-r1", FeatureID: "f1", Title: "Task"},
			{ID: "task-r2", FeatureID: "f2", Title: "Orphan"},
		},
	})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	if err := planApproveCmd.RunE(planApproveCmd, []string{}); err != nil {
		t.Fatalf("plan approve failed: %v", err)
	}
	if err := planRejectCmd.RunE(planRejectCmd, []string{}); err != nil {
		t.Fatalf("plan reject failed: %v", err)
	}
	if err := planPruneCmd.RunE(planPruneCmd, []string{}); err != nil {
		t.Fatalf("plan prune failed: %v", err)
	}
}

func TestTaskCommands(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-1",
		Title:    "Project",
		Features: []spec.Feature{{ID: "f1", Title: "Feature"}},
	})
	_ = repo.SavePlan(&planning.Plan{
		ID:             "p1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", FeatureID: "f1", Title: "Task"},
		},
	})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	cmd := createTaskCommand("start", "Start", "start")
	cmd.SetArgs([]string{"task-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task start failed: %v", err)
	}
}

func TestTaskCommand_CompleteWithEvidence(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-1",
		Title:    "Project",
		Features: []spec.Feature{{ID: "f1", Title: "Feature"}},
	})
	_ = repo.SavePlan(&planning.Plan{
		ID:             "p1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", FeatureID: "f1", Title: "Task"},
		},
	})
	state := planning.NewExecutionState("p1")
	state.TaskStates["task-1"] = planning.TaskResult{Status: planning.StatusInProgress}
	_ = repo.SaveState(state)

	cmd := createTaskCommand("complete", "Complete", "complete")
	cmd.SetArgs([]string{"task-1", "--evidence", "commit-123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("task complete failed: %v", err)
	}
}

func TestSyncCmd_UpdatesStatuses(t *testing.T) {
	repoRoot := findRepoRoot(t)
	root, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-1",
		Title:    "Project",
		Features: []spec.Feature{{ID: "f1", Title: "Feature"}},
	})
	_ = repo.SavePlan(&planning.Plan{
		ID:             "p1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "t1", FeatureID: "f1", Title: "Task 1"},
			{ID: "t2", FeatureID: "f1", Title: "Task 2"},
		},
	})
	state := planning.NewExecutionState("p1")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusInProgress}
	_ = repo.SaveState(state)

	pluginBin := filepath.Join(repoRoot, "roady-plugin-mock")
	if _, err := os.Stat(pluginBin); err != nil {
		pluginBin = filepath.Join(root, "roady-plugin-mock")
		source := filepath.Join(repoRoot, "cmd", "roady-plugin-mock", "main.go")
		cmd := exec.Command("go", "build", "-o", pluginBin, source)
		cmd.Dir = repoRoot
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build plugin: %v (%s)", err, strings.TrimSpace(string(out)))
		}
	}

	if err := syncCmd.RunE(syncCmd, []string{pluginBin}); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	updated, _ := repo.LoadState()
	if updated.TaskStates["t1"].Status != planning.StatusInProgress {
		t.Fatalf("expected t1 to be in progress, got %s", updated.TaskStates["t1"].Status)
	}
	if updated.TaskStates["t2"].Status != planning.StatusDone {
		t.Fatalf("expected t2 to be done, got %s", updated.TaskStates["t2"].Status)
	}
}

func TestWatchCmd_RunOnce(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	if err := os.MkdirAll("docs", 0700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile("docs/spec.md", []byte("# Project\n\n## Feature\nDesc"), 0600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	t.Setenv("ROADY_WATCH_ONCE", "true")
	t.Setenv("ROADY_WATCH_SEED_HASH", "seed")
	if err := watchCmd.RunE(watchCmd, []string{"docs"}); err != nil {
		t.Fatalf("watch run failed: %v", err)
	}
}

func TestMCPCmd_Skip(t *testing.T) {
	t.Setenv("ROADY_SKIP_MCP_START", "true")
	if err := mcpCmd.RunE(mcpCmd, []string{}); err != nil {
		t.Fatalf("mcp cmd failed: %v", err)
	}
}

func TestWatchCmd_AutoSync(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePolicy(&domain.PolicyConfig{AllowAI: true})
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	if err := os.MkdirAll("docs", 0700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile("docs/spec.md", []byte("# Project\n\n## Feature\nDesc"), 0600); err != nil {
		t.Fatalf("write spec: %v", err)
	}

	t.Setenv("ROADY_WATCH_ONCE", "true")
	t.Setenv("ROADY_WATCH_SEED_HASH", "seed")
	t.Setenv("ROADY_AI_PROVIDER", "mock")
	t.Setenv("ROADY_AI_MODEL", "test")
	autoSync = true
	defer func() { autoSync = false }()
	watchCmd.SetContext(context.Background())

	if err := watchCmd.RunE(watchCmd, []string{"docs"}); err != nil {
		t.Fatalf("watch auto-sync failed: %v", err)
	}
}

func TestStatusCmd_WithProgress(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Project",
		Version: "0.1.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature"},
		},
	})
	_ = repo.SavePlan(&planning.Plan{
		ID:             "p1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "t1", FeatureID: "f1", Title: "Task 1", Priority: planning.PriorityHigh},
			{ID: "t2", FeatureID: "f1", Title: "Task 2", Priority: planning.PriorityLow},
		},
	})
	state := planning.NewExecutionState("p1")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusVerified}
	state.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusInProgress}
	_ = repo.SaveState(state)

	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})
	if !strings.Contains(output, "Overall Progress") {
		t.Fatalf("expected progress output, got:\n%s", output)
	}
}

func TestSpecAnalyzeCmd(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()

	if err := os.MkdirAll("docs", 0700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	_ = os.WriteFile("docs/a.md", []byte("# Project\n\n## Feature A\nDesc A"), 0600)
	_ = os.WriteFile("docs/b.md", []byte("# Project\n\n## Feature B\nDesc B"), 0600)

	output := captureStdout(t, func() {
		if err := specAnalyzeCmd.RunE(specAnalyzeCmd, []string{"docs"}); err != nil {
			t.Fatalf("spec analyze failed: %v", err)
		}
	})
	if !strings.Contains(output, "Successfully analyzed directory") {
		t.Fatalf("expected analyze output, got:\n%s", output)
	}
}

func TestStatusCmd_DriftWarning(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Project",
		Version: "0.1.0",
		Features: []spec.Feature{
			{
				ID:    "f1",
				Title: "Feature",
				Requirements: []spec.Requirement{
					{ID: "r1", Title: "Req"},
				},
			},
		},
	})
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})
	if !strings.Contains(output, "DRIFT DETECTED") {
		t.Fatalf("expected drift warning, got:\n%s", output)
	}
}

func TestDriftDetectCmd_NoIssues(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:    "spec-1",
		Title: "Project",
		Features: []spec.Feature{
			{
				ID:    "f1",
				Title: "Feature",
				Requirements: []spec.Requirement{
					{ID: "r1", Title: "Req"},
				},
			},
		},
	})
	_ = repo.SavePlan(&planning.Plan{
		ID: "p1",
		Tasks: []planning.Task{
			{ID: "task-r1", FeatureID: "f1", Title: "Req"},
		},
	})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	_ = driftDetectCmd.Flags().Set("output", "text")

	output := captureStdout(t, func() {
		if err := driftDetectCmd.RunE(driftDetectCmd, []string{}); err != nil {
			t.Fatalf("drift detect failed: %v", err)
		}
	})
	if !strings.Contains(output, "No drift detected") {
		t.Fatalf("expected no drift output, got:\n%s", output)
	}
}

func TestDriftAcceptCmd(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Project",
		Version: "0.1.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature"},
		},
	})

	output := captureStdout(t, func() {
		if err := driftAcceptCmd.RunE(driftAcceptCmd, []string{}); err != nil {
			t.Fatalf("drift accept failed: %v", err)
		}
	})
	if !strings.Contains(output, "Drift accepted") {
		t.Fatalf("expected accept output, got:\n%s", output)
	}

	lock, err := repo.LoadSpecLock()
	if err != nil {
		t.Fatalf("load spec lock failed: %v", err)
	}
	if lock == nil {
		t.Fatal("expected spec lock to exist")
	}

	events, err := repo.LoadEvents()
	if err != nil {
		t.Fatalf("load events failed: %v", err)
	}
	if len(events) == 0 || events[len(events)-1].Action != "drift.accepted" {
		t.Fatal("expected drift.accepted event")
	}
}

func TestStatusCmd_NoPlan(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Project",
		Version: "0.1.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature"},
		},
	})

	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})
	if !strings.Contains(output, "no plan has been generated") {
		t.Fatalf("expected no-plan empty-state output, got:\n%s", output)
	}
	if !strings.Contains(output, "roady plan generate") {
		t.Fatalf("expected next-step CTA pointing at `roady plan generate`, got:\n%s", output)
	}
}

func TestDiscoverCmd_NoProjects(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()

	output := captureStdout(t, func() {
		if err := discoverCmd.RunE(discoverCmd, []string{"."}); err != nil {
			t.Fatalf("discover failed: %v", err)
		}
	})
	if !strings.Contains(output, "No Roady projects found") {
		t.Fatalf("expected no project output, got:\n%s", output)
	}
}

func TestAuditVerifyCmd_Violations(t *testing.T) {
	if os.Getenv("ROADY_TEST_AUDIT_VERIFY") == "1" {
		tempDir, _ := os.MkdirTemp("", "roady-audit-verify-*")
		_ = os.Chdir(tempDir)

		repo := storage.NewFilesystemRepository(".")
		_ = repo.Initialize()
		path, err := repo.ResolvePath("events.jsonl")
		if err != nil {
			os.Exit(2)
		}

		content := `{"id":"e1","timestamp":"2026-01-01T00:00:00Z","action":"spec.update","actor":"tester","prev_hash":"","hash":"bad"}`
		if err := os.WriteFile(path, []byte(content+"\n"), 0600); err != nil {
			os.Exit(2)
		}

		_ = auditVerifyCmd.RunE(auditVerifyCmd, []string{})
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run", "TestAuditVerifyCmd_Violations")
	cmd.Env = append(os.Environ(), "ROADY_TEST_AUDIT_VERIFY=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected audit verify to fail")
	}
}

func TestDoctorCmd_BudgetCheck(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{ID: "spec-1", Title: "Project"})
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	state := planning.NewExecutionState("p1")
	state.ProjectID = "p1"
	_ = repo.SaveState(state)
	_ = repo.SavePolicy(&domain.PolicyConfig{TokenLimit: 10})
	_ = repo.UpdateUsage(domain.UsageStats{
		ProviderStats: map[string]int{"mock:input": 2},
	})
	audit := application.NewAuditService(repo)
	_ = audit.Log("spec.update", "tester", nil)

	output := captureStdout(t, func() {
		if err := doctorCmd.RunE(doctorCmd, []string{}); err != nil {
			t.Fatalf("doctor failed: %v", err)
		}
	})
	if !strings.Contains(output, "Budget") {
		t.Fatalf("expected budget output, got:\n%s", output)
	}
}

func TestPlanGenerateCmd_Output(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:    "spec-1",
		Title: "Project",
		Features: []spec.Feature{
			{
				ID:    "f1",
				Title: "Feature",
				Requirements: []spec.Requirement{
					{ID: "r1", Title: "Req", Description: "Desc"},
				},
			},
		},
	})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	output := captureStdout(t, func() {
		if err := planGenerateCmd.RunE(planGenerateCmd, []string{}); err != nil {
			t.Fatalf("plan generate failed: %v", err)
		}
	})
	if !strings.Contains(output, "Successfully generated plan") {
		t.Fatalf("expected plan output, got:\n%s", output)
	}
}

func TestPlanApproveCmd_NoPlan(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()

	if err := planApproveCmd.RunE(planApproveCmd, []string{}); err == nil {
		t.Fatal("expected approve error with no plan")
	}
}

func TestSpecAnalyzeCmd_NoFeatures(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()

	if err := os.MkdirAll("docs", 0700); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	_ = os.WriteFile("docs/empty.md", []byte("# Project\n\nNo features here"), 0600)

	if err := specAnalyzeCmd.RunE(specAnalyzeCmd, []string{"docs"}); err == nil {
		t.Fatal("expected analyze error with no features")
	}
}

func TestSpecValidateCmd_Success(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Project",
		Version: "0.1.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature", Requirements: []spec.Requirement{{ID: "r1", Title: "Req"}}},
		},
	})

	output := captureStdout(t, func() {
		if err := specValidateCmd.RunE(specValidateCmd, []string{}); err != nil {
			t.Fatalf("spec validate failed: %v", err)
		}
	})
	if !strings.Contains(output, "Spec is valid") {
		t.Fatalf("expected spec valid output, got:\n%s", output)
	}
}

func TestPlanGenerateCmd_AI(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePolicy(&domain.PolicyConfig{AllowAI: true})
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:       "spec-1",
		Title:    "Project",
		Features: []spec.Feature{{ID: "core-foundation", Title: "Core"}},
	})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	t.Setenv("ROADY_AI_PROVIDER", "mock")
	t.Setenv("ROADY_AI_MODEL", "test")
	useAI = true
	defer func() { useAI = false }()
	planGenerateCmd.SetContext(context.Background())

	if err := planGenerateCmd.RunE(planGenerateCmd, []string{}); err != nil {
		t.Fatalf("plan generate ai failed: %v", err)
	}
}

func TestDriftDetectCmd_JSON(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:    "spec-1",
		Title: "Project",
		Features: []spec.Feature{
			{
				ID:    "f1",
				Title: "Feature",
				Requirements: []spec.Requirement{
					{ID: "r1", Title: "Req"},
				},
			},
		},
	})
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	_ = driftDetectCmd.Flags().Set("output", "json")
	defer func() { _ = driftDetectCmd.Flags().Set("output", "text") }()

	if err := driftDetectCmd.RunE(driftDetectCmd, []string{}); err == nil {
		t.Fatal("expected drift error")
	}
}

func TestSpecExplainCmd_EmitsPrompt(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePolicy(&domain.PolicyConfig{AllowAI: true})
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:    "spec-1",
		Title: "Project",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature", Description: "Desc"},
		},
	})

	output := captureStdout(t, func() {
		specExplainCmd.SetContext(context.Background())
		if err := specExplainCmd.RunE(specExplainCmd, []string{}); err != nil {
			t.Fatalf("spec explain failed: %v", err)
		}
	})
	// Roady emits the prompt; the caller runs it.
	if !strings.Contains(output, "architectural walkthrough") {
		t.Fatalf("expected an assembled explain prompt, got:\n%s", output)
	}
}

func TestDriftExplainCmd_EmitsPrompt(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	repo := storage.NewFilesystemRepository(".")
	_ = repo.Initialize()
	_ = repo.SavePolicy(&domain.PolicyConfig{AllowAI: true})
	_ = repo.SaveSpec(&spec.ProductSpec{
		ID:    "spec-1",
		Title: "Project",
		Features: []spec.Feature{
			{
				ID:    "f1",
				Title: "Feature",
				Requirements: []spec.Requirement{
					{ID: "r1", Title: "Req"},
				},
			},
		},
	})
	_ = repo.SavePlan(&planning.Plan{ID: "p1"})
	_ = repo.SaveState(planning.NewExecutionState("p1"))

	output := captureStdout(t, func() {
		driftExplainCmd.SetContext(context.Background())
		if err := driftExplainCmd.RunE(driftExplainCmd, []string{}); err != nil {
			t.Fatalf("drift explain failed: %v", err)
		}
	})
	if !strings.Contains(output, "Explain this drift") {
		t.Fatalf("expected an assembled drift prompt, got:\n%s", output)
	}
}
