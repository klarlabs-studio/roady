package cli

import (
	"bytes"
	"os"
	"testing"
)

func TestCmdFailures_Internal(t *testing.T) {
	tempEmpty, _ := os.MkdirTemp("", "roady-cli-fail-*")
	defer func() { _ = os.RemoveAll(tempEmpty) }()
	old, _ := os.Getwd()
	defer func() { _ = os.Chdir(old) }()
	_ = os.Chdir(tempEmpty)

	buf := new(bytes.Buffer)
	RootCmd.SetOut(buf)
	RootCmd.SetErr(buf)
	RootCmd.SilenceUsage = true
	RootCmd.SilenceErrors = true

	// 1. Uninitialized
	cmds := [][]string{
		{"plan", "generate"},
		{"status"},
		{"drift", "detect"},
		{"policy", "check"},
		{"spec", "validate"},
		{"task", "start", "any"},
	}
	for _, args := range cmds {
		RootCmd.SetArgs(args)
		_ = RootCmd.Execute()
	}

	// 2. Doctor failure
	RootCmd.SetArgs([]string{"doctor"})
	_ = RootCmd.Execute()

	// 3. Init failure
	_ = os.Mkdir(".roady", 0700)
	RootCmd.SetArgs([]string{"init", "fail"})
	_ = RootCmd.Execute()

	// 4. Task transition failure
	RootCmd.SetArgs([]string{"task", "complete", "missing"})
	_ = RootCmd.Execute()

	// 4.1 Task missing arg
	RootCmd.SetArgs([]string{"task", "start"})
	_ = RootCmd.Execute()

	// 5. Spec Import fail
	RootCmd.SetArgs([]string{"spec", "import", "/nonexistent"})
	_ = RootCmd.Execute()

	// 5.1 Spec Validate fail
	_ = os.WriteFile(".roady/spec.yaml", []byte("invalid"), 0600)
	RootCmd.SetArgs([]string{"spec", "validate"})
	_ = RootCmd.Execute()

	// 6. Policy load failure
	_ = os.WriteFile(".roady/policy.yaml", []byte("max_wip: [invalid]"), 0600)
	RootCmd.SetArgs([]string{"policy", "check"})
	_ = RootCmd.Execute()

	// 7. Sync failure (invalid plugin)
	RootCmd.SetArgs([]string{"sync", "/dev/null"})
	_ = RootCmd.Execute()

	// 8. MCP skip
	_ = os.Setenv("ROADY_SKIP_MCP_START", "true")
	defer func() { _ = os.Unsetenv("ROADY_SKIP_MCP_START") }()
	RootCmd.SetArgs([]string{"mcp"})
	_ = RootCmd.Execute()
}
