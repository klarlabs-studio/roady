package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func resetStatusFlags() {
	statusFilter = ""
	priorityFilter = ""
	readyOnly = false
	blockedOnly = false
	activeOnly = false
	statusLimit = 0
	statusJSON = false
	snapshotMode = false
}

func setupStatusTestData(t *testing.T) *storage.FilesystemRepository {
	t.Helper()

	repo := storage.NewFilesystemRepository(".")
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}

	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Test Project",
		Version: "1.0.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	plan := &planning.Plan{
		ID:             "p1",
		ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{
			{ID: "t1", Title: "High Priority Task", Priority: planning.PriorityHigh},
			{ID: "t2", Title: "Medium Priority Task", Priority: planning.PriorityMedium},
			{ID: "t3", Title: "Low Priority Task", Priority: planning.PriorityLow},
			{ID: "t4", Title: "In Progress Task", Priority: planning.PriorityHigh},
			{ID: "t5", Title: "Blocked Task", Priority: planning.PriorityMedium, DependsOn: []string{"t1"}},
		},
	}
	if err := repo.SavePlan(plan); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	state := planning.NewExecutionState("p1")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusDone}
	state.TaskStates["t3"] = planning.TaskResult{Status: planning.StatusVerified}
	state.TaskStates["t4"] = planning.TaskResult{Status: planning.StatusInProgress}
	state.TaskStates["t5"] = planning.TaskResult{Status: planning.StatusBlocked}
	if err := repo.SaveState(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	return repo
}

func TestStatusCmd_FilterByStatus(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	// Filter by pending status
	statusFilter = "pending"
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "High Priority Task") {
		t.Errorf("expected pending task, got:\n%s", output)
	}
	if strings.Contains(output, "In Progress Task") {
		t.Errorf("should not contain in_progress task, got:\n%s", output)
	}
}

func TestStatusCmd_FilterByMultipleStatuses(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	// Filter by pending and blocked
	statusFilter = "pending,blocked"
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "High Priority Task") {
		t.Errorf("expected pending task, got:\n%s", output)
	}
	if !strings.Contains(output, "Blocked Task") {
		t.Errorf("expected blocked task, got:\n%s", output)
	}
	if strings.Contains(output, "In Progress Task") {
		t.Errorf("should not contain in_progress task, got:\n%s", output)
	}
}

func TestStatusCmd_FilterByPriority(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	// Filter by high priority
	priorityFilter = "high"
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "High Priority Task") {
		t.Errorf("expected high priority task, got:\n%s", output)
	}
	if strings.Contains(output, "Low Priority Task") {
		t.Errorf("should not contain low priority task, got:\n%s", output)
	}
}

func TestStatusCmd_BlockedFlag(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	blockedOnly = true
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "Blocked Task") {
		t.Errorf("expected blocked task, got:\n%s", output)
	}
	if strings.Contains(output, "High Priority Task") {
		t.Errorf("should not contain non-blocked task, got:\n%s", output)
	}
}

func TestStatusCmd_ActiveFlag(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	activeOnly = true
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "In Progress Task") {
		t.Errorf("expected in_progress task, got:\n%s", output)
	}
	if strings.Contains(output, "Blocked Task") {
		t.Errorf("should not contain blocked task, got:\n%s", output)
	}
}

func TestStatusCmd_LimitFlag(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	statusLimit = 2
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	// Count task lines (each task has a status prefix)
	taskLines := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "[") && strings.Contains(line, "Task") {
			taskLines++
		}
	}
	if taskLines != 2 {
		t.Errorf("expected 2 tasks with limit, got %d tasks:\n%s", taskLines, output)
	}
}

func TestStatusCmd_JSONOutput(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	statusJSON = true
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	var result statusJSONOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output)
	}

	if result.Project != "Test Project" {
		t.Errorf("expected project name 'Test Project', got '%s'", result.Project)
	}
	if result.Plan == nil {
		t.Fatal("expected plan in JSON output")
	}
	if result.Plan.Tasks != 5 {
		t.Errorf("expected 5 tasks, got %d", result.Plan.Tasks)
	}
}

func TestStatusCmd_JSONWithFilter(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	statusJSON = true
	statusFilter = "in_progress"
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	var result statusJSONOutput
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output)
	}

	if len(result.Plan.Items) != 1 {
		t.Errorf("expected 1 filtered task, got %d", len(result.Plan.Items))
	}
	if len(result.Plan.Items) > 0 && result.Plan.Items[0].Status != "in_progress" {
		t.Errorf("expected in_progress status, got %s", result.Plan.Items[0].Status)
	}
}

func TestStatusCmd_CombinedFilters(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	// Filter by status AND priority
	statusFilter = "pending,in_progress"
	priorityFilter = "high"
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "High Priority Task") {
		t.Errorf("expected high priority pending task, got:\n%s", output)
	}
	if !strings.Contains(output, "In Progress Task") {
		t.Errorf("expected high priority in_progress task, got:\n%s", output)
	}
	if strings.Contains(output, "Medium Priority Task") {
		t.Errorf("should not contain medium priority task, got:\n%s", output)
	}
}

