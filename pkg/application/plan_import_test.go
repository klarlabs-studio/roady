package application

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func writePlan(t *testing.T, dir, rel, body string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func taskByTitle(t *testing.T, imp *PlanImport, title string) CaptureTask {
	t.Helper()
	for _, ct := range imp.Doc.Tasks {
		if ct.Title != nil && *ct.Title == title {
			return ct
		}
	}
	var got []string
	for _, ct := range imp.Doc.Tasks {
		got = append(got, *ct.Title)
	}
	t.Fatalf("no task titled %q; got %q", title, got)
	return CaptureTask{}
}

const kiroTasks = `# Implementation Plan

- [x] 1. Set up project structure
  - Create directories for models and services
  - _Requirements: 1.1_

- [ ] 2. Implement data models
- [ ] 2.1 Create the User model with validation
  - _Requirements: 1.2, 2.1_
- [ ] 2.2 Write unit tests for the User model
  - _Requirements: 1.2_

- [ ] 3. Wire the login endpoint
`

func TestPlanImport_Kiro(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, ".kiro/specs/user-auth/tasks.md", kiroTasks)

	imp, err := ImportPlanFile(path, &spec.ProductSpec{}, PlanImportOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Format != PlanFormatKiro {
		t.Fatalf("format = %s, want kiro", imp.Format)
	}
	if imp.Title != "user auth" {
		t.Errorf("title = %q, want the spec directory's name", imp.Title)
	}
	if imp.Skipped != 1 || len(imp.Doc.Tasks) != 4 {
		t.Fatalf("skipped %d, tasks %d; want the done step skipped and 4 imported", imp.Skipped, len(imp.Doc.Tasks))
	}
	if len(imp.Doc.Features) != 1 || imp.Doc.Features[0].ID != "user-auth" {
		t.Fatalf("want a feature named after the plan, got %+v", imp.Doc.Features)
	}

	parent := taskByTitle(t, imp, "Implement data models")
	model := taskByTitle(t, imp, "Create the User model with validation")
	tests := taskByTitle(t, imp, "Write unit tests for the User model")
	login := taskByTitle(t, imp, "Wire the login endpoint")

	if parent.ID != "task-user-auth-implement-data-models" {
		t.Errorf("id = %s: want task-<plan>-<step> so a re-import upserts", parent.ID)
	}
	if got := *parent.DependsOn; len(got) != 2 || got[0] != model.ID || got[1] != tests.ID {
		t.Errorf("parent depends on %v; want its sub-tasks (and not the skipped done step)", got)
	}
	if len(*model.DependsOn) != 0 {
		t.Errorf("sub-task depends on %v; a sub-task is not chained to its sibling", *model.DependsOn)
	}
	if got := *login.DependsOn; len(got) != 1 || got[0] != parent.ID {
		t.Errorf("step 3 depends on %v; want step 2", got)
	}
	if model.Description == nil || !strings.Contains(*model.Description, "Requirements: 1.2, 2.1") {
		t.Errorf("the _Requirements_ line should carry into the description: %v", model.Description)
	}
	if model.Source == nil || model.Source.Doc != ".kiro/specs/user-auth/tasks.md" || model.Source.Line != 8 {
		t.Errorf("source = %+v; want the project-relative path and the step's line", model.Source)
	}
}

const execPlan = `# Add offline sync

## Purpose / Big Picture

Users keep working without a connection.

## Progress

- [x] (2026-09-01 10:00Z) Spike the local queue.
- [ ] (2026-09-02 09:00Z) Persist queued writes to IndexedDB.
- [ ] Replay the queue on reconnect.

## Surprises & Discoveries

- Safari evicts storage after seven days.

## Decision Log

- Decision: last-writer-wins.

## Outcomes & Retrospective

## Concrete Steps

1. Run the migration.
`

func TestPlanImport_ExecPlan(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, "PLANS.md", execPlan)

	imp, err := ImportPlanFile(path, &spec.ProductSpec{}, PlanImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Format != PlanFormatExecPlan {
		t.Fatalf("format = %s, want execplan", imp.Format)
	}
	if len(imp.Doc.Tasks) != 2 || imp.Skipped != 1 {
		t.Fatalf("tasks %d skipped %d; want the Progress list only, done step skipped", len(imp.Doc.Tasks), imp.Skipped)
	}
	persist := taskByTitle(t, imp, "Persist queued writes to IndexedDB")
	replay := taskByTitle(t, imp, "Replay the queue on reconnect")
	if persist.ID != "task-add-offline-sync-persist-queued-writes-to-indexeddb" {
		t.Errorf("id = %s: the timestamp must not leak into the id", persist.ID)
	}
	if got := *replay.DependsOn; len(got) != 1 || got[0] != persist.ID {
		t.Errorf("replay depends on %v, want %s", got, persist.ID)
	}

	all, err := ImportPlanFile(path, &spec.ProductSpec{}, PlanImportOptions{IncludeDone: true, Parallel: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Doc.Tasks) != 3 {
		t.Fatalf("--include-done: %d tasks, want 3", len(all.Doc.Tasks))
	}
	for _, ct := range all.Doc.Tasks {
		if len(*ct.DependsOn) != 0 {
			t.Errorf("--parallel: %s depends on %v", ct.ID, *ct.DependsOn)
		}
	}
}

const claudePlan = "# Plan: Rate-limit the public API\n\n" +
	"## Context\n\nThe API has no limits.\n\n- Not a step: context bullets are narrative.\n\n" +
	"## Steps\n\n" +
	"1. **Add a token bucket** in `internal/ratelimit/bucket.go`. Keep it lock-free.\n" +
	"2. Wire the middleware into the router\n   - apply to /v1 only\n" +
	"3. Return `429` with a Retry-After header\n\n" +
	"## Verification\n\n- Run `go test ./internal/ratelimit`\n\n" +
	"```bash\n- not a step\n```\n"

func TestPlanImport_ClaudeMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, "plans/rate-limit.md", claudePlan)
	current := &spec.ProductSpec{Features: []spec.Feature{{ID: "api", Title: "Public API"}}}

	imp, err := ImportPlanFile(path, current, PlanImportOptions{FeatureID: "api"})
	if err != nil {
		t.Fatal(err)
	}
	if imp.Format != PlanFormatMarkdown || imp.Title != "Rate-limit the public API" {
		t.Fatalf("format %s title %q", imp.Format, imp.Title)
	}
	if len(imp.Doc.Features) != 0 {
		t.Errorf("--feature attaches to an existing feature; got new features %+v", imp.Doc.Features)
	}
	if len(imp.Doc.Tasks) != 3 {
		t.Fatalf("got %d tasks, want the three under Steps only", len(imp.Doc.Tasks))
	}
	// A bold lead is the step's title; the rest of the line is its detail.
	bucket := taskByTitle(t, imp, "Add a token bucket")
	if bucket.Description == nil || !strings.Contains(*bucket.Description, "internal/ratelimit/bucket.go") {
		t.Errorf("the text after the bold lead belongs in the description: %v", bucket.Description)
	}
	if *bucket.FeatureID != "api" {
		t.Errorf("feature = %s", *bucket.FeatureID)
	}
	if !strings.HasPrefix(bucket.ID, "task-rate-limit-the-public-api-") {
		t.Errorf("id = %s", bucket.ID)
	}
	wire := taskByTitle(t, imp, "Wire the middleware into the router")
	if wire.Description == nil || !strings.Contains(*wire.Description, "apply to /v1 only") {
		t.Errorf("nested bullet should fold into the description: %v", wire.Description)
	}

	if _, err := ImportPlanFile(path, current, PlanImportOptions{FeatureID: "nope"}); err == nil {
		t.Error("an unknown --feature must be refused")
	}
}

