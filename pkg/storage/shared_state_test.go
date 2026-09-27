package storage

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestGitCommonDir(t *testing.T) {
	main := t.TempDir()
	_ = os.MkdirAll(filepath.Join(main, ".git", "worktrees", "wt"), 0o755)
	sub := filepath.Join(main, "a", "b")
	_ = os.MkdirAll(sub, 0o755)
	mainGit := resolved(t, filepath.Join(main, ".git"))
	if c, top, ok := gitCommonDir(sub); !ok || c != mainGit || top != main {
		t.Errorf("main checkout: %q %q %v", c, top, ok)
	}

	// A linked worktree: .git is a file naming its git dir, whose commondir
	// points back at the main .git.
	wt := t.TempDir()
	wtGit := filepath.Join(main, ".git", "worktrees", "wt")
	_ = os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o644)
	if c, top, ok := gitCommonDir(wt); !ok || c != mainGit || top != wt {
		t.Errorf("worktree: %q %q %v", c, top, ok)
	}

	// A relative gitdir, and one without commondir (its own common dir).
	rel := t.TempDir()
	_ = os.MkdirAll(filepath.Join(rel, "real"), 0o755)
	_ = os.WriteFile(filepath.Join(rel, ".git"), []byte("gitdir: real"), 0o644)
	if c, _, ok := gitCommonDir(rel); !ok || c != resolved(t, filepath.Join(rel, "real")) {
		t.Errorf("relative gitdir: %q %v", c, ok)
	}

	// Opened through a symlink, the checkout names the same common dir as the
	// worktree, whose .git file holds the real path.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(main, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if c, _, ok := gitCommonDir(link); !ok || c != mainGit {
		t.Errorf("through a symlink: %q %v, want %q", c, ok, mainGit)
	}

	bad := t.TempDir()
	_ = os.WriteFile(filepath.Join(bad, ".git"), []byte("not a gitdir"), 0o644)
	if _, _, ok := gitCommonDir(bad); ok {
		t.Error("a malformed .git file is not a repository")
	}
}

func TestStateOutsideGitStaysInTheCheckout(t *testing.T) {
	dir := t.TempDir()
	r := NewFilesystemRepository(dir)
	_ = r.Initialize()
	if _, shared := r.StateLocation(); shared {
		t.Skip("the temp dir is inside a git repository")
	}
	s := planning.NewExecutionState("p")
	if err := r.SaveState(s); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".roady", "state.json")); err != nil {
		t.Error("state.json not written in the checkout")
	}
}

// Two worktrees of one repository read and write one state; the checkout
// keeps a mirror; the first save seeds from the checkout; policy can opt out.
func TestStateIsSharedAcrossWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	main := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(main, "init", "-q", "-b", "main")
	a := NewFilesystemRepository(main)
	_ = a.Initialize()
	seed := planning.NewExecutionState("p")
	seed.SetTaskStatus("t1", planning.StatusDone)
	_ = os.WriteFile(filepath.Join(main, ".roady", "state.json"), mustJSON(t, seed), 0o600)
	git(main, "add", ".")
	git(main, "commit", "-q", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	git(main, "worktree", "add", "-q", wt, "-b", "other")
	b := NewFilesystemRepository(wt)

	pathA, sharedA := a.StateLocation()
	pathB, sharedB := b.StateLocation()
	if !sharedA || !sharedB || pathA != pathB {
		t.Fatalf("worktrees do not share state: %s (%v) vs %s (%v)", pathA, sharedA, pathB, sharedB)
	}

	// Seeded from the checkout before the first save.
	st, _ := b.LoadState()
	if st.GetTaskStatus("t1") != planning.StatusDone {
		t.Fatalf("not seeded from the checkout: %+v", st.TaskStates)
	}
	st.SetTaskStatus("t2", planning.StatusInProgress)
	if err := b.SaveState(st); err != nil {
		t.Fatal(err)
	}
	fromA, _ := a.LoadState()
	if fromA.GetTaskStatus("t2") != planning.StatusInProgress {
		t.Error("worktree A does not see B's change")
	}
	mirror, _ := os.ReadFile(filepath.Join(wt, ".roady", "state.json"))
	if !contains(string(mirror), `"t2"`) {
		t.Error("B's checkout mirror was not updated")
	}

	// A stale save from the other worktree conflicts.
	stale := *st
	stale.Version--
	if err := a.SaveState(&stale); err == nil {
		t.Error("a stale save across worktrees must conflict")
	}

	// Opting out keeps state per checkout.
	off := false
	_ = a.SavePolicy(&domain.PolicyConfig{SharedState: &off})
	if _, shared := a.StateLocation(); shared {
		t.Error("shared_state: false still shared")
	}
}

func TestInitializeDropsStaleSharedState(t *testing.T) {
	main := t.TempDir()
	_ = os.MkdirAll(filepath.Join(main, ".git"), 0o755)
	r := NewFilesystemRepository(main)
	shared := filepath.Join(main, ".git", SharedStateDir, ".roady", StateFile)
	_ = os.MkdirAll(filepath.Dir(shared), 0o755)
	_ = os.WriteFile(shared, []byte(`{"project_id":"old","version":9}`), 0o600)
	if err := r.Initialize(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shared); !os.IsNotExist(err) {
		t.Error("a fresh project inherited the old shared state")
	}
	if err := r.SaveState(planning.NewExecutionState("new")); err != nil {
		t.Fatalf("first save of a fresh project: %v", err)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }

// resolved is path with symlinks resolved, as gitCommonDir reports it.
func resolved(t *testing.T, path string) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
