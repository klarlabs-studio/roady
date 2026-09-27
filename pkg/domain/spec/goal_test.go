package spec

import (
	"strings"
	"testing"
)

func TestParseHorizonAndStatus(t *testing.T) {
	if h, err := ParseHorizon(" Next "); err != nil || h != HorizonNext {
		t.Errorf("ParseHorizon = %q, %v", h, err)
	}
	if h, err := ParseHorizon(""); err != nil || h != "" {
		t.Errorf("empty horizon = %q, %v", h, err)
	}
	if _, err := ParseHorizon("soon"); err == nil {
		t.Error("soon is not a horizon")
	}
	if s, err := ParseGoalStatus("Out-of-Scope"); err != nil || s != GoalOutOfScope {
		t.Errorf("ParseGoalStatus = %q, %v", s, err)
	}
	if _, err := ParseGoalStatus("maybe"); err == nil {
		t.Error("maybe is not a status")
	}
}

func TestGoalStatusDefaults(t *testing.T) {
	g := Goal{ID: "g"}
	if g.EffectiveStatus() != GoalPlanned || !g.Open() {
		t.Errorf("an unset status reads as planned and open")
	}
	if (Goal{Status: GoalShipped}).Open() || (Goal{Status: GoalOutOfScope}).Open() || !(Goal{Status: GoalIdea}).Open() {
		t.Error("only ideas and planned goals are open")
	}
}

func TestValidateGoals(t *testing.T) {
	sp := &ProductSpec{ID: "s", Title: "S",
		Goals: []Goal{{ID: "a", Title: "A"}, {ID: "a", Title: "Dup"}, {ID: "", Title: "No id"}, {ID: "b"},
			{ID: "c", Title: "C", Horizon: "Now"}, {ID: "d", Title: "D", Status: "done"}},
		Features: []Feature{{ID: "f", Goal: "missing", Requirements: []Requirement{{ID: "r", Title: "R", Goal: "gone"}}}},
	}
	var msgs []string
	for _, err := range sp.Validate() {
		msgs = append(msgs, err.Error())
	}
	all := strings.Join(msgs, "\n")
	for _, want := range []string{"duplicate goal ID: a", "goal at index 2 missing ID", "goal 'b' missing title",
		"goal 'c': unknown horizon", "goal 'd': unknown status", "feature 'f' links to unknown goal 'missing'",
		"requirement 'r' links to unknown goal 'gone'"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if sp.GoalIndex("b") != 3 || sp.GoalIndex("zzz") != -1 {
		t.Error("GoalIndex")
	}
}

// Goals order work; they are not part of the intent a plan was approved
// for, so they leave the spec hash alone.
func TestGoalsDoNotChangeTheHash(t *testing.T) {
	sp := &ProductSpec{ID: "s", Features: []Feature{{ID: "f"}}}
	before := sp.Hash()
	sp.Goals = []Goal{{ID: "g", Title: "G", Horizon: HorizonNow}}
	sp.Features[0].Goal = "g"
	if sp.Hash() != before {
		t.Error("adding goals changed the spec hash")
	}
}
