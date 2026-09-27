package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// acceptProject has one task in each state that matters to acceptance.
func acceptProject(t *testing.T) (*application.TaskService, func() *planning.ExecutionState) {
	t.Helper()
	dir := claimsProject(t, "", "done-a", "done-b", "verified", "pending")
	svc, repo := agent(dir)
	st, err := repo.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	st.SetTaskStatus("done-a", planning.StatusDone)
	st.SetTaskStatus("done-b", planning.StatusDone)
	st.SetTaskStatus("verified", planning.StatusVerified)
	st.SetTaskStatus("pending", planning.StatusPending)
	if err := repo.SaveState(st); err != nil {
		t.Fatal(err)
	}
	return svc, func() *planning.ExecutionState {
		s, _ := repo.LoadState()
		return s
	}
}

func TestAcceptFinishedAllDone(t *testing.T) {
	svc, state := acceptProject(t)
	got, err := svc.AcceptFinished(context.Background(), nil, true, "felix", "finished before adopting roady")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "done-a,done-b" {
		t.Errorf("accepted %v, want the two done tasks", got)
	}
	st := state()
	for _, id := range got {
		a := st.TaskStates[id].Accepted
		if a == nil || a.By != "felix" || a.Reason != "finished before adopting roady" {
			t.Errorf("%s: acceptance %+v", id, a)
		}
	}
	for _, id := range []string{"verified", "pending"} {
		if st.TaskStates[id].Accepted != nil {
			t.Errorf("%s was accepted", id)
		}
	}

	hist, err := svc.TaskHistory("done-a")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(application.RenderHistory("done-a", hist), "accepted without verification: finished before adopting roady") {
		t.Errorf("history does not show the acceptance:\n%s", application.RenderHistory("done-a", hist))
	}

	if _, err := svc.AcceptFinished(context.Background(), nil, true, "felix", "again"); err == nil ||
		!strings.Contains(err.Error(), "no finished task is awaiting verification") {
		t.Errorf("a second all-done: %v", err)
	}
}

func TestAcceptFinishedIsAllOrNothing(t *testing.T) {
	svc, state := acceptProject(t)
	_, err := svc.AcceptFinished(context.Background(), []string{"done-a", "pending"}, false, "felix", "history")
	if err == nil || !strings.Contains(err.Error(), "pending is pending") {
		t.Fatalf("accepting a pending task: %v", err)
	}
	if state().TaskStates["done-a"].Accepted != nil {
		t.Error("done-a was accepted although the batch was refused")
	}
}

func TestAcceptFinishedRefusesWithoutAReasonOrATarget(t *testing.T) {
	svc, _ := acceptProject(t)
	for name, tc := range map[string]struct {
		ids     []string
		all     bool
		reason  string
		wantErr string
	}{
		"no reason":    {ids: []string{"done-a"}, reason: " ", wantErr: "needs a reason"},
		"no target":    {reason: "r", wantErr: "not both"},
		"both targets": {ids: []string{"done-a"}, all: true, reason: "r", wantErr: "not both"},
	} {
		if _, err := svc.AcceptFinished(context.Background(), tc.ids, tc.all, "felix", tc.reason); err == nil ||
			!strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: %v, want %q", name, err, tc.wantErr)
		}
	}
}
