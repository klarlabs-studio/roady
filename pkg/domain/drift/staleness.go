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
	// Commits are the commits CommitsSincePlan counted, newest first, when
	// the inspector lists them; nil when it only counts.
	Commits []Commit
	// Claimed counts commits since the plan that a task claims — by a
	// [roady:<task>] marker or as linked evidence — and so are not the plan
	// falling behind. CommitsSincePlan excludes them once they are counted.
	Claimed int
}

// Commit is a commit named by hash and subject.
type Commit struct {
	Hash    string `json:"hash"`
	Subject string `json:"subject"`
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
		// Say what was measured, not what was inferred from it. The field read
		// here is a timestamp, so "has not changed" was a claim the check could
		// not support — and one the operator could disprove by looking at the
		// file they had just edited.
		Message: staleMessage(days, activity),
		// The non-destructive routes come first. Regenerating is listed last
		// and with its cost stated: the reconciler keeps tasks it did not
		// propose, but a task whose id matches a spec requirement is replaced
		// wholesale, so hand-written titles, descriptions and estimates on
		// those are lost. A curated plan is exactly the case where that is the
		// wrong move, and it was previously the only advice offered.
		Hint: "If the commits are planned work committed without a [roady:<task>] marker, `roady git suggest` names the task each most likely served and `roady git link <commit> <task>` records it. " +
			"If the work is finished, verify the remaining tasks or archive the plan. " +
			"If work landed that no task covers, add it to the spec and reconcile — note that " +
			"'roady plan generate' keeps tasks it does not propose but overwrites any task whose id " +
			"matches a spec requirement, so curated titles and descriptions on those are lost.",
	}}
}

// staleMessage says what was counted: commits no task claims, and how many
// others were left out because one does.
func staleMessage(days int, a RepoActivity) string {
	if a.Claimed == 0 {
		return fmt.Sprintf("The plan was last updated %d days ago; %d commits have landed since. It may no longer describe the work being done.",
			days, a.CommitsSincePlan)
	}
	return fmt.Sprintf("The plan was last updated %d days ago; %d commits no task claims have landed since (%d more are linked to tasks). It may no longer describe the work being done.",
		days, a.CommitsSincePlan, a.Claimed)
}
