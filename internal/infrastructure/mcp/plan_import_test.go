package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlePlanImport(t *testing.T) {
	dir := t.TempDir()
	s, err := NewServer(dir)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	if _, err := s.handleInit(context.Background(), InitArgs{Name: "import"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	plan := "# Plan: Rate limits\n\n## Steps\n\n1. Add a token bucket\n2. Wire the middleware\n"
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(plan), 0o600); err != nil {
		t.Fatal(err)
	}

	res, err := s.handlePlanImport(context.Background(), PlanImportArgs{Path: "plan.md"})
	if err != nil {
		t.Fatal(err)
	}
	out, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("expected a result map, got %T: %+v", res, res)
	}
	if out["format"] != "markdown" || out["steps"] != 2 {
		t.Errorf("got %+v", out)
	}
	p, err := s.planSvc.GetPlan()
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, task := range p.Tasks {
		if task.Source.Doc == "plan.md" {
			found++
		}
	}
	if found != 2 {
		t.Errorf("want 2 tasks citing plan.md, found %d", found)
	}

	res, _ = s.handlePlanImport(context.Background(), PlanImportArgs{Path: "missing.md"})
	assertToolError(t, res, nil, "Failed to import plan")
}
