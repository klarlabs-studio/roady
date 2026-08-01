package mcp

import (
	"strings"
	"testing"

	"go.klarlabs.de/mcp/testutil"
)

func TestServer_ReadSchemaResource(t *testing.T) {
	root := t.TempDir()
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}
	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	client := testutil.NewTestClient(t, server.mcpServer)
	defer client.Close()

	content, err := client.ReadResource("roady://schema")
	if err != nil {
		t.Fatalf("read schema resource: %v", err)
	}
	if !strings.Contains(content, "schema_version") {
		t.Errorf("expected schema content to include schema_version, got: %s", content)
	}
}

func TestServer_ReadAppResource(t *testing.T) {
	root := t.TempDir()
	if err := initProjectDir(root); err != nil {
		t.Fatalf("init mock AI config: %v", err)
	}
	server, err := NewServer(root)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}

	client := testutil.NewTestClient(t, server.mcpServer)
	defer client.Close()

	content, err := client.ReadResource("ui://roady/status")
	if err != nil {
		t.Fatalf("read app resource: %v", err)
	}
	if content == "" {
		t.Error("expected non-empty app resource")
	}
}
