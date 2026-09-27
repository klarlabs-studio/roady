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

// --- Tests for JSON-returning methods ---

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

// --- Error path ---

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
	mt.setResourceResponse(`{"schema_version":"5.2.0","server_version":"0.15.0","changelog":"https://example.com"}`)
	c := newTestClient(t, mt)

	if err := c.Compatible(context.Background()); err != nil {
		t.Fatalf("Compatible: %v", err)
	}
}

func TestClient_Compatible_Incompatible(t *testing.T) {
	mt := newMockTransport()
	// A server still on the previous major must be rejected.
	mt.setResourceResponse(`{"schema_version":"4.0.0","server_version":"0.14.0","changelog":"https://example.com"}`)
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
	if SupportedSchemaMajor != "5" {
		t.Errorf("expected '5', got %q", SupportedSchemaMajor)
	}
}

func TestErrNoContent_Message(t *testing.T) {
	if ErrNoContent.Error() != "roady: empty tool result" {
		t.Errorf("unexpected: %s", ErrNoContent.Error())
	}
}

// Ensure json import is used
var _ = json.Marshal

func TestClient_ToolError(t *testing.T) {
	mt := newMockTransport()
	mt.setToolResponse("something went wrong", true)
	c := newTestClient(t, mt)

	_, err := c.Next(context.Background(), "")
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

func TestClient_AgentLoopCalls(t *testing.T) {
	calls := []struct {
		name string
		call func(c *Client) (string, error)
	}{
		{"Next", func(c *Client) (string, error) { return c.Next(context.Background(), "me") }},
		{"Capture", func(c *Client) (string, error) {
			return c.Capture(context.Background(), map[string]any{"tasks": []any{}}, true)
		}},
		{"PlanImport", func(c *Client) (string, error) { return c.PlanImport(context.Background(), "plan.md", nil) }},
		{"TaskCheck", func(c *Client) (string, error) { return c.TaskCheck(context.Background(), "task-1") }},
		{"DispatchTask", func(c *Client) (string, error) { return c.DispatchTask(context.Background(), "task-1", "sub", true) }},
		{"SemanticDrift", func(c *Client) (string, error) { return c.SemanticDrift(context.Background()) }},
		{"RecordSemanticDrift", func(c *Client) (string, error) {
			return c.RecordSemanticDrift(context.Background(), []map[string]any{{"requirement_id": "r", "agrees": true}})
		}},
	}
	for _, tc := range calls {
		mt := newMockTransport()
		mt.setToolResponse(`{"ok":true}`, false)
		out, err := tc.call(newTestClient(t, mt))
		if err != nil || out == "" {
			t.Errorf("%s: %q %v", tc.name, out, err)
		}
	}
}

func TestClient_NounMethods(t *testing.T) {
	ctx := context.Background()
	calls := map[string]func(c *Client) (string, error){
		"Task":        func(c *Client) (string, error) { return c.Task(ctx, "list", nil) },
		"Plan":        func(c *Client) (string, error) { return c.Plan(ctx, "get", nil) },
		"Spec":        func(c *Client) (string, error) { return c.Spec(ctx, "get", nil) },
		"Drift":       func(c *Client) (string, error) { return c.Drift(ctx, "detect", nil) },
		"Audit":       func(c *Client) (string, error) { return c.Audit(ctx, "verify", map[string]any{"baseline": "HEAD"}) },
		"State":       func(c *Client) (string, error) { return c.State(ctx, "get", nil) },
		"PolicyCheck": func(c *Client) (string, error) { return c.PolicyCheck(ctx) },
		"GitSync":     func(c *Client) (string, error) { return c.GitSync(ctx) },
		"Init":        func(c *Client) (string, error) { return c.Init(ctx, "demo") },
	}
	for name, call := range calls {
		mt := newMockTransport()
		mt.setToolResponse(`{"ok":true}`, false)
		if out, err := call(newTestClient(t, mt)); err != nil || out == "" {
			t.Errorf("%s: %q %v", name, out, err)
		}
	}
}
