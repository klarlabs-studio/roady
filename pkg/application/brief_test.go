package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func briefRepo() *MockRepo {
	return &MockRepo{
		Plan: &planning.Plan{ApprovalStatus: planning.ApprovalApproved, Tasks: []planning.Task{
			{ID: "a", Title: "Low one", Priority: "low"},
			{ID: "b", Title: "High one", Priority: "high", Description: "Because invoices must be gap-free.",
				Source: planning.TaskSource{Doc: "docs/prd.md", Line: 15}, Check: &planning.Check{Run: "go test ./inv"}},
			{ID: "c", Title: "After b", Priority: "medium", DependsOn: []string{"b"}},
		}},
		State: &planning.ExecutionState{TaskStates: map[string]planning.TaskResult{
			"a": {Status: planning.StatusPending}, "b": {Status: planning.StatusPending}, "c": {Status: planning.StatusPending},
		}},
	}
}

func brief(t *testing.T, repo *MockRepo, owner string) *application.TaskBrief {
	t.Helper()
	svc := application.NewTaskService(repo, application.NewAuditService(repo), nil)
	b, err := svc.Brief(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// With nothing in progress the brief suggests the highest-priority ready
// task, not the first in plan order.
func TestBriefSuggestsHighestPriorityReadyTask(t *testing.T) {
	b := brief(t, briefRepo(), "felix")
	if b.Mode != "suggested" || b.Task.ID != "b" {
		t.Fatalf("expected task b suggested, got %s %+v", b.Mode, b.Task)
	}
	out := b.Render()
	for _, want := range []string{"roady task start b", "Source: docs/prd.md:15", "`go test ./inv` passes", "Unblocks: c", "[roady:b]"} {
		if !strings.Contains(out, want) {
			t.Errorf("brief lacks %q:\n%s", want, out)
		}
	}
}

func TestBriefFocusesOnTheOwnersActiveTask(t *testing.T) {
	repo := briefRepo()
	repo.State.TaskStates["a"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "Felix"}
	repo.State.TaskStates["b"] = planning.TaskResult{Status: planning.StatusInProgress, Owner: "someone-else"}
	b := brief(t, repo, "felix")
	if b.Mode != "active" || b.Task.ID != "a" {
		t.Fatalf("expected felix's in-progress task a, got %s %+v", b.Mode, b.Task)
	}
	if !strings.HasPrefix(b.Render(), "Roady — current task") {
		t.Error("an active brief must say it is the current task")
	}
}

func TestBriefWithNothingToDo(t *testing.T) {
	repo := briefRepo()
	for id := range repo.State.TaskStates {
		repo.State.TaskStates[id] = planning.TaskResult{Status: planning.StatusVerified}
	}
	b := brief(t, repo, "felix")
	if b.Mode != "none" || !strings.Contains(b.Render(), "no task in progress and none ready") {
		t.Fatalf("got %s:\n%s", b.Mode, b.Render())
	}
}

// The brief is injected unconditionally, so it must stay small whatever the
// requirement text: roughly 500 tokens is the budget.
func TestBriefStaysWithinBudget(t *testing.T) {
	repo := briefRepo()
	repo.Plan.Tasks[1].Description = strings.Repeat("A very long requirement description. ", 400)
	out := brief(t, repo, "felix").Render()
	if len(out) > 2000 {
		t.Fatalf("brief is %d chars (~%d tokens); budget is ~500 tokens", len(out), len(out)/4)
	}
}
