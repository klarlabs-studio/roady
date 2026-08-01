package drift

import (
	"fmt"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// RepoActivity summarises what the repository has done since the plan was
// last updated. It is supplied by an adapter because the domain does not
// know about git.
type RepoActivity struct {
	// CommitsSincePlan counts commits touching anything outside .roady/
	// since the plan's UpdatedAt.
	CommitsSincePlan int
	// LastCommitAt is when the repository last moved at all.
	LastCommitAt time.Time
	// Unavailable is set when activity could not be determined — no git, a
	// shallow clone, a detached history. Callers must report nothing rather
	// than guess.
	Unavailable bool
}

// Staleness thresholds. A plan is called stale when the repository has moved
// substantially without it, judged on volume rather than age alone: a plan
// nobody needed to change because nobody was working is not stale, and a
// young plan that sixty commits have already outrun is.
const (
	staleCommitThreshold    = 10
	staleHighCommits        = 50
	staleCriticalCommits    = 200
	staleQuietRepoThreshold = 1
)

// DetectStalenessDrift reports a plan the repository has left behind.
//
// This closes a blind spot the other detectors share: they all compare
// Roady's own artifacts against each other, so a plan nobody edits stays
// internally consistent forever while the code moves on. Roady's own
// repository demonstrated it — 55 commits and seven releases past a plan
// marked 113/113 done, and drift detection reported a healthy project.
//
// Judging by commits rather than by file timestamps matters: a fresh clone
// rewrites mtimes, so timestamps would report every checkout as drift.
func (d *DriftDetector) DetectStalenessDrift(plan *planning.Plan, activity RepoActivity, now time.Time) []Issue {
	if plan == nil || len(plan.Tasks) == 0 {
		return nil
	}
	// Without a reliable signal, say nothing. A false accusation of drift
	// is worse than silence, because it trains people to ignore the report.
	if activity.Unavailable {
		return nil
	}
	if activity.CommitsSincePlan < staleCommitThreshold {
		return nil
	}
	// A repository nobody has touched is not outrunning its plan.
	if activity.CommitsSincePlan < staleQuietRepoThreshold {
		return nil
	}

	days := int(now.Sub(plan.UpdatedAt).Hours() / 24)
	if days < 0 {
		days = 0
	}

	severity := SeverityMedium
	switch {
	case activity.CommitsSincePlan >= staleCriticalCommits:
		severity = SeverityCritical
	case activity.CommitsSincePlan >= staleHighCommits:
		severity = SeverityHigh
	}

	return []Issue{{
		ID:          "drift-plan-stale",
		Type:        DriftTypePlan,
		Category:    CategoryStale,
		Severity:    severity,
		ComponentID: plan.ID,
		Message: fmt.Sprintf(
			"The plan has not changed in %d days while %d commits landed. It is unlikely to still describe the work being done.",
			days, activity.CommitsSincePlan),
		Hint: "Re-run 'roady spec analyze' and 'roady plan generate' to bring the plan back to what is actually being built, or archive it if the work is finished.",
	}}
}
