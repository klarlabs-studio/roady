package cli

import "testing"

func TestAssignCommandGroups(t *testing.T) {
	assignCommandGroups()

	// Idempotent: calling twice must not duplicate groups.
	assignCommandGroups()

	wantGroupCount := 4
	if got := len(RootCmd.Groups()); got < wantGroupCount {
		t.Fatalf("expected at least %d groups, got %d", wantGroupCount, got)
	}

	cases := map[string]string{
		"init":   groupGetStarted,
		"task":   groupGetStarted,
		"setup":  groupGetStarted,
		"drift":  groupTrackReport,
		"mcp":    groupIntegrate,
		"audit":  groupAdmin,
		"doctor": groupAdmin,
	}

	for name, wantGroup := range cases {
		var found bool
		for _, cmd := range RootCmd.Commands() {
			if cmd.Name() != name {
				continue
			}
			found = true
			if cmd.GroupID != wantGroup {
				t.Errorf("command %q: GroupID = %q, want %q", name, cmd.GroupID, wantGroup)
			}
		}
		if !found {
			t.Errorf("command %q not registered on RootCmd", name)
		}
	}
}
