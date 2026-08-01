package application

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	domainPlugin "github.com/felixgeelhaar/roady/pkg/domain/plugin"
)

// domainWorkspaceRepo embeds the full interface so stubs only implement the
// methods they actually exercise.
type domainWorkspaceRepo = domain.WorkspaceRepository

// recordingSyncer captures what would be written to the external tracker.
type recordingSyncer struct {
	pushed   map[string]planning.TaskStatus
	pushErr  map[string]error
	pushCall int
}

func newRecordingSyncer() *recordingSyncer {
	return &recordingSyncer{pushed: map[string]planning.TaskStatus{}, pushErr: map[string]error{}}
}

func (r *recordingSyncer) Init(map[string]string) error { return nil }
func (r *recordingSyncer) Sync(*planning.Plan, *planning.ExecutionState) (*domainPlugin.SyncResult, error) {
	return &domainPlugin.SyncResult{}, nil
}
func (r *recordingSyncer) Push(taskID string, status planning.TaskStatus) error {
	r.pushCall++
	if err, ok := r.pushErr[taskID]; ok {
		return err
	}
	r.pushed[taskID] = status
	return nil
}

func TestProviderFromPluginPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/usr/local/bin/roady-plugin-github", want: "github"},
		{path: "./roady-plugin-jira", want: "jira"},
		{path: "roady-plugin-linear", want: "linear"},
		// These three were previously all collapsed to "external", which
		// made their task links collide in ExternalRefs.
		{path: "roady-plugin-trello", want: "trello"},
		{path: "roady-plugin-asana", want: "asana"},
		{path: "roady-plugin-notion", want: "notion"},
		// A custom plugin keeps a stable identity of its own.
		{path: "/opt/bin/roady-plugin-acme", want: "acme"},
		{path: "/opt/bin/my-syncer", want: "my-syncer"},
		{path: "", want: "external"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := ProviderFromPluginPath(tt.path); got != tt.want {
				t.Errorf("ProviderFromPluginPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestProviderFromPluginPathIsCaseInsensitive(t *testing.T) {
	if got := ProviderFromPluginPath("/Bin/Roady-Plugin-GitHub"); got != "github" {
		t.Errorf("got %q, want github", got)
	}
}

func TestApplyOutboundPushesOnlyDivergentTasks(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates = map[string]planning.TaskResult{
		"agree":   {Status: planning.StatusDone},
		"diverge": {Status: planning.StatusBlocked},
	}

	svc := &SyncService{repo: &stubStateRepo{state: state}, pushEnabled: true}
	syncer := newRecordingSyncer()

	external := map[string]planning.TaskStatus{
		"agree":   planning.StatusDone,    // tracker already matches
		"diverge": planning.StatusPending, // Roady moved, tracker is stale
	}

	results := svc.applyOutbound(syncer, nil, external)

	if _, pushed := syncer.pushed["agree"]; pushed {
		t.Error("a task the tracker already agrees about must not be pushed")
	}
	if got := syncer.pushed["diverge"]; got != planning.StatusBlocked {
		t.Errorf("diverge pushed as %q, want blocked", got)
	}
	if len(results) != 1 || !strings.Contains(results[0], "diverge") {
		t.Errorf("unexpected results: %v", results)
	}
}

func TestApplyOutboundReportsFailuresWithoutAborting(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates = map[string]planning.TaskResult{
		"a": {Status: planning.StatusDone},
		"b": {Status: planning.StatusDone},
		"c": {Status: planning.StatusDone},
	}

	svc := &SyncService{repo: &stubStateRepo{state: state}, pushEnabled: true}
	syncer := newRecordingSyncer()
	syncer.pushErr["b"] = errBoom

	external := map[string]planning.TaskStatus{
		"a": planning.StatusPending,
		"b": planning.StatusPending,
		"c": planning.StatusPending,
	}

	results := svc.applyOutbound(syncer, nil, external)

	// One unreachable issue must not stop the rest of the run.
	if syncer.pushCall != 3 {
		t.Errorf("expected all 3 pushes attempted, got %d", syncer.pushCall)
	}
	if _, ok := syncer.pushed["c"]; !ok {
		t.Error("push after a failure should still happen")
	}

	joined := strings.Join(results, "\n")
	if !strings.Contains(joined, "Push Task b: error") {
		t.Errorf("failure not reported: %v", results)
	}
}

func TestApplyOutboundIsDeterministic(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates = map[string]planning.TaskResult{
		"z": {Status: planning.StatusDone},
		"a": {Status: planning.StatusDone},
		"m": {Status: planning.StatusDone},
	}
	external := map[string]planning.TaskStatus{
		"z": planning.StatusPending,
		"a": planning.StatusPending,
		"m": planning.StatusPending,
	}

	svc := &SyncService{repo: &stubStateRepo{state: state}, pushEnabled: true}

	first := strings.Join(svc.applyOutbound(newRecordingSyncer(), nil, external), "|")
	for range 20 {
		got := strings.Join(svc.applyOutbound(newRecordingSyncer(), nil, external), "|")
		if got != first {
			t.Fatalf("push order varies between runs:\n%s\n%s", got, first)
		}
	}
	if !strings.HasPrefix(first, "Push Task a") {
		t.Errorf("expected results sorted by task id, got %q", first)
	}
}

func TestApplyOutboundHandlesUnreadableState(t *testing.T) {
	svc := &SyncService{repo: &stubStateRepo{err: errBoom}, pushEnabled: true}

	results := svc.applyOutbound(newRecordingSyncer(), nil, map[string]planning.TaskStatus{"a": planning.StatusDone})

	if len(results) != 1 || !strings.Contains(results[0], "skipped") {
		t.Errorf("expected a skip message, got %v", results)
	}
}

// stubStateRepo satisfies only the state-loading part of the repository that
// outbound push touches.
type stubStateRepo struct {
	domainWorkspaceRepo
	state *planning.ExecutionState
	err   error
}

func (s *stubStateRepo) LoadState() (*planning.ExecutionState, error) { return s.state, s.err }

// fieldSyncer implements the optional FieldSyncer extension.
type fieldSyncer struct {
	*recordingSyncer
	fields map[string]domainPlugin.TaskFields
}

func newFieldSyncer() *fieldSyncer {
	return &fieldSyncer{
		recordingSyncer: newRecordingSyncer(),
		fields:          map[string]domainPlugin.TaskFields{},
	}
}

func (f *fieldSyncer) PushFields(taskID string, fields domainPlugin.TaskFields) error {
	f.fields[taskID] = fields
	return f.Push(taskID, fields.Status)
}

func TestPushTaskUsesFieldsWhenSupported(t *testing.T) {
	fs := newFieldSyncer()

	if err := pushTask(fs, "t1", planning.StatusDone, planning.PriorityHigh); err != nil {
		t.Fatalf("pushTask: %v", err)
	}

	got, ok := fs.fields["t1"]
	if !ok {
		t.Fatal("PushFields was not called on a plugin that implements it")
	}
	if got.Priority != planning.PriorityHigh || got.Status != planning.StatusDone {
		t.Errorf("fields = %+v, want status=done priority=high", got)
	}
}

func TestPushTaskFallsBackForPlainSyncers(t *testing.T) {
	// A plugin that only implements Syncer must still receive its status
	// update; attribute support is additive, not a compatibility break.
	plain := newRecordingSyncer()

	if err := pushTask(plain, "t1", planning.StatusBlocked, planning.PriorityHigh); err != nil {
		t.Fatalf("pushTask: %v", err)
	}

	if plain.pushed["t1"] != planning.StatusBlocked {
		t.Errorf("status not pushed through the plain interface: %v", plain.pushed)
	}
}

func TestApplyOutboundSendsPlanPriority(t *testing.T) {
	state := planning.NewExecutionState("plan-1")
	state.TaskStates = map[string]planning.TaskResult{"t1": {Status: planning.StatusDone}}

	// Priority is plan data, not execution state, so it must be read from
	// the plan rather than from state.json.
	plan := &planning.Plan{Tasks: []planning.Task{{ID: "t1", Priority: planning.PriorityLow}}}

	svc := &SyncService{repo: &stubStateRepo{state: state}, pushEnabled: true}
	fs := newFieldSyncer()

	svc.applyOutbound(fs, plan, map[string]planning.TaskStatus{"t1": planning.StatusPending})

	if fs.fields["t1"].Priority != planning.PriorityLow {
		t.Errorf("priority = %q, want low", fs.fields["t1"].Priority)
	}
}
