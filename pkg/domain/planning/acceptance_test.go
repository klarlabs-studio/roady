package planning

import (
	"strings"
	"testing"
	"time"
)

func TestAcceptOnlyADoneTask(t *testing.T) {
	s := NewExecutionState("p")
	s.SetTaskStatus("done", StatusDone)
	s.SetTaskStatus("verified", StatusVerified)
	s.SetTaskStatus("open", StatusInProgress)
	a := Acceptance{By: "felix", Reason: "finished before roady", At: time.Now()}

	if err := s.Accept("done", a); err != nil {
		t.Fatalf("accepting a done task: %v", err)
	}
	if !s.TaskStates["done"].IsAccepted() {
		t.Error("the done task is not accepted")
	}
	for _, id := range []string{"verified", "open", "missing"} {
		err := s.Accept(id, a)
		if err == nil || !strings.Contains(err.Error(), "only a done task") {
			t.Errorf("accepting %s: %v, want a refusal", id, err)
		}
		if s.TaskStates[id].Accepted != nil {
			t.Errorf("%s carries an acceptance after a refusal", id)
		}
	}
}

func TestAcceptanceEndsWhenTheTaskMovesOn(t *testing.T) {
	for _, next := range []TaskStatus{StatusVerified, StatusPending, StatusInProgress} {
		s := NewExecutionState("p")
		s.SetTaskStatus("t", StatusDone)
		_ = s.Accept("t", Acceptance{By: "felix", Reason: "history"})
		s.SetTaskStatus("t", next)
		if s.TaskStates["t"].Accepted != nil {
			t.Errorf("moving to %s kept the acceptance", next)
		}
	}
}
