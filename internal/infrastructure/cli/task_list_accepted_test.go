package cli

import (
	"strings"
	"testing"
)

// After roady task accept, task list said [done] for accepted tasks and could
// not filter them: the command agents try first could not tell accepted work
// from work awaiting verification.
func TestTaskListShowsAccepted(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	if _, err := runRoady(t, "", "init", "list"); err != nil {
		t.Fatal(err)
	}
	doc := `features:
  - id: old
    title: Old
    requirements:
      - id: a
        title: A
      - id: b
        title: B
`
	if out, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}
	if _, err := runRoady(t, "", "plan", "approve"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"task-a", "task-b"} {
		for _, verb := range []string{"start", "complete"} {
			if out, err := runRoady(t, "", "task", verb, id); err != nil {
				t.Fatalf("%s %s: %v\n%s", verb, id, err, out)
			}
		}
	}
	if out, err := runRoady(t, "", "task", "accept", "task-a", "--reason", "before roady"); err != nil {
		t.Fatalf("accept: %v\n%s", err, out)
	}

	out, _ := runRoady(t, "", "task", "list")
	if !strings.Contains(out, "[accepted]   task-a") || !strings.Contains(out, "[done]       task-b") {
		t.Errorf("task list does not tell accepted from done:\n%s", out)
	}
	out, _ = runRoady(t, "", "task", "list", "--status", "accepted")
	if !strings.Contains(out, "Tasks (accepted): 1") || !strings.Contains(out, "task-a") {
		t.Errorf("--status accepted:\n%s", out)
	}
	out, _ = runRoady(t, "", "task", "list", "--status", "done")
	if !strings.Contains(out, "Tasks (done): 1") || strings.Contains(out, "task-a") {
		t.Errorf("--status done lists accepted work as awaiting verification:\n%s", out)
	}
}