func TestStatusCmd_NoMatchingFilters(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	// Filter that matches nothing
	statusFilter = "verified"
	priorityFilter = "high"
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status failed: %v", err)
		}
	})

	if !strings.Contains(output, "No tasks match the current filters") {
		t.Errorf("expected no match message, got:\n%s", output)
	}
}

func TestFilterTasks_EmptyFilters(t *testing.T) {
	tasks := []planning.Task{
		{ID: "t1", Title: "Task 1", Priority: planning.PriorityHigh},
		{ID: "t2", Title: "Task 2", Priority: planning.PriorityLow},
	}
	state := planning.NewExecutionState("p1")
	state.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusPending}
	state.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusDone}

	resetStatusFlags()
	result := filterTasks(tasks, state)

	if len(result) != 2 {
		t.Errorf("expected 2 tasks with no filters, got %d", len(result))
	}
}

func TestContainsStatus(t *testing.T) {
	statuses := []planning.TaskStatus{planning.StatusPending, planning.StatusBlocked}

	if !containsStatus(statuses, planning.StatusPending) {
		t.Error("expected to contain pending")
	}
	if !containsStatus(statuses, planning.StatusBlocked) {
		t.Error("expected to contain blocked")
	}
	if containsStatus(statuses, planning.StatusDone) {
		t.Error("should not contain done")
	}
}

func TestContainsPriority(t *testing.T) {
	priorities := []planning.TaskPriority{planning.PriorityHigh, planning.PriorityMedium}

	if !containsPriority(priorities, planning.PriorityHigh) {
		t.Error("expected to contain high")
	}
	if !containsPriority(priorities, planning.PriorityMedium) {
		t.Error("expected to contain medium")
	}
	if containsPriority(priorities, planning.PriorityLow) {
		t.Error("should not contain low")
	}
}

func TestStatusCmd_Snapshot(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	snapshotMode = true
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status --snapshot failed: %v", err)
		}
	})

	if !strings.Contains(output, "Project Snapshot") {
		t.Errorf("expected snapshot header, got:\n%s", output)
	}
	if !strings.Contains(output, "Progress:") {
		t.Errorf("expected progress in snapshot, got:\n%s", output)
	}
}

func TestStatusCmd_SnapshotJSON(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()
	defer resetStatusFlags()

	setupStatusTestData(t)

	snapshotMode = true
	statusJSON = true
	output := captureStdout(t, func() {
		if err := statusCmd.RunE(statusCmd, []string{}); err != nil {
			t.Fatalf("status --snapshot --json failed: %v", err)
		}
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, output)
	}
	if _, ok := result["progress"]; !ok {
		t.Error("expected 'progress' in JSON snapshot")
	}
	if _, ok := result["total_tasks"]; !ok {
		t.Error("expected 'total_tasks' in JSON snapshot")
	}
}

func TestTaskReady(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	setupStatusTestData(t)

	taskQueryJSON = false
	output := captureStdout(t, func() {
		if err := taskReadyCmd.RunE(taskReadyCmd, []string{}); err != nil {
			t.Fatalf("task ready failed: %v", err)
		}
	})

	if !strings.Contains(output, "Ready Tasks") {
		t.Errorf("expected Ready Tasks header, got:\n%s", output)
	}
}

func TestTaskBlocked(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	setupStatusTestData(t)

	taskQueryJSON = false
	output := captureStdout(t, func() {
		if err := taskBlockedCmd.RunE(taskBlockedCmd, []string{}); err != nil {
			t.Fatalf("task blocked failed: %v", err)
		}
	})

	if !strings.Contains(output, "Blocked Tasks") {
		t.Errorf("expected Blocked Tasks header, got:\n%s", output)
	}
}

func TestTaskInProgress(t *testing.T) {
	_, cleanup := withTempDir(t)
	defer cleanup()

	setupStatusTestData(t)

	taskQueryJSON = false
	output := captureStdout(t, func() {
		if err := taskInProgressCmd.RunE(taskInProgressCmd, []string{}); err != nil {
			t.Fatalf("task in-progress failed: %v", err)
		}
	})

	if !strings.Contains(output, "In-Progress Tasks") {
		t.Errorf("expected In-Progress Tasks header, got:\n%s", output)
	}
}

func TestHasActiveFilters(t *testing.T) {
	resetStatusFlags()
	if hasActiveFilters() {
		t.Error("expected no active filters initially")
	}

	statusFilter = "pending"
	if !hasActiveFilters() {
		t.Error("expected active filter with statusFilter set")
	}

	resetStatusFlags()
	priorityFilter = "high"
	if !hasActiveFilters() {
		t.Error("expected active filter with priorityFilter set")
	}

	resetStatusFlags()
	readyOnly = true
	if !hasActiveFilters() {
		t.Error("expected active filter with readyOnly set")
	}

	resetStatusFlags()
	blockedOnly = true
	if !hasActiveFilters() {
		t.Error("expected active filter with blockedOnly set")
	}

	resetStatusFlags()
	activeOnly = true
	if !hasActiveFilters() {
		t.Error("expected active filter with activeOnly set")
	}
}
