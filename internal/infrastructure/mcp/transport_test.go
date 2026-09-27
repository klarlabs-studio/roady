package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.klarlabs.de/mcp/client"
)

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      int              `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func TestMCPHTTPTransport(t *testing.T) {
	tempDir := t.TempDir()

	if err := initProjectDir(tempDir); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	prevVersion, prevCommit, prevDate := Version, BuildCommit, BuildDate
	Version, BuildCommit, BuildDate = "test", "commit123", "2026-01-01"
	t.Cleanup(func() {
		Version, BuildCommit, BuildDate = prevVersion, prevCommit, prevDate
	})

	addr := pickFreeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ServeHTTP(ctx, addr)
	}()

	waitForHTTP(t, addr, 5*time.Second)

	// roady serves stateless Streamable HTTP (MCP 2026-07-28), which retires
	// the initialize handshake: a conforming client discovers the server and
	// sends Mcp-Method and the protocol version on every request. The mcp-go
	// client's Connect is that client. A hand-written 2024-11-05 initialize is
	// refused, correctly, and is what this test used to send.
	tr, err := client.NewHTTPTransport("http://" + addr)
	if err != nil {
		t.Fatalf("http transport: %v", err)
	}
	c := client.New(tr, client.WithTimeout(10*time.Second))
	defer func() { _ = c.Close() }()

	info, err := c.Connect(ctx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if info.Version != "test" {
		t.Fatalf("unexpected version: %v", info.Version)
	}
	if !info.Capabilities.Tools {
		t.Fatalf("expected tools capability")
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(tools) == 0 {
		t.Fatalf("tools/list returned no tools")
	}
}

func TestMCPStdioTransport(t *testing.T) {
	root := findRepoRoot(t)
	tempDir := t.TempDir()

	binPath := filepath.Join(tempDir, "roady")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/roady")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build roady: %v\n%s", err, out)
	}

	cmd := fmt.Sprintf("cd %s && ROADY_AI_PROVIDER=mock ROADY_AI_MODEL=test %s mcp --transport stdio", shellEscape(tempDir), shellEscape(binPath))
	transport, err := client.NewUnsafeStdioTransport("bash", "-lc", cmd)
	if err != nil {
		t.Fatalf("stdio transport: %v", err)
	}
	defer func() { _ = transport.Close() }()

	mcpClient := client.New(transport, client.WithTimeout(60*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	info, err := mcpClient.Initialize(ctx)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if !info.Capabilities.Tools {
		t.Fatalf("expected tools capability")
	}

	tools, err := mcpClient.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	found := false
	for _, tool := range tools {
		if tool.Name == "roady_next" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected roady_next in the tool list")
	}

	// A project that was never initialised gets an actionable answer, not a
	// crash: roady_next reports there is nothing to work on.
	result, err := mcpClient.CallTool(ctx, "roady_next", map[string]any{})
	if err != nil {
		t.Fatalf("call roady_next: %v", err)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "Roady") {
		t.Fatalf("unexpected roady_next response: %+v", result.Content)
	}
}

func TestMCPWebSocketTransport(t *testing.T) {
	tempDir := t.TempDir()

	if err := initProjectDir(tempDir); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}

	prevVersion, prevCommit, prevDate := Version, BuildCommit, BuildDate
	Version, BuildCommit, BuildDate = "test", "commit123", "2026-01-01"
	t.Cleanup(func() {
		Version, BuildCommit, BuildDate = prevVersion, prevCommit, prevDate
	})

	addr := pickFreeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ServeWebSocket(ctx, addr)
	}()

	// Wait for WebSocket server to start
	time.Sleep(100 * time.Millisecond)

	// Connect using raw WebSocket (mcp-go client doesn't have WebSocket transport)
	wsURL := fmt.Sprintf("ws://%s/mcp", addr)
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer func() { _ = ws.Close() }()

	// Send initialize request
	initReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]any{
				"name":    "roady-ws-test",
				"version": "0.0.0",
			},
			"capabilities": map[string]any{},
		},
	}
	if err := ws.WriteJSON(initReq); err != nil {
		t.Fatalf("write initialize: %v", err)
	}

	// Read initialize response
	var initResp jsonRPCResponse
	if err := ws.ReadJSON(&initResp); err != nil {
		t.Fatalf("read initialize: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("initialize error: %v", initResp.Error.Message)
	}
	if initResp.Result == nil {
		t.Fatalf("initialize missing result")
	}

	var result map[string]any
	if err := json.Unmarshal(*initResp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	capabilities := result["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"]; !ok {
		t.Fatalf("expected tools capability")
	}

	// Send tools/list request
	toolsReq := jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	}
	if err := ws.WriteJSON(toolsReq); err != nil {
		t.Fatalf("write tools/list: %v", err)
	}

	// Read tools/list response
	var toolsResp jsonRPCResponse
	if err := ws.ReadJSON(&toolsResp); err != nil {
		t.Fatalf("read tools/list: %v", err)
	}
	if toolsResp.Error != nil {
		t.Fatalf("tools/list error: %v", toolsResp.Error.Message)
	}

	var toolsResult map[string]any
	if err := json.Unmarshal(*toolsResp.Result, &toolsResult); err != nil {
		t.Fatalf("unmarshal tools: %v", err)
	}

	tools, ok := toolsResult["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools array")
	}

	found := false
	for _, tool := range tools {
		toolMap := tool.(map[string]any)
		if toolMap["name"] == "roady_status" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected roady_status tool")
	}
}

func pickFreeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	return addr
}

func waitForHTTP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("http://%s/health", addr)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not become healthy at %s", url)
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

func shellEscape(value string) string {
	escaped := strings.ReplaceAll(value, "'", "'\"'\"")
	return "'" + escaped + "'"
}
