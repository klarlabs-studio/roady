package application_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func renderSpec() *spec.ProductSpec {
	return &spec.ProductSpec{ID: "s", Title: "S",
		Goals: []spec.Goal{
			{ID: "g-now", Title: "Offline mode", Horizon: spec.HorizonNow, Milestone: "v2", Description: "Work without a network."},
			{ID: "g-cloud", Title: "Cloud", Horizon: spec.HorizonLater, Status: spec.GoalIdea},
			{ID: "g-old", Title: "First release", Status: spec.GoalShipped, Milestone: "v1"},
			{ID: "g-no", Title: "Jira parity", Status: spec.GoalOutOfScope, Description: "Use a tracker."},
		},
		Features: []spec.Feature{{ID: "sync", Title: "Sync", Goal: "g-now"}, {ID: "other", Goal: "g-now"}},
	}
}

func TestRenderRoadmapMarkdown(t *testing.T) {
	md := application.RenderRoadmapMarkdown(renderSpec())
	for _, want := range []string{
		"<!-- roady:roadmap sha256=",
		"# Roadmap\n",
		"## Now\n\n### Offline mode (v2)\n\nWork without a network.\n\nFeatures: Sync (`sync`), `other`\n",
		"## Later\n\n### Cloud\n\n_Idea — not yet agreed._\n",
		"## Shipped\n\n### First release (v1)\n",
		"## Out of scope\n\n- **Jira parity** — Use a tracker.\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered roadmap lacks %q:\n%s", want, md)
		}
	}
	if application.RenderRoadmapMarkdown(renderSpec()) != md {
		t.Error("rendering is not deterministic")
	}
	if !strings.Contains(application.RenderRoadmapMarkdown(&spec.ProductSpec{}), "No goals yet.") {
		t.Error("an empty roadmap should say so")
	}
}

// The rendered file round-trips; a hand edit, and goals moving on, are each
// told apart from a file roady did not write.
func TestCompareRoadmap(t *testing.T) {
	sp := renderSpec()
	md := application.RenderRoadmapMarkdown(sp)
	if got := application.CompareRoadmap(md, sp); got != application.RoadmapInSync {
		t.Fatalf("a fresh render reads as %s", got)
	}
	if got := application.CompareRoadmap(strings.ReplaceAll(md, "\n", "\r\n"), sp); got != application.RoadmapInSync {
		t.Errorf("a CRLF checkout of the same file reads as %s", got)
	}
	edited := strings.Replace(md, "Work without a network.", "Work without a network, mostly.", 1)
	if got := application.CompareRoadmap(edited, sp); got != application.RoadmapHandEdited {
		t.Errorf("a hand edit reads as %s", got)
	}
	sp.Goals[0].Horizon = spec.HorizonNext
	if got := application.CompareRoadmap(md, sp); got != application.RoadmapOutOfDate {
		t.Errorf("a roadmap the goals moved past reads as %s", got)
	}
	if got := application.CompareRoadmap("# Roadmap\n\nhand written\n", sp); got != application.RoadmapNotRendered {
		t.Errorf("a hand-written file reads as %s", got)
	}
}

func TestRoadmapDriftAndWrite(t *testing.T) {
	sp := renderSpec()
	path := filepath.Join(t.TempDir(), "ROADMAP.md")

	if issues := application.RoadmapDrift(path, sp); issues != nil {
		t.Errorf("a missing roadmap is not drift: %+v", issues)
	}
	res, err := application.WriteRoadmap(path, sp, false)
	if err != nil || !res.Written || !res.Missing {
		t.Fatalf("first render: %+v, %v", res, err)
	}
	if issues := application.RoadmapDrift(path, sp); issues != nil {
		t.Errorf("a fresh render is not drift: %+v", issues)
	}
	if res, _ = application.WriteRoadmap(path, sp, false); res.Written {
		t.Error("an up-to-date roadmap was rewritten")
	}

	// Goals move: stale, and a plain render fixes it.
	sp.Goals[1].Horizon = spec.HorizonNext
	issues := application.RoadmapDrift(path, sp)
	if len(issues) != 1 || issues[0].Category != drift.CategoryStale || issues[0].Type != drift.DriftTypeDoc {
		t.Fatalf("out of date: %+v", issues)
	}
	if res, err = application.WriteRoadmap(path, sp, false); err != nil || !res.Written || res.Before != application.RoadmapOutOfDate {
		t.Fatalf("re-render: %+v, %v", res, err)
	}

	// A hand edit is drift, and is not overwritten without force.
	raw, _ := os.ReadFile(path)
	_ = os.WriteFile(path, []byte(string(raw)+"\n- a note added by hand\n"), 0o644)
	issues = application.RoadmapDrift(path, sp)
	if len(issues) != 1 || issues[0].Category != drift.CategoryMismatch || !strings.Contains(issues[0].Hint, "roady goal") {
		t.Fatalf("hand edit: %+v", issues)
	}
	if _, err = application.WriteRoadmap(path, sp, false); !errors.Is(err, application.ErrRoadmapHandEdited) {
		t.Fatalf("a hand-edited roadmap must not be replaced silently: %v", err)
	}
	if res, err = application.WriteRoadmap(path, sp, true); err != nil || !res.Written {
		t.Fatalf("forced render: %+v, %v", res, err)
	}

	// A file roady never wrote is left alone too.
	_ = os.WriteFile(path, []byte("# Our roadmap\n"), 0o644)
	if issues := application.RoadmapDrift(path, sp); issues != nil {
		t.Errorf("a file roady did not write is not drift: %+v", issues)
	}
	if _, err = application.WriteRoadmap(path, sp, false); !errors.Is(err, application.ErrRoadmapHandEdited) {
		t.Errorf("a hand-written roadmap must not be replaced silently: %v", err)
	}
}

func TestDriftServiceReportsTheRoadmap(t *testing.T) {
	sp := renderSpec()
	repo := &MockRepo{Spec: sp}
	_ = repo.SaveSpecLock(sp)
	svc := application.NewDriftService(repo, application.NewAuditService(repo), nil, application.NewPolicyService(repo))
	path := filepath.Join(t.TempDir(), "ROADMAP.md")
	md := application.RenderRoadmapMarkdown(sp)
	_ = os.WriteFile(path, []byte(strings.Replace(md, "Use a tracker.", "Use Jira.", 1)), 0o644)

	report, err := svc.DetectDrift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range report.Issues {
		if i.Type == drift.DriftTypeDoc {
			t.Fatal("no roadmap path set, yet the roadmap was checked")
		}
	}
	svc.SetRoadmapPath(path)
	report, _ = svc.DetectDrift(t.Context())
	found := false
	for _, i := range report.Issues {
		found = found || i.ID == "roadmap-hand-edited"
	}
	if !found {
		t.Errorf("drift did not report the hand edit: %+v", report.Issues)
	}
}
