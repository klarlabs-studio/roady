package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// --- Audit (events) round-trip tests ---

func TestRecordAndLoadEvents(t *testing.T) {
	dir := t.TempDir()
	repo := NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		events []domain.Event
	}{
		{"empty", nil},
		{"single", []domain.Event{{ID: "e1", Action: "task.start"}}},
		{"multiple", []domain.Event{
			{ID: "e1", Action: "task.start"},
			{ID: "e2", Action: "task.complete"},
			{ID: "e3", Action: "plan.generate"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			r := NewFilesystemRepository(d)
			if err := r.Initialize(); err != nil {
				t.Fatal(err)
			}

			for _, ev := range tt.events {
				if err := r.RecordEvent(ev); err != nil {
					t.Fatalf("RecordEvent: %v", err)
				}
			}

			loaded, err := r.LoadEvents()
			if err != nil {
				t.Fatalf("LoadEvents: %v", err)
			}
			if len(loaded) != len(tt.events) {
				t.Errorf("expected %d events, got %d", len(tt.events), len(loaded))
			}
			for i, ev := range tt.events {
				if loaded[i].ID != ev.ID {
					t.Errorf("event[%d] ID = %s, want %s", i, loaded[i].ID, ev.ID)
				}
			}
		})
	}
}

func TestLoadEvents_MissingFile(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	if err := r.Initialize(); err != nil {
		t.Fatal(err)
	}

	events, err := r.LoadEvents()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty events, got %d", len(events))
	}
}

func TestLoadEvents_MalformedLines(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	if err := r.Initialize(); err != nil {
		t.Fatal(err)
	}

	if err := r.RecordEvent(domain.Event{ID: "good", Action: "test"}); err != nil {
		t.Fatal(err)
	}

	path, _ := r.ResolvePath(EventsFile)
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if _, err := f.Write([]byte("NOT JSON\n")); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	if err := r.RecordEvent(domain.Event{ID: "good2", Action: "test2"}); err != nil {
		t.Fatal(err)
	}

	events, err := r.LoadEvents()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(events) != 2 {
		t.Errorf("expected 2 good events (skipping malformed), got %d", len(events))
	}
}

// --- Plan round-trip tests ---

func TestSaveAndLoadPlan(t *testing.T) {
	tests := []struct {
		name string
		plan *planning.Plan
	}{
		{"empty tasks", &planning.Plan{ID: "p1", Tasks: []planning.Task{}}},
		{"with tasks", &planning.Plan{
			ID:     "p2",
			SpecID: "s1",
			Tasks: []planning.Task{
				{ID: "t1", Title: "Task 1", Priority: planning.PriorityHigh},
				{ID: "t2", Title: "Task 2", DependsOn: []string{"t1"}},
			},
			ApprovalStatus: planning.ApprovalApproved,
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			r := NewFilesystemRepository(d)
			if err := r.Initialize(); err != nil {
				t.Fatal(err)
			}

			if err := r.SavePlan(tt.plan); err != nil {
				t.Fatalf("SavePlan: %v", err)
			}
			loaded, err := r.LoadPlan()
			if err != nil {
				t.Fatalf("LoadPlan: %v", err)
			}
			if loaded.ID != tt.plan.ID {
				t.Errorf("ID = %s, want %s", loaded.ID, tt.plan.ID)
			}
			if len(loaded.Tasks) != len(tt.plan.Tasks) {
				t.Errorf("tasks = %d, want %d", len(loaded.Tasks), len(tt.plan.Tasks))
			}
		})
	}
}

func TestLoadPlan_Missing(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	if err := r.Initialize(); err != nil {
		t.Fatal(err)
	}

	plan, err := r.LoadPlan()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if plan != nil {
		t.Error("expected nil plan for missing file")
	}
}

