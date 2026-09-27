package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// mcp-go's spec had no id or title: status printed "Project:  (v0.1.0)"
// while doctor said everything looked good.
func TestDoctorFailsASpecThatValidateRejects(t *testing.T) {
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	if _, err := runRoady(t, "", "init", "doc"); err != nil {
		t.Fatal(err)
	}
	repo := storage.NewFilesystemRepository(dir)
	sp, _ := repo.LoadSpec()
	sp.ID, sp.Title = "", ""
	_ = repo.SaveSpec(sp)

	out, err := runRoady(t, "", "doctor")
	if err == nil || !strings.Contains(out, "Checking Spec File... FAIL") || !strings.Contains(out, "spec ID is required") {
		t.Errorf("doctor on a spec without id or title: %v\n%s", err, out)
	}
}

// A stored plan whose tasks name features by title: doctor says so, and
// doctor --fix repairs the links without touching the approval.
func TestDoctorFixRepairsTitleLinks(t *testing.T) {
	dir, cleanup := withPlainTempDir(t)
	defer cleanup()
	if _, err := runRoady(t, "", "init", "doc"); err != nil {
		t.Fatal(err)
	}
	doc := `features:
  - id: roadmap-2026
    title: 2026 Roadmap Alignment
`
	if out, err := runRoady(t, doc, "capture"); err != nil {
		t.Fatalf("capture: %v\n%s", err, out)
	}
	repo := storage.NewFilesystemRepository(dir)
	plan, _ := repo.LoadPlan()
	plan.Tasks = append(plan.Tasks, planning.Task{ID: "old", Title: "Old work", FeatureID: "2026 Roadmap Alignment"})
	plan.ApprovalStatus = planning.ApprovalApproved
	_ = repo.SavePlan(plan)

	out, _ := runRoady(t, "", "doctor")
	if !strings.Contains(out, "1 task(s) name their feature by its title") || !strings.Contains(out, "roady doctor --fix") {
		t.Errorf("doctor does not name the title link:\n%s", out)
	}
	out, _ = runRoady(t, "", "doctor", "--fix")
	if !strings.Contains(out, "Pointed 1 task(s) at their feature ids") {
		t.Errorf("doctor --fix:\n%s", out)
	}
	plan, _ = repo.LoadPlan()
	for _, task := range plan.Tasks {
		if task.ID == "old" && task.FeatureID != "roadmap-2026" {
			t.Errorf("old links %q after --fix", task.FeatureID)
		}
	}
	if plan.ApprovalStatus != planning.ApprovalApproved {
		t.Errorf("approval %s after --fix", plan.ApprovalStatus)
	}
	if out, _ = runRoady(t, "", "doctor"); strings.Contains(out, "name their feature by its title") {
		t.Errorf("doctor still reports title links after --fix:\n%s", out)
	}
}
