package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcplib "go.klarlabs.de/mcp"
)

func init() {
	// Pre-initialize test with mock AI config
}

func TestGenerateOpenAPI(t *testing.T) {
	srv := mcplib.NewServer(mcplib.ServerInfo{Name: "test", Version: "0.1.0"})
	srv.Tool("test_tool").
		Description("A test tool").
		Handler(func(ctx context.Context, args struct {
			Name string `json:"name" jsonschema:"description=The name"`
		}) (string, error) {
			return "ok", nil
		})

	tools := srv.Tools()
	if len(tools) == 0 {
		t.Fatal("no tools registered on server")
	}

	data, err := GenerateOpenAPI(srv)
	if err != nil {
		t.Fatalf("GenerateOpenAPI failed: %v", err)
	}

	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %s", spec.OpenAPI)
	}
	if spec.Info.Title != "Roady MCP API" {
		t.Errorf("unexpected title: %s", spec.Info.Title)
	}

	path, ok := spec.Paths["/tools/test_tool"]
	if !ok {
		t.Fatalf("expected /tools/test_tool path, got paths: %v", spec.Paths)
	}
	if path.Post == nil {
		t.Fatal("expected POST operation")
	}
	if path.Post.OperationID != "test_tool" {
		t.Errorf("unexpected operationId: %s", path.Post.OperationID)
	}
	if path.Post.Summary != "A test tool" {
		t.Errorf("unexpected summary: %s", path.Post.Summary)
	}
}

func TestGenerateOpenAPI_NoArgs(t *testing.T) {
	srv := mcplib.NewServer(mcplib.ServerInfo{Name: "test", Version: "0.1.0"})
	srv.Tool("no_args_tool").
		Description("No args").
		Handler(func(ctx context.Context, args struct{}) (string, error) {
			return "ok", nil
		})

	data, err := GenerateOpenAPI(srv)
	if err != nil {
		t.Fatalf("GenerateOpenAPI failed: %v", err)
	}

	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	path, ok := spec.Paths["/tools/no_args_tool"]
	if !ok {
		t.Fatal("expected /tools/no_args_tool path")
	}
	if path.Post == nil {
		t.Fatal("expected POST operation")
	}
	if path.Post.RequestBody != nil {
		t.Error("expected no request body for empty args tool")
	}
}

func TestHasProperties(t *testing.T) {
	tests := []struct {
		name     string
		schema   any
		expected bool
	}{
		{
			name:     "has properties",
			schema:   map[string]any{"properties": map[string]any{"name": "value"}},
			expected: true,
		},
		{
			name:     "empty properties",
			schema:   map[string]any{"properties": map[string]any{}},
			expected: false,
		},
		{
			name:     "no properties",
			schema:   map[string]any{"type": "object"},
			expected: false,
		},
		{
			name:     "nil schema",
			schema:   nil,
			expected: false,
		},
		{
			name:     "string schema",
			schema:   "not a map",
			expected: false,
		},
		{
			name:     "non-map properties",
			schema:   map[string]any{"properties": "not a map"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasProperties(tt.schema)
			if got != tt.expected {
				t.Errorf("hasProperties() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestServerOpenAPI(t *testing.T) {
	tempDir := t.TempDir()
	if err := initProjectDir(tempDir); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}
	server, err := NewServer(tempDir)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	data, err := server.OpenAPI()
	if err != nil {
		t.Fatalf("OpenAPI failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty OpenAPI document")
	}

	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %s", spec.OpenAPI)
	}
}
