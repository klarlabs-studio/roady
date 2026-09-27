package planning

import "testing"

func TestBlockKinds(t *testing.T) {
	for in, want := range map[string]BlockKind{"": "", "spec-conflict": BlockSpecConflict, "Cannot_Complete": BlockCannotComplete} {
		if got, err := ParseBlockKind(in); err != nil || got != want {
			t.Errorf("ParseBlockKind(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseBlockKind("bored"); err == nil {
		t.Error("an unknown reason must be refused")
	}
	if BlockKind("").NeedsDecision() || !BlockSpecConflict.NeedsDecision() || !BlockCannotComplete.NeedsDecision() {
		t.Error("only spec-conflict and cannot-complete need a person")
	}

	s := NewExecutionState("p")
	s.SetTaskStatus("a", StatusBlocked)
	r := s.TaskStates["a"]
	r.Block = &Block{Kind: BlockSpecConflict, Detail: "R2 vs R5"}
	s.TaskStates["a"] = r
	s.SetTaskStatus("b", StatusBlocked)
	r = s.TaskStates["b"]
	r.Block = &Block{Detail: "waiting on API keys"}
	s.TaskStates["b"] = r
	if nd := s.NeedsDecision(); len(nd) != 1 || nd["a"].Detail != "R2 vs R5" {
		t.Errorf("NeedsDecision = %+v", nd)
	}
	s.SetTaskStatus("a", StatusPending)
	if s.TaskStates["a"].Block != nil || len(s.NeedsDecision()) != 0 {
		t.Error("unblocking kept the block")
	}
}
