package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Planned work committed without a marker: suggest names its task, link
// records it, and suggest stops listing it.
func TestUntaggedCommitLinkThroughTheCLI(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	t.Setenv("ROADY_USER", "tester")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	if _, err := runRoady(t, "", "init", "links"); err != nil {
		t.Fatal(err)
	}
	doc := `features:
  - id: http
    title: Transport
    requirements:
      - id: streamable-http
        title: Streamable HTTP transport
`
	if out, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}
	if err := os.WriteFile("server.go", []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "server.go")
	git("commit", "-q", "-m", "feat(transport): streamable http server")

	out, err := runRoady(t, "", "git", "suggest")
	if err != nil || !strings.Contains(out, "→ task-streamable-http") || !strings.Contains(out, "roady git link") {
		t.Fatalf("suggest: %v\n%s", err, out)
	}
	out, err = runRoady(t, "", "git", "link", "HEAD", "task-streamable-http")
	if err != nil || !strings.Contains(out, "to task-streamable-http") {
		t.Fatalf("link: %v\n%s", err, out)
	}
	out, _ = runRoady(t, "", "git", "suggest")
	if !strings.Contains(out, "claimed by a task") {
		t.Errorf("suggest after link:\n%s", out)
	}
	out, _ = runRoady(t, "", "task", "history", "task-streamable-http")
	if !strings.Contains(out, "linked commit ") {
		t.Errorf("history does not show the linked commit:\n%s", out)
	}
}
