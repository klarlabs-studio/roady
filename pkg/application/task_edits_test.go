package application_test

import (
	"reflect"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func editsRepo(t *testing.T) *MockRepo {
	t.Helper()
	repo := captureRepo()
	capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{
		{ID: "task-a", Title: str("A"), FeatureID: str("base")},
		{ID: "task-b", Title: str("B"), FeatureID: str("base"), DependsOn: &[]string{"task-a"}},
	}})
	return repo
}

func taskIn(t *testing.T, p *planning.Plan, id string) planning.Task {
	t.Helper()
	for _, task := range p.Tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("no task %s", id)
	return planning.Task{}
}

func TestAddTaskDoc(t *testing.T) {
	repo := editsRepo(t)
	doc, id, err := application.AddTaskDoc(repo.Plan, application.AddTask{
		Title: "Handle empty input", After: []string{"task-a"}, Before: []string{"task-b"}, Priority: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id != "task-handle-empty-input" {
		t.Errorf("id = %s", id)
	}
	res := capture(t, repo, doc)
	if len(res.Rejected) > 0 || !res.Applied {
		t.Fatalf("%+v", res)
	}
	added := taskIn(t, repo.Plan, id)
	if added.FeatureID != "base" || !reflect.DeepEqual(added.DependsOn, []string{"task-a"}) || added.Priority != "high" {
		t.Errorf("added = %+v: want base's feature via --after, depending on task-a", added)
	}
	if b := taskIn(t, repo.Plan, "task-b"); !reflect.DeepEqual(b.DependsOn, []string{"task-a", id}) {
		t.Errorf("--before: task-b depends on %v", b.DependsOn)
	}

	// The same title again is the same task, not a duplicate.
	doc, id2, err := application.AddTaskDoc(repo.Plan, application.AddTask{Title: "Handle empty input", After: []string{"task-a"}, Before: []string{"task-b"}, Priority: "high"})
	if err != nil || id2 != id {
		t.Fatalf("re-add: %s %v", id2, err)
	}
	if res := capture(t, repo, doc); res.Changed() {
		t.Errorf("re-adding changed the plan: %+v", res)
	}

	// A different task with a colliding slug gets a suffix.
	repo.Plan.Tasks[0].Title = "Something else"
	if _, id3, _ := application.AddTaskDoc(repo.Plan, application.AddTask{Title: "a", Feature: "base"}); id3 != "task-a-2" {
		t.Errorf("collision id = %s", id3)
	}

	if _, _, err := application.AddTaskDoc(repo.Plan, application.AddTask{Title: "Nowhere"}); err == nil {
		t.Error("a task with no home must be refused before capture")
	}
	if _, _, err := application.AddTaskDoc(repo.Plan, application.AddTask{Title: "X", Feature: "base", Before: []string{"nope"}}); err == nil {
		t.Error("--before an unknown task must be refused")
	}
}

func TestEditTaskDoc(t *testing.T) {
	repo := editsRepo(t)
	doc, err := application.EditTaskDoc(repo.Plan, "task-b", application.EditTask{
		Priority: str("low"), AddDeps: []string{"task-c"}, DropDeps: []string{"task-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-c", Title: str("C"), FeatureID: str("base")}}})
	if res := capture(t, repo, doc); len(res.Rejected) > 0 {
		t.Fatalf("%+v", res.Rejected)
	}
	b := taskIn(t, repo.Plan, "task-b")
	if b.Priority != "low" || !reflect.DeepEqual(b.DependsOn, []string{"task-c"}) || b.Title != "B" {
		t.Errorf("edited = %+v", b)
	}
	if _, err := application.EditTaskDoc(repo.Plan, "task-b", application.EditTask{}); err == nil {
		t.Error("an edit with no fields must say so")
	}
	if _, err := application.EditTaskDoc(repo.Plan, "nope", application.EditTask{Title: str("x")}); err == nil {
		t.Error("unknown task")
	}
}

func TestSplitTaskDoc(t *testing.T) {
	repo := editsRepo(t)
	doc, ids, err := application.SplitTaskDoc(repo.Plan, "task-b", []string{"Lay out", "Write file"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res := capture(t, repo, doc); len(res.Rejected) > 0 {
		t.Fatalf("%+v", res.Rejected)
	}
	if !reflect.DeepEqual(ids, []string{"task-b-lay-out", "task-b-write-file"}) {
		t.Errorf("ids = %v", ids)
	}
	for _, id := range ids {
		if p := taskIn(t, repo.Plan, id); !reflect.DeepEqual(p.DependsOn, []string{"task-a"}) || p.FeatureID != "base" {
			t.Errorf("part %s = %+v: parts inherit the task's dependencies and feature", id, p)
		}
	}
	if b := taskIn(t, repo.Plan, "task-b"); !reflect.DeepEqual(b.DependsOn, ids) {
		t.Errorf("the split task waits for its parts: %v", b.DependsOn)
	}

	doc, ids, _ = application.SplitTaskDoc(repo.Plan, "task-a", []string{"One", "Two", "Three"}, true)
	capture(t, repo, doc)
	if p := taskIn(t, repo.Plan, ids[2]); !reflect.DeepEqual(p.DependsOn, []string{ids[1]}) {
		t.Errorf("--sequential: %v", p.DependsOn)
	}
	if a := taskIn(t, repo.Plan, "task-a"); !reflect.DeepEqual(a.DependsOn, []string{ids[2]}) {
		t.Errorf("--sequential parent: %v", a.DependsOn)
	}
	if _, _, err := application.SplitTaskDoc(repo.Plan, "task-a", []string{"only one"}, false); err == nil {
		t.Error("a split needs two parts")
	}
}

func TestMoveTaskDoc(t *testing.T) {
	repo := editsRepo(t)
	capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{ID: "other", Title: str("Other")}}})
	doc, err := application.MoveTaskDoc(repo.Plan, "task-a", "", "other")
	if err != nil {
		t.Fatal(err)
	}
	capture(t, repo, doc)
	if a := taskIn(t, repo.Plan, "task-a"); a.FeatureID != "other" {
		t.Errorf("moved = %+v", a)
	}
	if _, err := application.MoveTaskDoc(repo.Plan, "task-a", "", ""); err == nil {
		t.Error("a move needs a destination")
	}
}
