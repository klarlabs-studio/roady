package wiring

import (
	"testing"
)

func TestNewWorkspaceProvidesRepoAndAudit(t *testing.T) {
	tempDir := t.TempDir()
	ws := NewWorkspace(tempDir)
	if ws.Repo == nil {
		t.Fatal("expected repository instance")
	}
	if ws.Audit == nil {
		t.Fatal("expected audit service instance")
	}
	if err := ws.Repo.Initialize(); err != nil {
		t.Fatalf("failed to initialize repo: %v", err)
	}
	if !ws.Repo.IsInitialized() {
		t.Fatal("expected repository to be initialized")
	}
	if err := ws.Audit.Log("test.workspace", "tester", nil); err != nil {
		t.Fatalf("audit log failed: %v", err)
	}
}
