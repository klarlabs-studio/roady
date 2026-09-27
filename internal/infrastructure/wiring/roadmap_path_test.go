package wiring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// nexa renders its roadmap to memory/roadmap.md. With roadmap: set in the
// policy, drift checks that file instead of ROADMAP.md.
func TestRoadmapPathFromPolicy(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	sp := &spec.ProductSpec{ID: "s", Title: "S", Goals: []spec.Goal{{ID: "g", Title: "Offline", Horizon: spec.HorizonNow}}}
	_ = repo.SaveSpec(sp)
	_ = repo.SaveSpecLock(sp)
	_ = repo.SavePlan(&planning.Plan{ID: "p", SpecID: "s", ApprovalStatus: planning.ApprovalApproved})
	_ = repo.SaveState(planning.NewExecutionState("p"))
	_ = repo.SavePolicy(&domain.PolicyConfig{AllowAI: true, Roadmap: "memory/roadmap.md"})

	path := filepath.Join(root, "memory", "roadmap.md")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if _, err := application.WriteRoadmap(path, sp, false); err != nil {
		t.Fatal(err)
	}
	// The goals move on; the rendered file does not.
	sp.Goals = append(sp.Goals, spec.Goal{ID: "g2", Title: "Sync", Horizon: spec.HorizonNext})
	_ = repo.SaveSpec(sp)

	svc, err := BuildAppServices(root)
	if err != nil {
		t.Fatal(err)
	}
	report, err := svc.Drift.DetectDrift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, is := range report.Issues {
		if is.Path == path && strings.Contains(is.Message, "roadmap.md no longer matches the goals") {
			found = true
		}
	}
	if !found {
		t.Errorf("stale memory/roadmap.md not reported: %+v", report.Issues)
	}

	if p, ok := application.RoadmapPath(root, nil, false); !ok || p != filepath.Join(root, "ROADMAP.md") {
		t.Errorf("default %s %v", p, ok)
	}
	if _, ok := application.RoadmapPath(root, &domain.PolicyConfig{}, true); ok {
		t.Error("a sub-project has no default roadmap file")
	}
	if p, ok := application.RoadmapPath(root, &domain.PolicyConfig{Roadmap: "docs/r.md"}, true); !ok || p != filepath.Join(root, "docs/r.md") {
		t.Errorf("sub-project with a setting: %s %v", p, ok)
	}
}
