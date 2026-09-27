package application_test

import (
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
