package sdk

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.klarlabs.de/mcp/protocol"
)

// mockTransport implements client.Transport and returns canned responses
// based on the method name in the request.
type mockTransport struct {
	closed    bool
	responses map[string]any // method -> result for Response
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		responses: make(map[string]any),
	}
}

// setToolResponse configures a mock response for a tools/call request.
// The result simulates what the MCP server returns for CallTool.
func (m *mockTransport) setToolResponse(text string, isError bool) {
	content := []any{
		map[string]any{"type": "text", "text": text},
	}
	result := map[string]any{"content": content}
	if isError {
		result["isError"] = true
	}
	m.responses["tools/call"] = result
}

// setResourceResponse configures a mock response for resources/read.
func (m *mockTransport) setResourceResponse(text string) {
	m.responses["resources/read"] = map[string]any{
		"contents": []any{
			map[string]any{"uri": "roady://schema", "text": text},
		},
	}
}

func (m *mockTransport) Send(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
	result, ok := m.responses[req.Method]
	if !ok {
		// Return a default initialize response for the handshake
		if req.Method == "initialize" {
			return protocol.NewResponse(req.ID, map[string]any{
				"serverInfo":      map[string]any{"name": "mock", "version": "1.0.0"},
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}), nil
		}
		// For notifications, just return nil
		if req.IsNotification() {
			return nil, nil
		}
		// Default empty tool result
		return protocol.NewResponse(req.ID, map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
		}), nil
	}
	return protocol.NewResponse(req.ID, result), nil
}

func (m *mockTransport) Close() error {
	m.closed = true
	return nil
}

// helper to create an initialized client
func newTestClient(t *testing.T, mt *mockTransport) *Client {
	t.Helper()
	c := NewClient(mt)
	ctx := context.Background()
	if _, err := c.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// --- Tests for text-returning methods ---

func TestClient_Init(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Initialized project test-proj", false)
	c := newTestClient(t, mt)

	msg, err := c.Init(context.Background(), "test-proj")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if msg != "Initialized project test-proj" {
		t.Errorf("got %q, want %q", msg, "Initialized project test-proj")
	}
}

func TestClient_GeneratePlan(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Plan generated with 5 tasks", false)
	c := newTestClient(t, mt)

	msg, err := c.GeneratePlan(context.Background())
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if msg != "Plan generated with 5 tasks" {
		t.Errorf("got %q", msg)
	}
}

func TestClient_ApprovePlan(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Plan approved", false)
	c := newTestClient(t, mt)

	msg, err := c.ApprovePlan(context.Background())
	if err != nil {
		t.Fatalf("ApprovePlan: %v", err)
	}
	if msg != "Plan approved" {
		t.Errorf("got %q", msg)
	}
}

func TestClient_AcceptDrift(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Drift accepted", false)
	c := newTestClient(t, mt)

	msg, err := c.AcceptDrift(context.Background())
	if err != nil {
		t.Fatalf("AcceptDrift: %v", err)
	}
	if msg != "Drift accepted" {
		t.Errorf("got %q", msg)
	}
}

func TestClient_ExplainDrift(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("The spec has diverged from the plan because...", false)
	c := newTestClient(t, mt)

	msg, err := c.ExplainDrift(context.Background())
	if err != nil {
		t.Fatalf("ExplainDrift: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty explanation")
	}
}

func TestClient_ExplainSpec(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("This spec defines 3 features...", false)
	c := newTestClient(t, mt)

	msg, err := c.ExplainSpec(context.Background())
	if err != nil {
		t.Fatalf("ExplainSpec: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty explanation")
	}
}

func TestClient_Status(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("3 pending, 2 in progress, 1 done", false)
	c := newTestClient(t, mt)

	msg, err := c.Status(context.Background(), nil)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty status")
	}
}

