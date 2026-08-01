package mcp

import (
	"bytes"
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

	resp := sendJSONRPC(t, addr, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]any{
				"name":    "roady-test",
				"version": "0.0.0",
			},
			"capabilities": map[string]any{},
		},
	})

	if resp.Error != nil {
		t.Fatalf("initialize error: %v", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatalf("initialize missing result")
	}
	var result map[string]any
	if err := json.Unmarshal(*resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	serverInfo := result["serverInfo"].(map[string]any)
	if serverInfo["version"] != "test" {
		t.Fatalf("unexpected version: %v", serverInfo["version"])
	}
	capabilities := result["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"]; !ok {
		t.Fatalf("expected tools capability")
	}

	resp = sendJSONRPC(t, addr, jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %v", resp.Error.Message)
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
		if tool.Name == "roady_init" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected roady_init tool")
	}

	result, err := mcpClient.CallTool(ctx, "roady_init", map[string]any{"name": "test"})
	if err != nil {
		t.Fatalf("call roady_init: %v", err)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, "Project test initialized") {
		t.Fatalf("unexpected roady_init response: %+v", result.Content)
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

func sendJSONRPC(t *testing.T, addr string, req jsonRPCRequest) jsonRPCResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	url := fmt.Sprintf("http://%s/mcp", addr)
	httpResp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("http post: %v", err)
	}
	defer httpResp.Body.Close() //nolint:errcheck // best-effort close on read body

	var resp jsonRPCResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
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
