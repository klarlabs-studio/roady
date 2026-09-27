package application_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// A goal is captured like anything else and features link to it; neither
// is a change of intent, so an approved plan stays approved.
func TestCaptureGoalsKeepsTheApproval(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	repo.Plan.ApprovalStatus = planning.ApprovalApproved

	res := capture(t, repo, application.CaptureDoc{
		Goals:    []application.CaptureGoal{{ID: "goal-billing", Title: str("Billing"), Horizon: str("Next"), Milestone: str("v1")}},
		Features: []application.CaptureFeature{{ID: "inv", Goal: str("goal-billing")}},
	})
	if !res.Applied || len(res.Rejected) > 0 {
		t.Fatalf("expected an applied capture, got %+v", res)
	}
	if strings.Join(res.Created, ",") != "goal:goal-billing" || strings.Join(res.Updated, ",") != "link:feature:inv" {
		t.Errorf("created %v, updated %v", res.Created, res.Updated)
	}
	if res.PlanApproval != string(planning.ApprovalApproved) {
		t.Errorf("a goal must not reopen the approval, got %s (%s)", res.PlanApproval, res.ApprovalReason)
	}
	g := repo.Spec.Goals[0]
	if g.Horizon != spec.HorizonNext || g.Milestone != "v1" || repo.Spec.Features[1].Goal != "goal-billing" {
		t.Errorf("goal %+v, feature goal %q", g, repo.Spec.Features[1].Goal)
	}

	// Moving it and changing its status is an update; sending it again is not.
	res = capture(t, repo, application.CaptureDoc{Goals: []application.CaptureGoal{{ID: "goal-billing", Horizon: str("now"), Status: str("planned")}}})
	if strings.Join(res.Updated, ",") != "goal:goal-billing" {
		t.Errorf("updated %v", res.Updated)
	}
	res = capture(t, repo, application.CaptureDoc{Goals: []application.CaptureGoal{{ID: "goal-billing", Horizon: str("now")}}})
	if res.Changed() {
		t.Errorf("an unchanged goal reported a change: %+v", res)
	}
}

func TestCaptureGoalsRejectsBadInput(t *testing.T) {
	cases := map[string]application.CaptureDoc{
		"no title":      {Goals: []application.CaptureGoal{{ID: "g"}}},
		"no id":         {Goals: []application.CaptureGoal{{Title: str("G")}}},
		"bad horizon":   {Goals: []application.CaptureGoal{{ID: "g", Title: str("G"), Horizon: str("soon")}}},
		"bad status":    {Goals: []application.CaptureGoal{{ID: "g", Title: str("G"), Status: str("maybe")}}},
		"unknown goal":  {Features: []application.CaptureFeature{{ID: "base", Goal: str("goal-missing")}}},
		"unknown goal2": {Features: []application.CaptureFeature{{ID: "f", Title: str("F"), Requirements: []application.CaptureRequirement{{ID: "r", Title: str("R"), Goal: str("nope")}}}}},
	}
	for name, doc := range cases {
		repo := captureRepo()
		res := capture(t, repo, doc)
		if len(res.Rejected) == 0 || res.Applied {
			t.Errorf("%s: expected a rejection, got %+v", name, res)
		}
	}
}

