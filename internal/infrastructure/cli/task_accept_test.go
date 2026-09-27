package cli

import (
	"strings"
	"testing"
)

// Finished work nobody can verify any more: accepted, it leaves "awaiting
// verification" for a line of its own, and is still not counted as verified.
func TestTaskAcceptThroughTheCLI(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")

	if _, err := runRoady(t, "", "init", "accept"); err != nil {
		t.Fatalf("init: %v", err)
	}
	doc := `features:
  - id: old
    title: Shipped before roady
    requirements:
      - id: a
        title: A
      - id: b
        title: B
`
	if out, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}
	if out, err := runRoady(t, "", "plan", "approve"); err != nil {
		t.Fatalf("approve: %v\n%s", err, out)
	}
	for _, id := range []string{"task-a", "task-b"} {
		for _, verb := range []string{"start", "complete"} {
			if out, err := runRoady(t, "", "task", verb, id); err != nil {
				t.Fatalf("%s %s: %v\n%s", verb, id, err, out)
			}
		}
	}

	if _, err := runRoady(t, "", "task", "accept", "--all-done"); err == nil || !strings.Contains(err.Error(), "needs a reason") {
		t.Errorf("accept without a reason: %v", err)
	}
	out, err := runRoady(t, "", "task", "accept", "--all-done", "--reason", "shipped before roady")
	if err != nil || !strings.Contains(out, "Accepted 2 tasks as done without verification") {
		t.Fatalf("accept: %v\n%s", err, out)
	}

	out, err = runRoady(t, "", "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{"- Done:        0 (awaiting verification)", "- Accepted:    2 (done, accepted without verification)", "- Verified:    0"} {
		if !strings.Contains(out, want) {
			t.Errorf("status lacks %q:\n%s", want, out)
		}
	}
	out, _ = runRoady(t, "", "status", "--json")
	if !strings.Contains(out, `"accepted": 2`) || !strings.Contains(out, `"awaiting_verification": 0`) {
		t.Errorf("status --json:\n%s", out)
	}
}
