package storage

import (
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/drift"
)

// GitActivityInspector reports repository movement from git history.
type GitActivityInspector struct {
	root string
}

func NewGitActivityInspector(root string) *GitActivityInspector {
	return &GitActivityInspector{root: root}
}

// ActivitySince counts commits that touched anything outside .roady/ since
// the given time.
//
// Excluding .roady/ is the point: updating the plan is itself a commit, so
// counting it would let a project stay "fresh" by only ever editing its own
// bookkeeping. Commits are used rather than file timestamps because a fresh
// clone rewrites mtimes, which would report every checkout as drift.
//
// Any failure — no git, a shallow clone, an empty history — is reported as
// Unavailable so the detector stays silent rather than guessing.
func (g *GitActivityInspector) ActivitySince(since time.Time) drift.RepoActivity {
	if since.IsZero() {
		return drift.RepoActivity{Unavailable: true}
	}

	count, err := g.run("rev-list", "--count",
		"--since="+since.Format(time.RFC3339), "HEAD", "--", ".", ":(exclude).roady")
	if err != nil {
		return drift.RepoActivity{Unavailable: true}
	}

	n, convErr := strconv.Atoi(strings.TrimSpace(count))
	if convErr != nil {
		return drift.RepoActivity{Unavailable: true}
	}

	activity := drift.RepoActivity{CommitsSincePlan: n}

	if out, lastErr := g.run("log", "-1", "--format=%cI"); lastErr == nil {
		if ts, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(out)); parseErr == nil {
			activity.LastCommitAt = ts
		}
	}

	return activity
}

func (g *GitActivityInspector) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...) // #nosec G204 -- fixed subcommands, no user input
	cmd.Dir = g.root
	out, err := cmd.Output()
	return string(out), err
}