func TestRoadmapGroupsGoalsAndCountsWork(t *testing.T) {
	sp := &spec.ProductSpec{ID: "s", Title: "S",
		Goals: []spec.Goal{
			{ID: "g-now", Title: "Now thing", Horizon: spec.HorizonNow},
			{ID: "g-idea", Title: "Maybe", Status: spec.GoalIdea},
			{ID: "g-later", Title: "Later idea", Horizon: spec.HorizonLater, Status: spec.GoalIdea},
			{ID: "g-shipped", Title: "Done thing", Status: spec.GoalShipped, Milestone: "v0.1"},
			{ID: "g-no", Title: "Never", Status: spec.GoalOutOfScope},
		},
		Features: []spec.Feature{
			{ID: "f1", Title: "F1", Goal: "g-now", Requirements: []spec.Requirement{{ID: "r1", Title: "R1"}, {ID: "r2", Title: "R2", Goal: "g-later"}}},
			{ID: "f2", Title: "F2"},
		},
	}
	plan := &planning.Plan{Tasks: []planning.Task{
		{ID: "task-r1", FeatureID: "f1"}, {ID: "task-extra", FeatureID: "f1"}, {ID: "task-r2", FeatureID: "f1"}, {ID: "task-f2", FeatureID: "f2"},
	}}
	state := planning.NewExecutionState("p")
	state.TaskStates["task-r1"] = planning.TaskResult{Status: planning.StatusVerified}

	rm := application.BuildRoadmap(sp, plan, state)
	var names []string
	for _, s := range rm.Sections {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Now,Later,Ideas,Shipped,Out of scope" {
		t.Fatalf("sections %v", names)
	}
	now := rm.Sections[0].Goals[0]
	if now.Tasks != 2 || now.Done != 1 || strings.Join(now.Features, ",") != "f1" {
		t.Errorf("now goal %+v", now)
	}
	if later := rm.Sections[1].Goals[0]; later.Tasks != 1 {
		t.Errorf("a requirement's own goal link should take its task: %+v", later)
	}
	if strings.Join(rm.Unlinked, ",") != "f2" {
		t.Errorf("unlinked %v", rm.Unlinked)
	}

	var b bytes.Buffer
	rm.Render(&b)
	for _, want := range []string{"Now\n  g-now  Now thing  (1/2 tasks done)", "g-later  Later idea  (idea, 0/1", "g-shipped  Done thing  (v0.1)", "Features serving no goal: f2"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("render lacks %q:\n%s", want, b.String())
		}
	}

	b.Reset()
	application.BuildRoadmap(&spec.ProductSpec{}, nil, nil).Render(&b)
	if !strings.Contains(b.String(), "No goals yet") {
		t.Errorf("empty roadmap: %s", b.String())
	}
}

func TestGoalDocs(t *testing.T) {
	sp := &spec.ProductSpec{Goals: []spec.Goal{{ID: "goal-a", Title: "A"}}, Features: []spec.Feature{{ID: "f"}}}
	doc, id, err := application.AddGoalDoc(sp, application.GoalEdit{Title: str("Offline Mode!"), Features: []string{"f"}})
	if err != nil || id != "goal-offline-mode" || *doc.Features[0].Goal != id {
		t.Fatalf("add: %v %s %+v", err, id, doc)
	}
	if _, _, err := application.AddGoalDoc(sp, application.GoalEdit{}); err == nil {
		t.Error("a goal needs a title")
	}
	if _, _, err := application.EditGoalDoc(sp, application.GoalEdit{ID: "goal-missing", Horizon: str("now")}); err == nil {
		t.Error("editing a missing goal should fail")
	}
	if _, _, err := application.EditGoalDoc(sp, application.GoalEdit{ID: "goal-a", Features: []string{"nope"}}); err == nil {
		t.Error("linking a missing feature should fail")
	}
	if _, _, err := application.EditGoalDoc(sp, application.GoalEdit{}); err == nil {
		t.Error("edit needs an id")
	}
}

func TestShippedGoalsNewestFirst(t *testing.T) {
	sp := &spec.ProductSpec{Goals: []spec.Goal{
		{ID: "a", Title: "a", Status: spec.GoalShipped, Milestone: "v0.5.0"},
		{ID: "b", Title: "b", Status: spec.GoalShipped, Milestone: "v0.22.x"},
		{ID: "c", Title: "c", Status: spec.GoalShipped},
		{ID: "d", Title: "d", Status: spec.GoalShipped, Milestone: "v0.10.x"},
		{ID: "e", Title: "e", Status: spec.GoalShipped, Milestone: "v1.0.0"},
	}}
	var ids []string
	for _, g := range application.BuildRoadmap(sp, nil, nil).Sections[0].Goals {
		ids = append(ids, g.ID)
	}
	if strings.Join(ids, ",") != "e,b,d,a,c" {
		t.Errorf("shipped order %v", ids)
	}
}
