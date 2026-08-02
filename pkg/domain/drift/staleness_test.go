package drift

import (
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func TestDetectStalenessDrift(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	daysAgo := func(d int) time.Time { return now.AddDate(0, 0, -d) }

	tests := []struct {
		name         string
		plan         *planning.Plan
		activity     RepoActivity
		wantIssues   int
		wantContains string
	}{
		{
			name:     "a plan tracking recent work is healthy",
			plan:     &planning.Plan{UpdatedAt: daysAgo(2), Tasks: []planning.Task{{ID: "t1"}}},
			activity: RepoActivity{CommitsSincePlan: 3, LastCommitAt: daysAgo(1)},
		},
		{
			name: "an abandoned plan is drift",
			// The case that motivated this: every task done, the plan
			// untouched for months, and the repository moved on without it.
			plan:         &planning.Plan{UpdatedAt: daysAgo(90), Tasks: []planning.Task{{ID: "t1"}}},
			activity:     RepoActivity{CommitsSincePlan: 55, LastCommitAt: daysAgo(1)},
			wantIssues:   1,
			wantContains: "55 commits",
		},
		{
			name:       "few commits is not yet drift, however old the plan",
			plan:       &planning.Plan{UpdatedAt: daysAgo(200), Tasks: []planning.Task{{ID: "t1"}}},
			activity:   RepoActivity{CommitsSincePlan: 2, LastCommitAt: daysAgo(1)},
			wantIssues: 0,
		},
		{
			name: "many commits is drift even on a young plan",
			// Volume matters independently of age: a plan a week old that
			// 60 commits have already outrun is not describing the work.
			plan:         &planning.Plan{UpdatedAt: daysAgo(7), Tasks: []planning.Task{{ID: "t1"}}},
			activity:     RepoActivity{CommitsSincePlan: 60, LastCommitAt: daysAgo(1)},
			wantIssues:   1,
			wantContains: "60 commits",
		},
		{
			name: "a dormant repository is not drift",
			// Nobody is working. The plan is not lagging reality.
			plan:       &planning.Plan{UpdatedAt: daysAgo(120), Tasks: []planning.Task{{ID: "t1"}}},
			activity:   RepoActivity{CommitsSincePlan: 0, LastCommitAt: daysAgo(120)},
			wantIssues: 0,
		},
		{
			name:       "no plan means nothing to be stale",
			plan:       nil,
			activity:   RepoActivity{CommitsSincePlan: 99},
			wantIssues: 0,
		},
		{
			name:       "an empty plan is not stale",
			plan:       &planning.Plan{UpdatedAt: daysAgo(90)},
			activity:   RepoActivity{CommitsSincePlan: 99},
			wantIssues: 0,
		},
		{
			name: "unknown activity is not reported as drift",
			// Git unavailable. Silence beats a false accusation.
			plan:       &planning.Plan{UpdatedAt: daysAgo(90), Tasks: []planning.Task{{ID: "t1"}}},
			activity:   RepoActivity{Unavailable: true},
			wantIssues: 0,
		},
	}

	detector := NewDriftDetector()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issues := detector.DetectStalenessDrift(tt.plan, tt.activity, now)

			if len(issues) != tt.wantIssues {
				t.Fatalf("expected %d issues, got %d: %+v", tt.wantIssues, len(issues), issues)
			}
			if tt.wantIssues == 0 {
				return
			}
			if issues[0].Category != CategoryStale {
				t.Errorf("category = %q, want %q", issues[0].Category, CategoryStale)
			}
			if issues[0].Type != DriftTypePlan {
				t.Errorf("type = %q, want %q", issues[0].Type, DriftTypePlan)
			}
			if tt.wantContains != "" && !strings.Contains(issues[0].Message, tt.wantContains) {
				t.Errorf("message %q should mention %q", issues[0].Message, tt.wantContains)
			}
			if issues[0].Hint == "" {
				t.Error("a staleness issue should say what to do about it")
			}
		})
	}
}

func TestStalenessSeverityScalesWithDivergence(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	detector := NewDriftDetector()
	plan := &planning.Plan{UpdatedAt: now.AddDate(0, 0, -60), Tasks: []planning.Task{{ID: "t1"}}}

	modest := detector.DetectStalenessDrift(plan, RepoActivity{CommitsSincePlan: 20, LastCommitAt: now}, now)
	severe := detector.DetectStalenessDrift(plan, RepoActivity{CommitsSincePlan: 500, LastCommitAt: now}, now)

	if len(modest) != 1 || len(severe) != 1 {
		t.Fatalf("expected one issue each, got %d and %d", len(modest), len(severe))
	}
	if severityRank(severe[0].Severity) >= severityRank(modest[0].Severity) {
		t.Errorf("500 commits behind (%s) should outrank 20 (%s)", severe[0].Severity, modest[0].Severity)
	}
}
