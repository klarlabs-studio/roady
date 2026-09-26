//go:build mcpwire

// This test is behind the mcpwire build tag until go.klarlabs.de/mcp is bumped
// to a release containing the dispatch fix (branch
// fix/omit-nil-structured-content on felixgeelhaar/mcp-go). Against the pinned
// v1.24.1 it fails, correctly. Remove the tag with the bump; the roady task
// task-mcp-error-live-path stays blocked until then.
//
//	go test -tags mcpwire ./internal/infrastructure/mcp/ -run TestToolErrorOnTheWire

package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
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
		"name":      "roady_task_transition",
		"arguments": map[string]any{"task_id": "no-such-task", "event": "start"},
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

type wireClient struct {
	url     string
	session string
	nextID  int
}

func (c *wireClient) post(t *testing.T, body map[string]any) []byte {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post %v: %v", body["method"], err)
	}
	defer func() { _ = resp.Body.Close() }()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.session = sid
	}
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	// Streamable HTTP may answer as a single JSON body or as an SSE stream.
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		sc := bufio.NewScanner(&buf)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data:") {
				return []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		return nil
	}
	return buf.Bytes()
}

func (c *wireClient) call(t *testing.T, method string, params map[string]any) []byte {
	t.Helper()
	c.nextID++
	return c.post(t, map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method, "params": params})
}

func (c *wireClient) notify(t *testing.T, method string) {
	t.Helper()
	c.post(t, map[string]any{"jsonrpc": "2.0", "method": method})
}

func (c *wireClient) waitReady(t *testing.T) {
	t.Helper()
	health := strings.TrimSuffix(c.url, "/mcp") + "/health"
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(health); err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server at %s did not come up", c.url)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	return fmt.Sprintf("127.0.0.1:%d", l.Addr().(*net.TCPAddr).Port)
}