func TestLoadPlan_InvalidJSON(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	if err := r.Initialize(); err != nil {
		t.Fatal(err)
	}

	path, _ := r.ResolvePath(PlanFile)
	if err := os.WriteFile(path, []byte("{invalid"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := r.LoadPlan()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- State round-trip tests ---

func TestSaveAndLoadState(t *testing.T) {
	tests := []struct {
		name  string
		state *planning.ExecutionState
	}{
		{"empty", planning.NewExecutionState("p1")},
		{"with tasks", func() *planning.ExecutionState {
			s := planning.NewExecutionState("p1")
			s.TaskStates["t1"] = planning.TaskResult{Status: planning.StatusDone, Owner: "alice"}
			s.TaskStates["t2"] = planning.TaskResult{Status: planning.StatusInProgress}
			return s
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			r := NewFilesystemRepository(d)
			_ = r.Initialize()

			if err := r.SaveState(tt.state); err != nil {
				t.Fatalf("SaveState: %v", err)
			}
			loaded, err := r.LoadState()
			if err != nil {
				t.Fatalf("LoadState: %v", err)
			}
			if loaded.ProjectID != tt.state.ProjectID {
				t.Errorf("ProjectID = %s, want %s", loaded.ProjectID, tt.state.ProjectID)
			}
			if len(loaded.TaskStates) != len(tt.state.TaskStates) {
				t.Errorf("task states = %d, want %d", len(loaded.TaskStates), len(tt.state.TaskStates))
			}
		})
	}
}

func TestLoadState_Missing(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	_ = r.Initialize()

	state, err := r.LoadState()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if state.ProjectID != "unknown" {
		t.Errorf("expected default state with project 'unknown', got %s", state.ProjectID)
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	_ = r.Initialize()

	path, _ := r.ResolvePath(StateFile)
	if err := os.WriteFile(path, []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := r.LoadState()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// --- Policy round-trip tests ---

func TestLoadPolicy_MissingReturnsDefaults(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	_ = r.Initialize()

	pol, err := r.LoadPolicy()
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	if pol.MaxWIP != 3 {
		t.Errorf("default MaxWIP = %d, want 3", pol.MaxWIP)
	}
	if !pol.AllowAI {
		t.Error("default AllowAI should be true")
	}
}

func TestLoadPolicy_Legacy(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	_ = r.Initialize()

	path, _ := r.ResolvePath(PolicyFile)
	if err := os.WriteFile(path, []byte("max_wip: 5\nallow_ai: true\nai_provider: openai\nai_model: gpt-4\n"), 0600); err != nil {
		t.Fatal(err)
	}

	pol, err := r.LoadPolicy()
	if err != nil {
		t.Fatalf("LoadPolicy legacy: %v", err)
	}
	if pol.MaxWIP != 5 {
		t.Errorf("MaxWIP = %d, want 5", pol.MaxWIP)
	}
}

// --- Plugin config round-trip tests ---

// --- Codebase inspector tests ---

func TestCodebaseInspector_FileExists(t *testing.T) {
	inspector := NewCodebaseInspector()

	tests := []struct {
		name   string
		path   string
		exists bool
	}{
		{"existing file", os.Args[0], true},
		{"nonexistent file", "/nonexistent/path/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists, err := inspector.FileExists(tt.path)
			if err != nil {
				t.Fatalf("FileExists: %v", err)
			}
			if exists != tt.exists {
				t.Errorf("FileExists(%s) = %v, want %v", tt.path, exists, tt.exists)
			}
		})
	}
}

func TestCodebaseInspector_FileNotEmpty(t *testing.T) {
	inspector := NewCodebaseInspector()

	d := t.TempDir()
	emptyFile := filepath.Join(d, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	nonEmptyFile := filepath.Join(d, "notempty.txt")
	if err := os.WriteFile(nonEmptyFile, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		path     string
		notEmpty bool
	}{
		{"empty file", emptyFile, false},
		{"non-empty file", nonEmptyFile, true},
		{"nonexistent", "/no/such/file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := inspector.FileNotEmpty(tt.path)
			if err != nil {
				t.Fatalf("FileNotEmpty: %v", err)
			}
			if result != tt.notEmpty {
				t.Errorf("FileNotEmpty(%s) = %v, want %v", tt.path, result, tt.notEmpty)
			}
		})
	}
}

func TestCodebaseInspector_GitStatus(t *testing.T) {
	inspector := NewCodebaseInspector()

	// Test with a nonexistent file in the current repo
	status, err := inspector.GitStatus("/nonexistent/path/file.txt")
	if err != nil {
		t.Fatalf("GitStatus: %v", err)
	}
	// Should return one of: clean, untracked, error, unknown
	validStatuses := map[string]bool{
		"clean": true, "untracked": true, "error": true,
		"unknown": true, "modified": true, "missing": true,
	}
	if !validStatuses[status] {
		t.Errorf("unexpected git status: %s", status)
	}
}

// --- State save error handling ---

func TestSaveState_ReadonlyDir(t *testing.T) {
	d := t.TempDir()
	r := NewFilesystemRepository(d)
	_ = r.Initialize()

	if err := os.Chmod(filepath.Join(d, RoadyDir), 0400); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(filepath.Join(d, RoadyDir), 0700); err != nil {
			t.Fatal(err)
		}
	}()

	err := r.SaveState(planning.NewExecutionState("p1"))
	if err == nil {
		t.Error("expected write error on readonly dir")
	}
}

func TestSaveAndLoadPolicy(t *testing.T) {
	tests := []struct {
		name   string
		policy *domain.PolicyConfig
	}{
		{"defaults", &domain.PolicyConfig{MaxWIP: 3, AllowAI: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := t.TempDir()
			r := NewFilesystemRepository(d)
			_ = r.Initialize()

			if err := r.SavePolicy(tt.policy); err != nil {
				t.Fatalf("SavePolicy: %v", err)
			}
			loaded, err := r.LoadPolicy()
			if err != nil {
				t.Fatalf("LoadPolicy: %v", err)
			}
			if loaded.MaxWIP != tt.policy.MaxWIP {
				t.Errorf("MaxWIP = %d, want %d", loaded.MaxWIP, tt.policy.MaxWIP)
			}
			if loaded.AllowAI != tt.policy.AllowAI {
				t.Errorf("AllowAI = %v, want %v", loaded.AllowAI, tt.policy.AllowAI)
			}
		})
	}
}
