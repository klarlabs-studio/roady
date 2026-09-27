package application_test

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// A project that renamed its features after finishing work under the old
// names (nexa: 36 such tasks) must not be told to prune its history. Finished
// orphans are info and survive prune; an open orphan is still a problem.
func TestFinishedOrphansAreHistory(t *testing.T) {
	dir := claimsProject(t, "off", "task-a")
	repo := storage.NewFilesystemRepository(dir)
	plan, _ := repo.LoadPlan()
	plan.Tasks = append(plan.Tasks,
		planning.Task{ID: "old-done", Title: "Shipped", FeatureID: "renamed-away"},
		planning.Task{ID: "old-verified", Title: "Proven", FeatureID: "renamed-away"},
		planning.Task{ID: "old-open", Title: "Never started", FeatureID: "renamed-away"},
	)
	_ = repo.SavePlan(plan)
	st, _ := repo.LoadState()
	st.TaskStates["old-done"] = planning.TaskResult{Status: planning.StatusDone}
	st.TaskStates["old-verified"] = planning.TaskResult{Status: planning.StatusVerified}
	_ = repo.SaveState(st)

	audit := application.NewAuditService(repo)
	report, err := application.NewDriftService(repo, audit, nil, application.NewPolicyService(repo)).DetectDrift(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sev := map[string]drift.Severity{}
	for _, is := range report.Issues {
		if is.Category == drift.CategoryOrphan {
			sev[is.ComponentID] = is.Severity
		}
	}
	if sev["old-done"] != drift.SeverityInfo || sev["old-verified"] != drift.SeverityInfo {
		t.Errorf("finished orphans should be info: %v", sev)
	}
	if sev["old-open"] != drift.SeverityMedium {
		t.Errorf("an open orphan is still medium: %v", sev)
	}

	if err := application.NewPlanService(repo, audit).PrunePlan(); err != nil {
		t.Fatal(err)
	}
	plan, _ = repo.LoadPlan()
	kept := map[string]bool{}
	for _, tk := range plan.Tasks {
		kept[tk.ID] = true
	}
	if !kept["old-done"] || !kept["old-verified"] || kept["old-open"] || !kept["task-a"] {
		t.Errorf("prune kept %v", kept)
	}
}

func TestStatusWordTitleIsRefused(t *testing.T) {
	repo := captureRepo()
	capture(t, repo, wholePlan())
	for _, title := range []string{"done", "Done.", " in progress ", "verified", "✓ done"} {
		res := capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-x", Title: str(title), FeatureID: str("base")}}})
		if res.Applied || len(res.Rejected) != 1 || !strings.Contains(res.Rejected[0].Reason, "roady task complete task-x") {
			t.Errorf("%q: %+v", title, res)
		}
	}
	res := capture(t, repo, application.CaptureDoc{Features: []application.CaptureFeature{{ID: "base", Requirements: []application.CaptureRequirement{{ID: "rq", Title: str("complete")}}}}})
	if res.Applied || len(res.Rejected) != 1 {
		t.Errorf("a requirement titled with a status: %+v", res)
	}
	// Words that merely contain a status are fine.
	res = capture(t, repo, application.CaptureDoc{Tasks: []application.CaptureTask{{ID: "task-y", Title: str("Done screen for onboarding"), FeatureID: str("base")}}})
	if !res.Applied {
		t.Errorf("a real title was refused: %+v", res)
	}

	plan := &planning.Plan{Tasks: []planning.Task{{ID: "a", Title: "done"}, {ID: "b", Title: "Real"}, {ID: "c", Title: "Blocked"}}}
	if got := strings.Join(application.StatusTitledTasks(plan), ","); got != "a,c" {
		t.Errorf("status-titled %s", got)
	}
}
