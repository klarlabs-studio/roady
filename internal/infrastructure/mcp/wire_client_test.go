package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// wireClient is a minimal spec-compliant MCP HTTP client for tests: it sends
// the headers real clients send and reads either a JSON or an SSE response,
// so tests hold under every HTTP mode mcp-go serves (legacy split,
// session-based Streamable HTTP, stateless 2026-07-28).
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
	// The headers a spec-compliant client sends, so the test holds under every
	// HTTP mode mcp-go serves: the legacy split ignores them, session-based
	// Streamable HTTP requires the protocol version after initialize, and the
	// stateless (2026-07-28) model requires Mcp-Method on every request.
	if m, ok := body["method"].(string); ok {
		req.Header.Set("Mcp-Method", m)
		if m != "initialize" {
			req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		}
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
