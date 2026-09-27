package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// TestToolErrorOnTheWire drives a real tools/call through the MCP server and
// inspects the bytes a client receives.
//
// TestErrorResultsOmitStructuredContent marshals the result struct directly,
// which is not the path a client sees: the server's dispatch copies the
// fields into a response map, and that copy wrote structuredContent: null for
// every error result (#92). The unit test passed while every failing tool call
// on the wire still carried the null. Only a request through the dispatch
// catches that.
func TestToolErrorOnTheWire(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServer(dir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if _, err := s.handleInit(context.Background(), InitArgs{Name: "wire"}); err != nil {
		t.Fatalf("init: %v", err)
	}

	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.ServeHTTP(ctx, addr) }()
	c := &wireClient{url: "http://" + addr + "/mcp"}
	c.waitReady(t)

	c.call(t, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "wire-test", "version": "1"},
	})
	c.notify(t, "notifications/initialized")

	raw := c.call(t, "tools/call", map[string]any{
		"name":      "roady_task",
		"arguments": map[string]any{"action": "start", "task_id": "no-such-task"},
	})

	var envelope struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode response %s: %v", raw, err)
	}
	if string(envelope.Result["isError"]) != "true" {
		t.Fatalf("expected an error result, got %s", raw)
	}
	if sc, present := envelope.Result["structuredContent"]; present && string(sc) == "null" {
		t.Errorf("error result carries structuredContent: null, which fails strict client validation (#92):\n  %s", raw)
	}
	if !bytes.Contains(envelope.Result["content"], []byte("Failed to transition")) {
		t.Errorf("error result lost its message: %s", raw)
	}
}
