package application

import (
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// Single-task edits: `roady add`, `edit`, `split` and `move` as thin
// builders over capture. Each turns one intention into a CaptureDoc, so it is
// validated the same way, applied all or nothing, and recorded as one
// plan.capture event — the commands add ergonomics, not a second write path.

// AddTask describes a task to add.
type AddTask struct {
	Title       string
	ID          string // derived from the title when empty
	Description string
	Requirement string
	Feature     string
	// After are tasks the new one depends on. With neither Requirement nor
	// Feature, the new task joins the feature of the first of them.
	After []string
	// Before are tasks that should wait for the new one: it is added to
	// their dependencies.
	Before   []string
	Priority string
	Estimate string
	Check    *planning.Check
	// Goal attaches the task to a roadmap goal. With no requirement,
	// feature or --after to place it, the task is unplanned work.
	Goal string
}

// AddTaskDoc builds the capture for a new task and returns its id.
//
// Adding the same title again is a no-op, not a duplicate: the derived id is
// reused when the task it names already has that title.
func AddTaskDoc(plan *planning.Plan, a AddTask) (CaptureDoc, string, error) {
	plan = orEmptyPlan(plan)
	title := strings.TrimSpace(a.Title)
	if title == "" {
		return CaptureDoc{}, "", fmt.Errorf("a task needs a title")
	}
	id := strings.TrimSpace(a.ID)
	if id == "" {
		id = derivedTaskID(plan, title)
	}

	feature, requirement := a.Feature, a.Requirement
	if feature == "" && requirement == "" {
		for _, dep := range a.After {
			if t, ok := findTask(plan, dep); ok && t.FeatureID != "" {
				feature = t.FeatureID
				break
			}
		}
	}

	ct := CaptureTask{ID: id, Title: &title}
	if a.Goal != "" {
		ct.Goal = strPtr(a.Goal)
	}
	if requirement != "" {
		ct.Requirement = &requirement
	}
	if feature != "" {
		ct.FeatureID = &feature
	}
	if a.Description != "" {
		ct.Description = strPtr(a.Description)
	}
	if a.Priority != "" {
		ct.Priority = strPtr(a.Priority)
	}
	if a.Estimate != "" {
		ct.Estimate = strPtr(a.Estimate)
	}
	if len(a.After) > 0 {
		deps := uniqueStrings(a.After)
		ct.DependsOn = &deps
	}
	if a.Check != nil {
		c := *a.Check
		ct.Check = &c
	}
	doc := CaptureDoc{Tasks: []CaptureTask{ct}}

	for _, waiter := range uniqueStrings(a.Before) {
		t, ok := findTask(plan, waiter)
		if !ok {
			return CaptureDoc{}, "", fmt.Errorf("--before %s: no such task", waiter)
		}
		deps := uniqueStrings(append(append([]string{}, t.DependsOn...), id))
		doc.Tasks = append(doc.Tasks, CaptureTask{ID: waiter, DependsOn: &deps})
	}
	return doc, id, nil
}

// derivedTaskID is task-<slug of title>, suffixed when another task already
// holds it under a different title.
func derivedTaskID(plan *planning.Plan, title string) string {
	slug := shortSlug(title)
	if slug == "" {
		slug = "task"
	}
	base := "task-" + slug
	id := base
	for n := 2; ; n++ {
		t, ok := findTask(plan, id)
		if !ok || strings.EqualFold(strings.TrimSpace(t.Title), title) {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// EditTask names the fields to change; nil leaves a field as it is.
type EditTask struct {
	Title       *string
	Description *string
	Priority    *string
	Estimate    *string
	DependsOn   *[]string // replaces the list
	AddDeps     []string
	DropDeps    []string
	Check       *planning.Check
}

// EditTaskDoc builds the capture for editing one task.
func EditTaskDoc(plan *planning.Plan, id string, e EditTask) (CaptureDoc, error) {
	plan = orEmptyPlan(plan)
	t, ok := findTask(plan, id)
	if !ok {
		return CaptureDoc{}, fmt.Errorf("no task %q", id)
	}
	ct := CaptureTask{ID: id, Title: e.Title, Description: e.Description, Priority: e.Priority, Estimate: e.Estimate, Check: e.Check}
	if e.DependsOn != nil || len(e.AddDeps) > 0 || len(e.DropDeps) > 0 {
		deps := append([]string{}, t.DependsOn...)
		if e.DependsOn != nil {
			deps = append([]string{}, (*e.DependsOn)...)
		}
		deps = append(deps, e.AddDeps...)
		drop := map[string]bool{}
		for _, d := range e.DropDeps {
			drop[d] = true
		}
		kept := make([]string, 0, len(deps))
		for _, d := range uniqueStrings(deps) {
			if !drop[d] {
				kept = append(kept, d)
			}
		}
		ct.DependsOn = &kept
	}
	if ct == (CaptureTask{ID: id}) {
		return CaptureDoc{}, fmt.Errorf("nothing to change: give at least one field to edit")
	}
	return CaptureDoc{Tasks: []CaptureTask{ct}}, nil
}

// SplitTaskDoc breaks a task into parts. The parts take over what the task
// waited for, and the task now waits for its parts — so it stays in place
// for everything that depends on it, and keeps its acceptance check as the
// proof that the parts add up. Sequential chains the parts in order.
func SplitTaskDoc(plan *planning.Plan, id string, parts []string, sequential bool) (CaptureDoc, []string, error) {
	plan = orEmptyPlan(plan)
	t, ok := findTask(plan, id)
	if !ok {
		return CaptureDoc{}, nil, fmt.Errorf("no task %q", id)
	}
	var titles []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			titles = append(titles, p)
		}
	}
	if len(titles) < 2 {
		return CaptureDoc{}, nil, fmt.Errorf("split %s into at least two parts", id)
	}
	base := id
	if !strings.HasPrefix(base, "task-") {
		base = "task-" + base
	}
	var doc CaptureDoc
	ids := make([]string, 0, len(titles))
	used := map[string]bool{id: true}
	for i, title := range titles {
		pid := base + "-" + shortSlug(title)
		for n := 2; used[pid]; n++ {
			pid = fmt.Sprintf("%s-%s-%d", base, shortSlug(title), n)
		}
		used[pid] = true
		deps := append([]string{}, t.DependsOn...)
		if sequential && i > 0 {
			deps = []string{ids[i-1]}
		}
		feature := t.FeatureID
		title := title
		ct := CaptureTask{ID: pid, Title: &title, FeatureID: &feature, DependsOn: &deps}
		if t.Priority != "" {
			pr := string(t.Priority)
			ct.Priority = &pr
		}
		doc.Tasks = append(doc.Tasks, ct)
		ids = append(ids, pid)
	}
	parentDeps := append([]string{}, ids...)
	if sequential {
		parentDeps = []string{ids[len(ids)-1]}
	}
	doc.Tasks = append(doc.Tasks, CaptureTask{ID: id, DependsOn: &parentDeps})
	return doc, ids, nil
}

// MoveTaskDoc re-homes a task under another requirement or feature.
func MoveTaskDoc(plan *planning.Plan, id, requirement, feature string) (CaptureDoc, error) {
	plan = orEmptyPlan(plan)
	if _, ok := findTask(plan, id); !ok {
		return CaptureDoc{}, fmt.Errorf("no task %q", id)
	}
	if requirement == "" && feature == "" {
		return CaptureDoc{}, fmt.Errorf("move %s where? --req <requirement> or --feature <feature>", id)
	}
	ct := CaptureTask{ID: id}
	if requirement != "" {
		ct.Requirement = &requirement
	}
	if feature != "" {
		ct.FeatureID = &feature
	}
	return CaptureDoc{Tasks: []CaptureTask{ct}}, nil
}

func strPtr(s string) *string { return &s }

func orEmptyPlan(p *planning.Plan) *planning.Plan {
	if p == nil {
		return &planning.Plan{}
	}
	return p
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
