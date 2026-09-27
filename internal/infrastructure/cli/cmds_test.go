package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAllCmds_Internal(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-cli-all-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	_ = os.Chdir(tempDir)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SilenceUsage = true
	RootCmd.SilenceErrors = true

	// 1. Init
	RootCmd.SetArgs([]string{"init", "test"})
	_ = RootCmd.Execute()

	// 2. Plan
	RootCmd.SetArgs([]string{"plan", "generate"})
	_ = RootCmd.Execute()

	// 3. Status
	RootCmd.SetArgs([]string{"status"})
	_ = RootCmd.Execute()

	// Status empty
	_ = os.Remove(filepath.Join(tempDir, ".roady", "plan.json"))
	RootCmd.SetArgs([]string{"status"})
	_ = RootCmd.Execute()

	// 4. Drift
	RootCmd.SetArgs([]string{"drift", "detect"})
	_ = RootCmd.Execute()

	// Drift detected
	_ = os.WriteFile(filepath.Join(tempDir, ".roady", "spec.yaml"), []byte("id: test\ntitle: test\nfeatures: [{id: f1, title: F1}, {id: f2, title: F2}]"), 0600)
	_ = os.WriteFile(filepath.Join(tempDir, ".roady", "plan.json"), []byte("{\"id\":\"p1\",\"tasks\":[{\"id\":\"t1\",\"feature_id\":\"f1\"}]}"), 0600)
	RootCmd.SetArgs([]string{"drift", "detect"})
	_ = RootCmd.Execute()

	// Drift with JSON and issues
	_ = os.WriteFile(filepath.Join(tempDir, ".roady", "spec.yaml"), []byte("id: test\ntitle: test\nfeatures:\n  - id: f1\n    title: F1"), 0600)
	RootCmd.SetArgs([]string{"drift", "detect", "--output", "json"})
	_ = RootCmd.Execute()

	// 5. Policy
	RootCmd.SetArgs([]string{"policy", "check"})
	_ = RootCmd.Execute()

	// 6. Spec
	RootCmd.SetArgs([]string{"spec", "validate"})
	_ = RootCmd.Execute()

	// Spec Validate failure
	_ = os.WriteFile(filepath.Join(tempDir, ".roady", "spec.yaml"), []byte("id: test\ntitle: test\nfeatures: [{id: f1}, {id: f1}]"), 0600)
	RootCmd.SetArgs([]string{"spec", "validate"})
	_ = RootCmd.Execute()

	// 7. Task
	RootCmd.SetArgs([]string{"task", "start", "task-core-foundation"})
	_ = RootCmd.Execute()

	// Task block
	RootCmd.SetArgs([]string{"task", "block", "task-core-foundation"})
	_ = RootCmd.Execute()

	// Task unblock
	RootCmd.SetArgs([]string{"task", "unblock", "task-core-foundation"})
	_ = RootCmd.Execute()

	// Task stop
	RootCmd.SetArgs([]string{"task", "stop", "task-core-foundation"})
	_ = RootCmd.Execute()

	// Task complete
	RootCmd.SetArgs([]string{"task", "start", "task-core-foundation"})
	_ = RootCmd.Execute()
	RootCmd.SetArgs([]string{"task", "complete", "task-core-foundation"})
	_ = RootCmd.Execute()

	RootCmd.SetArgs([]string{"task", "block", "task-core-foundation"})
	_ = RootCmd.Execute()

	RootCmd.SetArgs([]string{"task", "unblock", "task-core-foundation"})
	_ = RootCmd.Execute()

	RootCmd.SetArgs([]string{"task", "stop", "task-core-foundation"})
	_ = RootCmd.Execute()

	RootCmd.SetArgs([]string{"task", "start", "task-core-foundation"})
	_ = RootCmd.Execute()

	RootCmd.SetArgs([]string{"task", "complete", "task-core-foundation"})
	_ = RootCmd.Execute()

	// 8. Doctor
	RootCmd.SetArgs([]string{"doctor"})
	_ = RootCmd.Execute()

	// 9. Spec Import
	md := filepath.Join(tempDir, "test.md")
	_ = os.WriteFile(md, []byte("# Test\n## F1"), 0600)
	RootCmd.SetArgs([]string{"spec", "import", md})
	_ = RootCmd.Execute()

	// Import non-markdown (still works because we just scan lines)
	txt := filepath.Join(tempDir, "test.txt")
	_ = os.WriteFile(txt, []byte("plain text"), 0600)
	RootCmd.SetArgs([]string{"spec", "import", txt})
	_ = RootCmd.Execute()

	// 11. MCP (Internal)
	_ = os.Setenv("ROADY_SKIP_MCP_START", "true")
	defer func() { _ = os.Unsetenv("ROADY_SKIP_MCP_START") }()
	RootCmd.SetArgs([]string{"mcp"})
	_ = RootCmd.Execute()
}
