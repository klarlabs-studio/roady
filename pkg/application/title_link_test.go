package application_test

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// A plan stored with titles in feature_id is repaired in place: ids replace
// titles, a link that resolves to nothing is left alone, the approval stays
// and the repair is in the audit log.
func TestRepairTitleLinksKeepsApproval(t *testing.T) {
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	sp := &spec.ProductSpec{ID: "s", Title: "S", Features: []spec.Feature{
		{ID: "roadmap-2026", Title: "2026 Roadmap Alignment"},
	}}
	_ = repo.SaveSpec(sp)
	_ = repo.SavePlan(&planning.Plan{ID: "p", SpecID: "s", ApprovalStatus: planning.ApprovalApproved, Tasks: []planning.Task{
		{ID: "a", Title: "A", FeatureID: "2026 Roadmap Alignment"},
		{ID: "b", Title: "B", FeatureID: "roadmap-2026"},
		{ID: "c", Title: "C", FeatureID: "Gone"},
	}})
	svc := application.NewPlanService(repo, application.NewAuditService(repo))
	before, _ := repo.LoadPlan()

	fixes, err := svc.RepairFeatureLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(fixes) != 1 || fixes["a"] != "roadmap-2026" {
		t.Errorf("fixes %v, want a → roadmap-2026", fixes)
	}
	plan, _ := repo.LoadPlan()
	want := map[string]string{"a": "roadmap-2026", "b": "roadmap-2026", "c": "Gone"}
	for _, task := range plan.Tasks {
		if task.FeatureID != want[task.ID] {
			t.Errorf("%s links %q, want %q", task.ID, task.FeatureID, want[task.ID])
		}
	}
	if plan.ApprovalStatus != planning.ApprovalApproved {
		t.Errorf("approval %s after a link repair", plan.ApprovalStatus)
	}
	if !plan.UpdatedAt.Equal(before.UpdatedAt) {
		t.Errorf("updated_at moved from %s to %s: a repair would hide the plan's staleness", before.UpdatedAt, plan.UpdatedAt)
	}
	events, _ := repo.LoadEvents()
	if len(events) == 0 || events[len(events)-1].Action != "plan.links_repaired" {
		t.Error("the repair is not in the audit log")
	}
	if again, _ := svc.RepairFeatureLinks(); len(again) != 0 {
		t.Errorf("a second repair changed %v", again)
	}
}
