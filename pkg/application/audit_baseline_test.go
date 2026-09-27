package application

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/storage"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// Removing the newest committed events leaves the chain internally consistent,
// so only a comparison against the committed log can report it.
func TestVerifyAgainstBaselineCatchesTruncation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")

	repo := storage.NewFilesystemRepository(dir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	audit := NewAuditService(repo)
	for _, action := range []string{"one", "two", "three"} {
		if err := audit.Log(action, "tester", nil); err != nil {
			t.Fatalf("log: %v", err)
		}
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "log")

	eventsPath, err := repo.ResolvePath(storage.EventsFile)
	if err != nil {
		t.Fatal(err)
	}
	baseline, ok, reason := CommittedFile(dir, eventsPath, DefaultAuditBaseline)
	if !ok {
		t.Fatalf("expected a committed baseline: %s", reason)
	}

	// Untouched: nothing missing. Appending more is growth, not a finding.
	if err := audit.Log("four", "tester", nil); err != nil {
		t.Fatal(err)
	}
	if v, err := audit.VerifyAgainstBaseline(baseline, "HEAD"); err != nil || len(v) != 0 {
		t.Fatalf("growth reported: %v %v", v, err)
	}

	// Truncate to the first line: the chain alone still verifies.
	data, _ := os.ReadFile(eventsPath)
	first := data[:indexOf(data, '\n')+1]
	if err := os.WriteFile(eventsPath, first, 0o600); err != nil {
		t.Fatal(err)
	}
	if v, _ := audit.VerifyIntegrityDetailed(); len(v) != 0 {
		t.Fatalf("precondition: truncated chain should verify on its own, got %v", v)
	}
	v, err := audit.VerifyAgainstBaseline(baseline, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatalf("expected the two committed entries removed by truncation, got %d: %v", len(v), v)
	}
}

func TestCommittedFileWithoutGitIsNoBaseline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".roady", "events.jsonl")
	if _, ok, reason := CommittedFile(dir, path, "HEAD"); ok || reason == "" {
		t.Fatalf("expected no baseline with a reason, got ok=%v reason=%q", ok, reason)
	}
	if _, ok, _ := CommittedFile(dir, path, ""); ok {
		t.Fatal("an empty ref must mean no baseline")
	}
}

func indexOf(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return len(b) - 1
}
