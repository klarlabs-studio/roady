package application

import (
	"strings"
	"testing"
)

// mcp-go keeps its roadmap by phase: a status table, a "Remaining" list,
// then one ## section per phase full of checkbox bullets.
const phaseRoadmap = `# mcp-go Spec Revisions Roadmap

Plan to bring mcp-go current.

## Guiding strategy

Negotiate, do not fork.

## Phase 0 — Foundation (v1.22.0)

Version negotiation and the conformance harness.
Everything later builds on it.

- [x] Protocol version negotiation
- [x] Conformance harness

## Phase 1 — Certify 2025-03-26 (v1.23.0)

Streamable HTTP and annotations.

- [x] Streamable HTTP

## Phase 5 — Next revision

### Track the Transports WG

HTTP/2 over stdio.
`

func TestGoalImportSectionMapWholeSections(t *testing.T) {
	opts, err := RoadmapImportOptionsFrom([]string{"Phase 5=next"}, []string{"Phase=shipped"})
	if err != nil {
		t.Fatal(err)
	}
	imp, err := ImportRoadmapMarkdownWith(strings.NewReader(phaseRoadmap), opts)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]CaptureGoal{}
	for _, g := range imp.Doc.Goals {
		got[*g.Title] = g
	}
	p0, ok := got["Phase 0 — Foundation"]
	if !ok || p0.Status == nil || *p0.Status != "shipped" || p0.Milestone == nil || *p0.Milestone != "v1.22.0" {
		t.Fatalf("phase 0: %+v (all: %v)", p0, keys(got))
	}
	if p0.Description == nil || *p0.Description != "Version negotiation and the conformance harness. Everything later builds on it." {
		t.Errorf("phase 0 description: %v", deref(p0.Description))
	}
	if _, ok := got["Phase 1 — Certify 2025-03-26"]; !ok {
		t.Errorf("phase 1 missing: %v", keys(got))
	}
	// The longer prefix wins: Phase 5 is a horizon section, read goal by goal.
	if g, ok := got["Track the Transports WG"]; !ok || g.Horizon == nil || *g.Horizon != "next" {
		t.Errorf("phase 5 goal: %+v (all: %v)", g, keys(got))
	}
	for title := range got {
		if strings.Contains(title, "Protocol version negotiation") || strings.HasPrefix(title, "Phase 5") {
			t.Errorf("imported %q: checkbox bullets and mapped sections are not goals of their own", title)
		}
	}
	if len(imp.Skipped) != 1 || imp.Skipped[0] != "Guiding strategy" {
		t.Errorf("skipped %v, want only Guiding strategy", imp.Skipped)
	}
}

func TestGoalImportSectionMapRefusesWhatMatchesNothing(t *testing.T) {
	opts, _ := RoadmapImportOptionsFrom([]string{"Phsae=now"}, nil)
	if _, err := ImportRoadmapMarkdownWith(strings.NewReader(phaseRoadmap), opts); err == nil || !strings.Contains(err.Error(), `no ## section starts with "Phsae"`) {
		t.Errorf("a mapping matching nothing: %v", err)
	}
	if _, err := RoadmapImportOptionsFrom([]string{"Phase=soonish"}, nil); err == nil || !strings.Contains(err.Error(), "not a horizon or status") {
		t.Errorf("an unknown target: %v", err)
	}
	if _, err := RoadmapImportOptionsFrom([]string{"Phase"}, nil); err == nil {
		t.Error("a mapping without = was accepted")
	}
}

func keys(m map[string]CaptureGoal) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