func TestClient_CheckPolicy(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("No policy violations found.", false)
	c := newTestClient(t, mt)

	msg, err := c.CheckPolicy(context.Background())
	if err != nil {
		t.Fatalf("CheckPolicy: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_TransitionTask(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Task task-1 transitioned to in_progress", false)
	c := newTestClient(t, mt)

	msg, err := c.TransitionTask(context.Background(), "task-1", "start", "evidence")
	if err != nil {
		t.Fatalf("TransitionTask: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_TransitionTask_NoEvidence(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Task task-1 transitioned", false)
	c := newTestClient(t, mt)

	msg, err := c.TransitionTask(context.Background(), "task-1", "start", "")
	if err != nil {
		t.Fatalf("TransitionTask: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_AddFeature(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Feature added: Auth", false)
	c := newTestClient(t, mt)

	msg, err := c.AddFeature(context.Background(), "Auth", "User authentication")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_QueryProject(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("The project has 5 features...", false)
	c := newTestClient(t, mt)

	msg, err := c.QueryProject(context.Background(), "How many features?")
	if err != nil {
		t.Fatalf("QueryProject: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_GetReadyTasks(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("task-1, task-2", false)
	c := newTestClient(t, mt)

	msg, err := c.GetReadyTasks(context.Background())
	if err != nil {
		t.Fatalf("GetReadyTasks: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_GetBlockedTasks(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("task-3", false)
	c := newTestClient(t, mt)

	msg, err := c.GetBlockedTasks(context.Background())
	if err != nil {
		t.Fatalf("GetBlockedTasks: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_GetInProgressTasks(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("task-4", false)
	c := newTestClient(t, mt)

	msg, err := c.GetInProgressTasks(context.Background())
	if err != nil {
		t.Fatalf("GetInProgressTasks: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_GitSync(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Synced 3 tasks from git", false)
	c := newTestClient(t, mt)

	msg, err := c.GitSync(context.Background())
	if err != nil {
		t.Fatalf("GitSync: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

// --- Tests for JSON-returning methods ---

func TestClient_GetSpec(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"id":"s1","title":"Test Project","features":[]}`, false)
	c := newTestClient(t, mt)

	spec, err := c.GetSpec(context.Background())
	if err != nil {
		t.Fatalf("GetSpec: %v", err)
	}
	if spec.ID != "s1" {
		t.Errorf("got ID %q, want %q", spec.ID, "s1")
	}
}

func TestClient_GetPlan(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"id":"p1","tasks":[{"id":"t1","title":"Task 1"}]}`, false)
	c := newTestClient(t, mt)

	plan, err := c.GetPlan(context.Background())
	if err != nil {
		t.Fatalf("GetPlan: %v", err)
	}
	if plan.ID != "p1" {
		t.Errorf("got ID %q, want %q", plan.ID, "p1")
	}
}

func TestClient_GetState(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"task_states":{"t1":{"status":"pending"}}}`, false)
	c := newTestClient(t, mt)

	state, err := c.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.TaskStates) != 1 {
		t.Errorf("got %d states, want 1", len(state.TaskStates))
	}
}

func TestClient_DetectDrift(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"issues":[{"id":"d1","type":"missing"}]}`, false)
	c := newTestClient(t, mt)

	report, err := c.DetectDrift(context.Background())
	if err != nil {
		t.Fatalf("DetectDrift: %v", err)
	}
	if len(report.Issues) != 1 {
		t.Errorf("got %d issues, want 1", len(report.Issues))
	}
}

func TestClient_SuggestPriorities(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"suggestions":[{"task_id":"t1","suggested_priority":"high","reason":"critical"}]}`, false)
	c := newTestClient(t, mt)

	ps, err := c.SuggestPriorities(context.Background())
	if err != nil {
		t.Fatalf("SuggestPriorities: %v", err)
	}
	if len(ps.Suggestions) != 1 {
		t.Errorf("got %d suggestions, want 1", len(ps.Suggestions))
	}
}

func TestClient_ReviewSpec(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"score":90,"summary":"Good","findings":[]}`, false)
	c := newTestClient(t, mt)

	review, err := c.ReviewSpec(context.Background())
	if err != nil {
		t.Fatalf("ReviewSpec: %v", err)
	}
	if review.Score != 90 {
		t.Errorf("got score %d, want 90", review.Score)
	}
}

func TestClient_GetSnapshot(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"progress":0.5,"total_tasks":10}`, false)
	c := newTestClient(t, mt)

	snap, err := c.GetSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.TotalTasks != 10 {
		t.Errorf("got total_tasks %d, want 10", snap.TotalTasks)
	}
}

func TestClient_SmartDecompose(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"tasks":[{"id":"st1","title":"Smart Task"}],"summary":"AI plan"}`, false)
	c := newTestClient(t, mt)

	sp, err := c.SmartDecompose(context.Background())
	if err != nil {
		t.Fatalf("SmartDecompose: %v", err)
	}
	if len(sp.Tasks) != 1 {
		t.Errorf("got %d tasks, want 1", len(sp.Tasks))
	}
}

func TestClient_UpdatePlan(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Plan updated with 2 tasks", false)
	c := newTestClient(t, mt)

	msg, err := c.UpdatePlan(context.Background(), []Task{
		{ID: "t1", Title: "Task 1"},
		{ID: "t2", Title: "Task 2"},
	})
	if err != nil {
		t.Fatalf("UpdatePlan: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

// --- Typed helper methods ---

func TestClient_StatusTyped(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"total_tasks":10,"filtered_count":3,"tasks":[{"id":"t1","title":"Task","status":"pending"}]}`, false)
	c := newTestClient(t, mt)

	sr, err := c.StatusTyped(context.Background(), StatusRequest{Status: "pending", Limit: 5})
	if err != nil {
		t.Fatalf("StatusTyped: %v", err)
	}
	if sr.TotalTasks != 10 {
		t.Errorf("got total %d, want 10", sr.TotalTasks)
	}
}

func TestClient_StatusTyped_AllFilters(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`{"total_tasks":5,"tasks":[]}`, false)
	c := newTestClient(t, mt)

	_, err := c.StatusTyped(context.Background(), StatusRequest{
		Status:   "in_progress",
		Priority: "high",
		Ready:    true,
		Blocked:  true,
		Active:   true,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("StatusTyped: %v", err)
	}
}

func TestClient_TransitionTaskTyped(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("Transitioned", false)
	c := newTestClient(t, mt)

	msg, err := c.TransitionTaskTyped(context.Background(), TransitionRequest{
		TaskID:   "task-1",
		Event:    "start",
		Evidence: "proof",
		Actor:    "alice",
	})
	if err != nil {
		t.Fatalf("TransitionTaskTyped: %v", err)
	}
	if msg == "" {
		t.Error("expected non-empty result")
	}
}

func TestClient_CheckPolicyTyped_NoViolations(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("No policy violations found.", false)
	c := newTestClient(t, mt)

	vs, err := c.CheckPolicyTyped(context.Background())
	if err != nil {
		t.Fatalf("CheckPolicyTyped: %v", err)
	}
	if vs != nil {
		t.Errorf("expected nil violations, got %v", vs)
	}
}

func TestClient_CheckPolicyTyped_WithViolations(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse(`[{"rule":"max_wip","message":"Too many WIP tasks"}]`, false)
	c := newTestClient(t, mt)

	vs, err := c.CheckPolicyTyped(context.Background())
	if err != nil {
		t.Fatalf("CheckPolicyTyped: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("got %d violations, want 1", len(vs))
	}
}

// --- Error path ---

func TestClient_ToolError(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("something went wrong", true)
	c := newTestClient(t, mt)

	_, err := c.Init(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for IsError response")
	}
	toolErr, ok := err.(*ToolError)
	if !ok {
		t.Fatalf("expected ToolError, got %T: %v", err, err)
	}
	if toolErr.Message != "something went wrong" {
		t.Errorf("got message %q, want %q", toolErr.Message, "something went wrong")
	}
}

// --- Schema / Compatible ---

func TestClient_GetSchema(t *testing.T) {
	mt := newMockTransport()
	mt.setResourceResponse(`{"schema_version":"1.2.0","server_version":"0.9.0","changelog":"https://example.com"}`)
	c := newTestClient(t, mt)

	schema, err := c.GetSchema(context.Background())
	if err != nil {
		t.Fatalf("GetSchema: %v", err)
	}
	if schema.SchemaVersion != "1.2.0" {
		t.Errorf("got version %q, want %q", schema.SchemaVersion, "1.2.0")
	}
}

func TestClient_Compatible(t *testing.T) {
	mt := newMockTransport()
	// Same major as SupportedSchemaMajor, newer minor: compatible.
	mt.setResourceResponse(`{"schema_version":"4.2.0","server_version":"0.15.0","changelog":"https://example.com"}`)
	c := newTestClient(t, mt)

	if err := c.Compatible(context.Background()); err != nil {
		t.Fatalf("Compatible: %v", err)
	}
}

func TestClient_Compatible_Incompatible(t *testing.T) {
	mt := newMockTransport()
	// A server still on the previous major must be rejected.
	mt.setResourceResponse(`{"schema_version":"3.8.0","server_version":"0.14.0","changelog":"https://example.com"}`)
	c := newTestClient(t, mt)

	err := c.Compatible(context.Background())
	if err == nil {
		t.Fatal("expected error for incompatible schema")
	}
}

// --- Close ---

func TestClient_Close(t *testing.T) {
	mt := newMockTransport()
	c := NewClient(mt)
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !mt.closed {
		t.Error("expected transport to be closed")
	}
}

// --- Options ---

func TestNewClient_DefaultOptions(t *testing.T) {
	mt := newMockTransport()
	c := NewClient(mt)
	if c.timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", c.timeout)
	}
}

func TestNewClient_CustomTimeout(t *testing.T) {
	mt := newMockTransport()
	c := NewClient(mt, WithTimeout(60*time.Second))
	if c.timeout != 60*time.Second {
		t.Errorf("expected timeout 60s, got %v", c.timeout)
	}
}

func TestNewClient_CustomRetry(t *testing.T) {
	mt := newMockTransport()
	c := NewClient(mt, WithRetry(5, 100*time.Millisecond))
	if c.retryCfg.MaxAttempts != 5 {
		t.Errorf("expected max attempts 5, got %d", c.retryCfg.MaxAttempts)
	}
}

// --- Constants ---

// TestSupportedSchemaMajor pins the SDK's declared major so it cannot drift
// silently from the server's SchemaVersion in
// internal/infrastructure/mcp/schema.go. That package is internal and cannot
// be imported here, so the literal is the coupling — when the server's major
// changes, this test is the reminder to move the SDK with it.
func TestSupportedSchemaMajor(t *testing.T) {
	if SupportedSchemaMajor != "4" {
		t.Errorf("expected '4', got %q", SupportedSchemaMajor)
	}
}

func TestErrNoContent_Message(t *testing.T) {
	if ErrNoContent.Error() != "roady: empty tool result" {
		t.Errorf("unexpected: %s", ErrNoContent.Error())
	}
}

// Ensure json import is used
var _ = json.Marshal
