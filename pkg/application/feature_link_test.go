package application

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func linkFixture() []spec.Feature {
	return []spec.Feature{
		{ID: "phase-a-pilot-finalization-2-weeks", Title: "Phase A — Pilot Finalization (2 weeks)"},
		{ID: "user-authentication", Title: "User Authentication"},
	}
}

// The reported plan carried feature TITLES in feature_id, so drift reported
// every task as an orphan the moment it was written — the plan was born
// drifted. Roady knows the title, so it can repair the link instead of
// storing something it will complain about later. See issue #73.
func TestResolveFeatureLinksRepairsTitles(t *testing.T) {
	tasks := []planning.Task{
		{ID: "task-1", FeatureID: "Phase A — Pilot Finalization (2 weeks)"},
		{ID: "task-2", FeatureID: "User Authentication"},
	}

	warnings := resolveFeatureLinks(linkFixture(), tasks)

	if tasks[0].FeatureID != "phase-a-pilot-finalization-2-weeks" {
		t.Errorf("task-1 feature_id = %q, want the feature's id", tasks[0].FeatureID)
	}
	if tasks[1].FeatureID != "user-authentication" {
		t.Errorf("task-2 feature_id = %q, want the feature's id", tasks[1].FeatureID)
	}
	if len(warnings) != 0 {
		t.Errorf("repairing a title should be silent, got %v", warnings)
	}
}

// An id written by an older Roady, before the slugifier was fixed, still
// names a real feature.
func TestResolveFeatureLinksRepairsLegacySlugs(t *testing.T) {
	tasks := []planning.Task{
		{ID: "task-1", FeatureID: "phase-a-—-pilot-finalization-(2-weeks)"},
	}

	resolveFeatureLinks(linkFixture(), tasks)

	if tasks[0].FeatureID != "phase-a-pilot-finalization-2-weeks" {
		t.Errorf("feature_id = %q, want the current id", tasks[0].FeatureID)
	}
}

func TestResolveFeatureLinksLeavesCorrectLinksAlone(t *testing.T) {
	tasks := []planning.Task{{ID: "task-1", FeatureID: "user-authentication"}}

	warnings := resolveFeatureLinks(linkFixture(), tasks)

	if tasks[0].FeatureID != "user-authentication" {
		t.Errorf("a correct link was rewritten to %q", tasks[0].FeatureID)
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

// A link Roady cannot resolve must be reported. Accepting it silently is what
// produced a plan that was 63%% orphaned before anyone looked at it.
func TestResolveFeatureLinksReportsUnknownFeatures(t *testing.T) {
	tasks := []planning.Task{
		{ID: "task-1", FeatureID: "no-such-feature"},
		{ID: "task-2", FeatureID: "user-authentication"},
	}

	warnings := resolveFeatureLinks(linkFixture(), tasks)

	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "task-1") || !strings.Contains(warnings[0], "no-such-feature") {
		t.Errorf("warning names neither the task nor the feature: %q", warnings[0])
	}
	// The value is left as written rather than guessed at.
	if tasks[0].FeatureID != "no-such-feature" {
		t.Errorf("an unresolvable link was rewritten to %q", tasks[0].FeatureID)
	}
}

func TestResolveFeatureLinksReportsMissingLink(t *testing.T) {
	tasks := []planning.Task{{ID: "task-1", FeatureID: ""}}

	warnings := resolveFeatureLinks(linkFixture(), tasks)

	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "task-1") {
		t.Errorf("warning does not name the task: %q", warnings[0])
	}
}

// Case and surrounding whitespace are not a reason to orphan a task.
func TestResolveFeatureLinksIsForgivingAboutCase(t *testing.T) {
	tasks := []planning.Task{
		{ID: "task-1", FeatureID: "  user authentication  "},
		{ID: "task-2", FeatureID: "USER-AUTHENTICATION"},
	}

	warnings := resolveFeatureLinks(linkFixture(), tasks)

	for _, task := range tasks {
		if task.FeatureID != "user-authentication" {
			t.Errorf("%s feature_id = %q, want user-authentication", task.ID, task.FeatureID)
		}
	}
	if len(warnings) != 0 {
		t.Errorf("unexpected warnings: %v", warnings)
	}
}

func TestResolveFeatureLinksHandlesNoSpec(t *testing.T) {
	tasks := []planning.Task{{ID: "task-1", FeatureID: "anything"}}

	// With no features to match against, nothing can be resolved — but the
	// caller should not be drowned in a warning per task either.
	warnings := resolveFeatureLinks(nil, tasks)

	if tasks[0].FeatureID != "anything" {
		t.Errorf("feature_id was rewritten to %q with no spec to match against", tasks[0].FeatureID)
	}
	if len(warnings) != 0 {
		t.Errorf("an empty spec should not produce per-task warnings, got %v", warnings)
	}
}