func TestPlanImport_HeadingsOnly(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, "plan.md", `# Migrate to Postgres

## Overview
Why we move.

## Step 1: Dual-write
Write to both stores.

## Step 2: Backfill
Copy history.

## Risks
Downtime.
`)
	imp, err := ImportPlanFile(path, nil, PlanImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Doc.Tasks) != 2 {
		t.Fatalf("got %d tasks, want one per non-narrative heading", len(imp.Doc.Tasks))
	}
	dual := taskByTitle(t, imp, "Dual-write")
	if dual.Description == nil || !strings.Contains(*dual.Description, "Write to both stores.") {
		t.Errorf("description = %v", dual.Description)
	}
	taskByTitle(t, imp, "Backfill")
}

func TestPlanImport_Errors(t *testing.T) {
	dir := t.TempDir()
	done := writePlan(t, dir, "done.md", "# Done\n\n## Steps\n\n- [x] one\n- [x] two\n")
	if _, err := ImportPlanFile(done, nil, PlanImportOptions{}); err == nil || !strings.Contains(err.Error(), "--include-done") {
		t.Errorf("all-done plan: %v", err)
	}
	empty := writePlan(t, dir, "empty.md", "# Nothing\n\nJust prose.\n")
	if _, err := ImportPlanFile(empty, nil, PlanImportOptions{}); err == nil {
		t.Error("a plan with no steps must be refused")
	}
	if _, err := ImportPlanFile(done, nil, PlanImportOptions{Format: "jira"}); err == nil {
		t.Error("an unknown format must be refused")
	}
}
