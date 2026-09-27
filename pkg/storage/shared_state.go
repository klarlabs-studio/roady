package storage

import (
	"os"
	"path/filepath"
	"strings"
)

// Execution state is shared by every worktree of a git repository: it lives
// in the common git directory, so a claim or a completion in one worktree is
// seen by the others at once (docs/rfcs/0002-shared-execution-state.md). The
// checkout's .roady/state.json is kept as a mirror for commits and review.

// SharedStateDir is the directory under the common git dir that holds it.
const SharedStateDir = "roady"

// gitCommonDir finds the common git directory for dir without running git:
// the nearest .git above it is either that directory, or a file naming the
// worktree's git dir, whose commondir file points at the common one. It also
// returns the worktree's top-level directory.
func gitCommonDir(dir string) (common, top string, ok bool) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", false
	}
	for d := abs; ; d = filepath.Dir(d) {
		dotgit := filepath.Join(d, ".git")
		info, err := os.Stat(dotgit)
		if err == nil {
			if info.IsDir() {
				return dotgit, d, true
			}
			gitdir, ok := readGitdirFile(dotgit)
			if !ok {
				return "", "", false
			}
			raw, err := os.ReadFile(filepath.Join(gitdir, "commondir")) // #nosec G304 -- inside the repository's git dir
			if err != nil {
				return gitdir, d, true // a plain linked git dir without commondir is its own common dir
			}
			c := strings.TrimSpace(string(raw))
			if !filepath.IsAbs(c) {
				c = filepath.Join(gitdir, c)
			}
			return filepath.Clean(c), d, true
		}
		if parent := filepath.Dir(d); parent == d {
			return "", "", false
		}
	}
}

// readGitdirFile reads a ".git" file ("gitdir: <path>").
func readGitdirFile(path string) (string, bool) {
	raw, err := os.ReadFile(path) // #nosec G304 -- the repository's own .git file
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(string(raw))
	gitdir, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return "", false
	}
	gitdir = strings.TrimSpace(gitdir)
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(path), gitdir)
	}
	return filepath.Clean(gitdir), true
}

// sharedStatePath returns where this project's shared state lives, or ""
// when state stays in the checkout: outside git, or turned off by policy.
func (r *FilesystemRepository) sharedStatePath() string {
	if cfg, err := r.LoadPolicy(); err == nil && cfg != nil && cfg.SharedState != nil && !*cfg.SharedState {
		return ""
	}
	common, top, ok := gitCommonDir(r.root)
	if !ok {
		return ""
	}
	base, err := filepath.Abs(r.ProjectBase())
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(top, base)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return filepath.Join(common, SharedStateDir, rel, StateFile)
}

// StateLocation reports where execution state is read from and written to:
// the shared file in the common git dir, or the checkout's state.json.
func (r *FilesystemRepository) StateLocation() (path string, shared bool) {
	if p := r.sharedStatePath(); p != "" {
		return p, true
	}
	p, _ := r.ResolvePath(StateFile)
	return p, false
}
