package mcp

import (
	"strings"
	"testing"

	mcpserver "go.klarlabs.de/mcp/server"
)

// assertToolError checks that a handler reported a failure the MCP way: a
// normal result carrying isError, with a readable message. Handlers used to
// return a Go error here, which the library masked as "internal error" before
// it reached the agent.
func assertToolError(t *testing.T, result any, err error, wantSubstring string) {
	t.Helper()

	if err != nil {
		t.Fatalf("a tool failure must be a result, not a protocol error: %v", err)
	}

	res, ok := result.(*mcpserver.StructuredResult)
	if !ok {
		t.Fatalf("expected a tool-error result, got %T", result)
	}
	if !res.IsError {
		t.Fatal("expected isError to be set")
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Fatal("a tool error must carry a message the model can read")
	}
	if wantSubstring != "" && !strings.Contains(res.Content[0].Text, wantSubstring) {
		t.Errorf("message %q should mention %q", res.Content[0].Text, wantSubstring)
	}
}

// TestToolErrorsAreResultsNotProtocolErrors pins the contract for the whole
// surface: a task that legitimately could not be done is reported to the
// agent, not swallowed by the transport.
func TestToolErrorsAreResultsNotProtocolErrors(t *testing.T) {
	res := mcpErr("Ensure the project is initialized.")

	if !res.IsError {
		t.Error("mcpErr must set isError")
	}
	if len(res.Content) != 1 || res.Content[0].Type != "text" {
		t.Fatalf("expected one text content block, got %+v", res.Content)
	}
	if res.Content[0].Text != "Ensure the project is initialized." {
		t.Errorf("message not carried through: %q", res.Content[0].Text)
	}
}
