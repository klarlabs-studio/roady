package mcp

import "testing"

// TestEveryRegisteredAppExists guards the gap between appEntries and what
// app/build.sh actually produced. A registered URI whose file is missing
// compiles and starts fine, then fails only when a client requests it — so
// nothing catches it before a user does.
func TestEveryRegisteredAppExists(t *testing.T) {
	if len(appEntries) == 0 {
		t.Fatal("no MCP apps registered")
	}

	for _, entry := range appEntries {
		if _, err := distFS.ReadFile(entry.filePath); err != nil {
			t.Errorf("%s is registered but its file is unreadable: %v", entry.uri, err)
		}
	}
}
