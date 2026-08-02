package planning

import "testing"

func TestParseDependencyRef(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantProject string
		wantTask    string
		wantExt     bool
	}{
		{
			name:     "a bare id is local",
			raw:      "task-auth-signup",
			wantTask: "task-auth-signup",
		},
		{
			name:        "@project:task names another sub-project",
			raw:         "@feature-auth:task-signup",
			wantProject: "feature-auth",
			wantTask:    "task-signup",
			wantExt:     true,
		},
		{
			name:        "surrounding whitespace tolerated",
			raw:         "  @feature-auth:task-signup  ",
			wantProject: "feature-auth",
			wantTask:    "task-signup",
			wantExt:     true,
		},
		{
			// The legacy form predates the @ sigil and is still in the wild;
			// dropping it would silently turn a real dependency into a
			// dangling local reference.
			name:        "legacy project:task without the sigil still parses",
			raw:         "feature-auth:task-signup",
			wantProject: "feature-auth",
			wantTask:    "task-signup",
			wantExt:     true,
		},
		{
			name:     "a task id containing a colon but no project stays local",
			raw:      "@:task-signup",
			wantTask: "@:task-signup",
		},
		{
			name:     "a project with no task is not a usable reference",
			raw:      "@feature-auth:",
			wantTask: "@feature-auth:",
		},
		{
			name:     "empty",
			raw:      "",
			wantTask: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDependencyRef(tt.raw)

			if got.IsExternal() != tt.wantExt {
				t.Fatalf("IsExternal() = %v, want %v (%+v)", got.IsExternal(), tt.wantExt, got)
			}
			if got.Project != tt.wantProject {
				t.Errorf("Project = %q, want %q", got.Project, tt.wantProject)
			}
			if got.TaskID != tt.wantTask {
				t.Errorf("TaskID = %q, want %q", got.TaskID, tt.wantTask)
			}
		})
	}
}

func TestDependencyRefString(t *testing.T) {
	tests := []struct {
		ref  DependencyRef
		want string
	}{
		{ref: DependencyRef{TaskID: "task-1"}, want: "task-1"},
		{ref: DependencyRef{Project: "auth", TaskID: "task-1"}, want: "@auth:task-1"},
	}

	for _, tt := range tests {
		if got := tt.ref.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

// TestParseDependencyRefRoundTrips is the property that keeps the canonical
// form stable: anything String() emits must parse back to the same reference,
// or a plan rewritten by the reconciler would drift from what was authored.
func TestParseDependencyRefRoundTrips(t *testing.T) {
	refs := []DependencyRef{
		{TaskID: "task-1"},
		{Project: "feature-auth", TaskID: "task-signup"},
	}

	for _, ref := range refs {
		got := ParseDependencyRef(ref.String())
		if got != ref {
			t.Errorf("%+v -> %q -> %+v", ref, ref.String(), got)
		}
	}
}

func TestValidateDAGIgnoresExternalDependencies(t *testing.T) {
	// A cross-project edge cannot be resolved from inside one plan, so local
	// cycle detection must skip it rather than treat it as a dangling task.
	plan := &Plan{Tasks: []Task{
		{ID: "task-a", Title: "A", DependsOn: []string{"@other:task-x"}},
		{ID: "task-b", Title: "B", DependsOn: []string{"task-a"}},
	}}

	if err := plan.ValidateDAG(); err != nil {
		t.Errorf("an external dependency should not break local DAG validation: %v", err)
	}
}

func TestValidateDAGStillCatchesLocalCycles(t *testing.T) {
	plan := &Plan{Tasks: []Task{
		{ID: "task-a", Title: "A", DependsOn: []string{"task-b"}},
		{ID: "task-b", Title: "B", DependsOn: []string{"task-a"}},
	}}

	if err := plan.ValidateDAG(); err == nil {
		t.Error("expected a cycle to be reported")
	}
}

func TestExternalDependencies(t *testing.T) {
	task := Task{ID: "t1", DependsOn: []string{
		"task-local", "@auth:task-signup", "billing:task-invoice",
	}}

	ext := task.ExternalDependencies()

	if len(ext) != 2 {
		t.Fatalf("expected 2 external dependencies, got %d: %+v", len(ext), ext)
	}
	if ext[0].Project != "auth" || ext[1].Project != "billing" {
		t.Errorf("unexpected projects: %+v", ext)
	}
}
