package application_test

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func TestDecideRecordsAndSupersedes(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	repo.Plan.ApprovalStatus = planning.ApprovalApproved

	doc, id, err := application.DecideDoc(repo.Spec, application.Decide{Title: "Numbering", Choice: "Per-year sequence", Requirements: []string{"seq"}})
	if err != nil || id != "decision-numbering" {
		t.Fatalf("DecideDoc: %s %v", id, err)
	}
	res := capture(t, repo, doc)
	if strings.Join(res.Created, ",") != "decision:decision-numbering" || res.PlanApproval != string(planning.ApprovalApproved) {
		t.Fatalf("decide: %+v", res)
	}
	if d := repo.Spec.Decisions[0]; d.Date == "" || d.Requirements[0] != "seq" {
		t.Errorf("recorded %+v", d)
	}

	doc, _, _ = application.DecideDoc(repo.Spec, application.Decide{ID: "decision-numbering-v2", Title: "Numbering", Choice: "Global sequence", Supersedes: "decision-numbering"})
	res = capture(t, repo, doc)
	if !res.Applied || repo.Spec.Decisions[0].Status != spec.DecisionSuperseded || repo.Spec.Decisions[0].SupersededBy != "decision-numbering-v2" {
		t.Fatalf("supersede: %+v %+v", res, repo.Spec.Decisions)
	}

	if _, _, err := application.DecideDoc(repo.Spec, application.Decide{Title: "x"}); err == nil {
		t.Error("a decision needs a choice")
	}
	for name, d := range map[string]application.Decide{
		"unknown req":        {Title: "a", Choice: "b", Requirements: []string{"nope"}},
		"supersedes nothing": {Title: "c", Choice: "d", Supersedes: "nope"},
	} {
		doc, _, _ := application.DecideDoc(repo.Spec, d)
		if res := capture(t, repo, doc); len(res.Rejected) == 0 {
			t.Errorf("%s: expected a rejection", name)
		}
	}
}

// `roady next` shows the decisions behind the active task.
func TestBriefShowsDecisions(t *testing.T) {
	dir := claimsProject(t, "1h", "task-a")
	svc, repo := agent(dir)
	sp, _ := repo.LoadSpec()
	sp.Features[0].Requirements = []spec.Requirement{{ID: "a", Title: "A"}}
	sp.Decisions = []spec.Decision{
		{ID: "d-req", Title: "Storage", Choice: "SQLite, one file", Date: "2026-09-01", Requirements: []string{"a"}},
		{ID: "d-other", Title: "Unrelated", Choice: "x", Requirements: []string{"zzz"}},
		{ID: "d-old", Title: "Storage", Choice: "Postgres", Requirements: []string{"a"}, Status: spec.DecisionSuperseded, SupersededBy: "d-req"},
	}
	_ = repo.SaveSpec(sp)
	if err := svc.StartTask(t.Context(), "task-a", "claude"); err != nil {
		t.Fatal(err)
	}
	brief, err := svc.Brief(t.Context(), "claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(brief.Decisions) != 1 || brief.Decisions[0].ID != "d-req" {
		t.Fatalf("decisions %+v", brief.Decisions)
	}
	if !strings.Contains(brief.Render(), "Decided: Storage — SQLite, one file (d-req)") {
		t.Errorf("render:\n%s", brief.Render())
	}
}
