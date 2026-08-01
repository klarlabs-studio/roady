package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// seedProject writes a small but complete project into the current working
// directory, so the command runners exercise the real load-services path.
func seedProject(t *testing.T) {
	t.Helper()

	repo := storage.NewFilesystemRepository(".")
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "spec-1", Title: "Checkout", Version: "0.1.0",
		Features: []spec.Feature{{ID: "f1", Title: "Signup"}},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}
	if err := repo.SavePlan(&planning.Plan{
		ID: "plan-1", SpecID: "spec-1", ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "task-1", Title: "Wire signup", FeatureID: "f1", Priority: planning.PriorityHigh},
			{ID: "task-2", Title: "Refunds", FeatureID: "f1"},
		},
	}); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState("plan-1")
	state.TaskStates["task-1"] = planning.TaskResult{
		Status: planning.StatusDone, Owner: "alice", Evidence: []string{"commit abc123"},
	}
	state.TaskStates["task-2"] = planning.TaskResult{Status: planning.StatusPending}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	audit := application.NewAuditService(repo)
	_ = audit.Log("task.started", "alice", map[string]any{
		"task_id": "task-1", "session_id": "s1", "agent": "claude-code",
	})
	_ = audit.Log("task.completed", "system", map[string]any{
		"task_id": "task-1", "evidence": "commit abc123", "session_id": "s1", "agent": "claude-code",
	})
}

// resetReportFlags clears package-level flag state between runs, since cobra
// flag vars persist across tests in the same binary.
func resetReportFlags() {
	reportFormat, reportOutput, reportSince, reportProject, reportMax = "markdown", "", "", "", 0
	trailAgent, trailSession, trailSince, trailFormat, trailOutput = "", "", "", "markdown", ""
	digestSince, digestAdapter, digestDryRun = "", "", false
	stateRebuildDryRun = false
}

func TestRunReportMarkdown(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	reportProject = "checkout"
	out := captureStdout(t, func() {
		if err := runReport(reportCmd, nil); err != nil {
			t.Errorf("runReport: %v", err)
		}
	})

	for _, want := range []string{
		"# checkout — progress report",
		"## Progress",
		"## Who is on what",
		"alice",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q\n%s", want, out)
		}
	}
}

func TestRunReportJSONIsParseable(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	reportFormat = "json"
	out := captureStdout(t, func() {
		if err := runReport(reportCmd, nil); err != nil {
			t.Errorf("runReport: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json output did not parse: %v\n%s", err, out)
	}
	if _, ok := parsed["progress"]; !ok {
		t.Errorf("expected a progress key, got %v", parsed)
	}
}

func TestRunReportHTMLToFile(t *testing.T) {
	dir, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	reportFormat = "html"
	reportOutput = filepath.Join(dir, "status.html")

	if err := runReport(reportCmd, nil); err != nil {
		t.Fatalf("runReport: %v", err)
	}

	data, err := os.ReadFile(reportOutput)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(data), "<title>") {
		t.Error("expected an HTML document")
	}
	if strings.Contains(string(data), "<script") {
		t.Error("published HTML must stay script-free")
	}
}

func TestRunReportRejectsUnknownFormat(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	reportFormat = "yaml"
	if err := runReport(reportCmd, nil); err == nil {
		t.Fatal("expected an error for an unsupported format")
	}
}

func TestRunAuditTrailRequiresASubject(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	err := runAuditTrail(auditTrailCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "--agent") {
		t.Fatalf("expected a message naming the alternatives, got %v", err)
	}
}

func TestRunAuditTrailForTask(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	out := captureStdout(t, func() {
		if err := runAuditTrail(auditTrailCmd, []string{"task-1"}); err != nil {
			t.Errorf("runAuditTrail: %v", err)
		}
	})

	for _, want := range []string{
		"# Audit trail — task `task-1`",
		"## Chain integrity",
		"claude-code",
		"not proof of identity",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("trail missing %q\n%s", want, out)
		}
	}
}

func TestRunAuditTrailByAgentJSON(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	trailAgent = "claude-code"
	trailFormat = "json"

	out := captureStdout(t, func() {
		if err := runAuditTrail(auditTrailCmd, nil); err != nil {
			t.Errorf("runAuditTrail: %v", err)
		}
	})

	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json did not parse: %v\n%s", err, out)
	}
	if subject, ok := parsed["subject"].(map[string]any); !ok || subject["kind"] != "agent" {
		t.Errorf("expected an agent subject, got %v", parsed["subject"])
	}
}

func TestNotifyDigestDryRun(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	digestDryRun = true
	out := captureStdout(t, func() {
		if err := notifyDigest(t.Context(), os.Stdout); err != nil {
			t.Errorf("notifyDigest: %v", err)
		}
	})

	if !strings.Contains(out, "complete") {
		t.Errorf("expected a headline, got %q", out)
	}
	// A digest is a nudge, not the full document.
	if strings.Contains(out, "## Progress") {
		t.Error("digest must not contain the full report body")
	}
}

func TestNotifyDigestWithoutAdaptersExplainsHow(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)
	resetReportFlags()

	err := notifyDigest(t.Context(), os.Stdout)
	if err == nil || !strings.Contains(err.Error(), "roady notify add") {
		t.Fatalf("expected guidance on configuring an adapter, got %v", err)
	}
}

func TestReportRebuildOutput(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	out := captureStdout(t, func() {
		reportRebuild(&application.RebuildResult{EventsReplayed: 3, TasksAffected: 2}, true)
	})
	if !strings.Contains(out, "Would rebuild") || !strings.Contains(out, "already matches") {
		t.Errorf("dry-run wording wrong: %q", out)
	}

	out = captureStdout(t, func() {
		reportRebuild(&application.RebuildResult{
			EventsReplayed: 4, TasksAffected: 2,
			Changed: []application.StateChange{{TaskID: "task-1", From: "", To: planning.StatusDone}},
		}, false)
	})
	if !strings.Contains(out, "Rebuilt") || !strings.Contains(out, "task-1") {
		t.Errorf("change list missing: %q", out)
	}
	// An absent prior status should read as "(absent)", not as an empty gap.
	if !strings.Contains(out, "(absent)") {
		t.Errorf("expected absent-status rendering: %q", out)
	}
}

func TestInferProjectNameUsesDirectory(t *testing.T) {
	dir, cleanup := withTempDir(t)
	defer cleanup()
	seedProject(t)

	if got := inferProjectName(); got != filepath.Base(dir) {
		t.Errorf("inferProjectName() = %q, want %q", got, filepath.Base(dir))
	}
}
