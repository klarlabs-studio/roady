package spec

import (
	"strings"
	"testing"
)

func TestDecisionsFor(t *testing.T) {
	sp := &ProductSpec{Decisions: []Decision{
		{ID: "global", Title: "G", Choice: "c", Date: "2026-01-01"},
		{ID: "req", Title: "R", Choice: "c", Date: "2026-03-01", Requirements: []string{"r1"}},
		{ID: "feat", Title: "F", Choice: "c", Date: "2026-02-01", Features: []string{"f1"}},
		{ID: "goal", Title: "Go", Choice: "c", Date: "2026-04-01", Goals: []string{"g1"}},
		{ID: "old", Title: "O", Choice: "c", Date: "2026-05-01", Requirements: []string{"r1"}, Status: DecisionSuperseded},
		{ID: "other", Title: "X", Choice: "c", Requirements: []string{"r2"}},
	}}
	var ids []string
	for _, d := range sp.DecisionsFor("f1", "r1", "g1") {
		ids = append(ids, d.ID)
	}
	if strings.Join(ids, ",") != "goal,req,feat,global" {
		t.Errorf("DecisionsFor = %v: standing, applicable, newest first", ids)
	}
}

func TestValidateDecisions(t *testing.T) {
	sp := &ProductSpec{ID: "s", Title: "S",
		Features: []Feature{{ID: "f", Requirements: []Requirement{{ID: "r", Title: "R"}}}},
		Decisions: []Decision{{ID: "a", Title: "A", Choice: "c"}, {ID: "a", Title: "A", Choice: "c"}, {ID: "b"},
			{ID: "c", Title: "C", Choice: "x", Status: "maybe", Goals: []string{"nog"}, Features: []string{"nof"}, Requirements: []string{"nor"}, SupersededBy: "zzz"}},
	}
	var all []string
	for _, e := range sp.Validate() {
		all = append(all, e.Error())
	}
	joined := strings.Join(all, "\n")
	for _, want := range []string{"duplicate decision ID: a", "decision 'b' needs a title and a choice", "unknown status", "unknown goal 'nog'",
		"unknown feature 'nof'", "unknown requirement 'nor'", "superseded by unknown decision 'zzz'"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
}
