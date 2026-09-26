package sdk

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.klarlabs.de/mcp/client"
)

func TestTextResult(t *testing.T) {
	t.Run("extracts text", func(t *testing.T) {
		r := &client.ToolResult{
			Content: []client.ContentItem{{Type: "text", Text: "hello"}},
		}
		got, err := textResult(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "hello" {
			t.Fatalf("got %q, want %q", got, "hello")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		r := &client.ToolResult{}
		_, err := textResult(r)
		if err != ErrNoContent {
			t.Fatalf("got %v, want ErrNoContent", err)
		}
	})
}

func TestUnmarshalText(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		r := &client.ToolResult{
			Content: []client.ContentItem{{Type: "text", Text: `{"id":"s1","title":"My Spec"}`}},
		}
		spec, err := unmarshalText[Spec](r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if spec.ID != "s1" || spec.Title != "My Spec" {
			t.Fatalf("unexpected spec: %+v", spec)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := &client.ToolResult{
			Content: []client.ContentItem{{Type: "text", Text: "not json"}},
		}
		_, err := unmarshalText[Spec](r)
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		r := &client.ToolResult{}
		_, err := unmarshalText[Spec](r)
		if err != ErrNoContent {
			t.Fatalf("got %v, want ErrNoContent", err)
		}
	})
}

func TestMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1.0.0", "1"},
		{"2.3.4", "2"},
		{"10.0.1", "10"},
		{"0.1.0", "0"},
		{"3", "3"},
	}
	for _, tt := range tests {
		got := majorVersion(tt.input)
		if got != tt.want {
			t.Errorf("majorVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToolError(t *testing.T) {
	e := &ToolError{Tool: "roady_init", Message: "bad"}
	if !strings.Contains(e.Error(), "roady_init") {
		t.Fatalf("error should contain tool name: %s", e.Error())
	}
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func TestIntegrationInitGetSpec(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root := findRepoRoot(t)
	tempDir := t.TempDir()

	binPath := filepath.Join(tempDir, "roady")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/roady")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build roady: %v\n%s", err, out)
	}

	// Use environment variables for AI config so project can be initialized
	cmd := fmt.Sprintf("cd '%s' && ROADY_AI_PROVIDER=mock ROADY_AI_MODEL=test '%s' mcp --transport stdio", tempDir, binPath)
	transport, err := client.NewUnsafeStdioTransport("bash", "-lc", cmd)
	if err != nil {
		t.Fatalf("stdio transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	c := NewClient(transport, WithTimeout(60*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	info, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if !info.Capabilities.Tools {
		t.Fatalf("expected tools capability")
	}

	msg, err := c.Init(ctx, "test-sdk")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(msg, "test-sdk") {
		t.Fatalf("unexpected init result: %s", msg)
	}

	spec, err := c.GetSpec(ctx)
	if err != nil {
		t.Fatalf("get spec: %v", err)
	}
	if spec.Title != "test-sdk" {
		t.Fatalf("unexpected spec title: %s", spec.Title)
	}

	// Verify spec is persisted
	spec2, err := c.GetSpec(ctx)
	if err != nil {
		t.Fatalf("get spec again: %v", err)
	}
	if spec2.Title != "test-sdk" {
		t.Fatalf("spec not persisted: %s", spec2.Title)
	}

	// GetSchema should return valid schema info
	schema, err := c.GetSchema(ctx)
	if err != nil {
		t.Fatalf("get schema: %v", err)
	}
	if schema.SchemaVersion == "" {
		t.Fatal("expected non-empty schema version")
	}
	if schema.ServerVersion == "" {
		t.Fatal("expected non-empty server version")
	}
	if schema.Changelog == "" {
		t.Fatal("expected non-empty changelog URL")
	}

	// Compatible should pass for current SDK
	if err := c.Compatible(ctx); err != nil {
		t.Fatalf("compatible: %v", err)
	}
}

// The SDK against roady's own HTTP server, as a real caller runs it.
//
// roady's HTTP transport is mcp-go's stateless Streamable HTTP (MCP
// 2026-07-28), which retires the initialize handshake: a client discovers the
// server and every request carries Mcp-Method and the protocol version. The
// SDK used to call Initialize, the legacy handshake, and nothing exercised it
// over HTTP — the stdio test above cannot see a transport-level refusal.
func TestIntegrationConnectOverHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	root := findRepoRoot(t)
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "roady")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/roady")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build roady: %v\n%s", err, out)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	srv := exec.Command(binPath, "mcp", "--transport", "http", "--addr", addr)
	srv.Dir = tempDir
	srv.Env = append(os.Environ(), "ROADY_AI_PROVIDER=mock", "ROADY_AI_MODEL=test")
	if err := srv.Start(); err != nil {
		t.Fatalf("start roady: %v", err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _ = srv.Wait() })

	deadline := time.Now().Add(15 * time.Second)
	for {
		conn, derr := net.Dial("tcp", addr)
		if derr == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("roady HTTP server did not come up")
		}
		time.Sleep(50 * time.Millisecond)
	}

	transport, err := client.NewHTTPTransport("http://" + addr)
	if err != nil {
		t.Fatalf("http transport: %v", err)
	}
	c := NewClient(transport, WithTimeout(30*time.Second))
	defer func() { _ = c.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("connect over HTTP: %v", err)
	}
	if !info.Capabilities.Tools {
		t.Fatalf("expected tools capability")
	}
	msg, err := c.Init(ctx, "test-sdk-http")
	if err != nil {
		t.Fatalf("init over HTTP: %v", err)
	}
	if !strings.Contains(msg, "test-sdk-http") {
		t.Fatalf("unexpected init result: %s", msg)
	}
}
