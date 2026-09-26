package application_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
)

// An import is a capture: running it twice changes nothing the second time,
// and every task cites the plan's line.
func TestPlanImport_CaptureIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	body := "# Plan: Rate limits\n\n## Steps\n\n1. Add a token bucket\n2. Wire the middleware\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := captureRepo()
	svc := application.NewCaptureService(repo, application.NewAuditService(repo))

	for i, wantChanged := range []bool{true, false} {
		imp, err := application.ImportPlanFile(path, repo.Spec, application.PlanImportOptions{Root: dir})
		if err != nil {
			t.Fatal(err)
		}
		res, err := svc.Capture(imp.Doc, application.CaptureOptions{Actor: "test"})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Rejected) > 0 {
			t.Fatalf("rejected: %+v", res.Rejected)
		}
		if res.Changed() != wantChanged {
			t.Errorf("run %d: changed = %v, want %v (%+v)", i+1, res.Changed(), wantChanged, res)
		}
	}
	if len(repo.Plan.Tasks) != 2 {
		t.Fatalf("plan has %d tasks, want 2", len(repo.Plan.Tasks))
	}
	second := repo.Plan.Tasks[1]
	if second.Source.Doc != "plan.md" || second.Source.Line != 6 {
		t.Errorf("source = %+v, want plan.md:6", second.Source)
	}
	if len(second.DependsOn) != 1 || second.DependsOn[0] != repo.Plan.Tasks[0].ID {
		t.Errorf("depends on %v, want the first step", second.DependsOn)
	}
}
