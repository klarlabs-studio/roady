package drift

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// mcp-go's plan named every feature by its title. Those features exist, so
// the tasks are a link to repair, not orphans of a feature the spec lost.
func TestTitleLinkedTasksAreOneLinkIssue(t *testing.T) {
	sp := &spec.ProductSpec{Features: []spec.Feature{
		{ID: "revisions", Title: "Spec Revisions Alignment (2024-11-05 → 2026-07-28)"},
		{ID: "roadmap-2026", Title: "2026 Roadmap Alignment"},
	}}
	plan := &planning.Plan{Tasks: []planning.Task{
		{ID: "a", FeatureID: "Spec Revisions Alignment (2024-11-05 → 2026-07-28)"},
		{ID: "b", FeatureID: "2026 roadmap alignment"},
		{ID: "c", FeatureID: "roadmap-2026"},
		{ID: "d", FeatureID: "Something Removed"},
	}}
	issues := NewDriftDetector().DetectPlanDrift(sp, plan)

	var link, orphans []Issue
	for _, is := range issues {
		switch is.Category {
		case CategoryLink:
			link = append(link, is)
		case CategoryOrphan:
			orphans = append(orphans, is)
		}
	}
	if len(link) != 1 || !strings.Contains(link[0].Message, "2 tasks") || !strings.Contains(link[0].Message, "a, b") ||
		!strings.Contains(link[0].Hint, "roady doctor --fix") {
		t.Errorf("link issues: %+v", link)
	}
	if len(orphans) != 1 || orphans[0].ComponentID != "d" {
		t.Errorf("orphans: %+v, want only d", orphans)
	}
}
