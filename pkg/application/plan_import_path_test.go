package application

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

const onePlan = "# Release prep\n\n## Tasks\n\n- [ ] Write the changelog\n"

// spec.yaml and plan.json are committed. A plan imported from outside the
// project wrote its absolute path into both -- a home directory, a session's
// temp directory -- which names the user's machine and points at nothing for
// anyone else (#139).
func TestPlanImport_CitesAPlanOutsideTheProjectWithoutTheMachine(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()

	for _, c := range []struct {
		name, path, want string
	}{
		{"inside the project", writePlan(t, root, "docs/plan.md", onePlan), "docs/plan.md"},
		{"under the home directory", writePlan(t, home, ".claude/plans/plan.md", onePlan), "~/.claude/plans/plan.md"},
		{"anywhere else", writePlan(t, t.TempDir(), "session/plan.md", onePlan), "plan.md"},
	} {
		imp, err := ImportPlanFile(c.path, nil, PlanImportOptions{Root: root})
		if err != nil {
			t.Fatal(err)
		}
		if got := *imp.Doc.Features[0].Description; got != "Imported from "+c.want+" (markdown)." {
			t.Errorf("%s: description %q, want it to cite %q", c.name, got, c.want)
		}
		if got := imp.Doc.Tasks[0].Source.Doc; got != c.want {
			t.Errorf("%s: task source %q, want %q", c.name, got, c.want)
		}
		for _, leak := range []string{home, filepath.Dir(c.path)} {
			if c.name != "inside the project" && strings.Contains(*imp.Doc.Features[0].Description, leak) {
				t.Errorf("%s: description carries %q", c.name, leak)
			}
		}
	}
}

// Re-importing a plan whose feature already exists -- the same plan from a
// different path, or after editing it -- must not rewrite the feature's
// description. "Imported from ..." is provenance, not intent; rewriting it
// marked the feature's intent changed and reset plan approval for a path-only
// change (#139).
func TestPlanImport_ReimportKeepsTheFeatureDescription(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, "plan.md", onePlan)
	current := &spec.ProductSpec{Features: []spec.Feature{{
		ID: "release-prep", Title: "Release prep", Description: "Imported from elsewhere/plan.md (markdown).",
	}}}
	imp, err := ImportPlanFile(path, current, PlanImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(imp.Doc.Features) != 0 {
		t.Errorf("re-import rewrote the existing feature: %+v", imp.Doc.Features)
	}
	if got := *imp.Doc.Tasks[0].FeatureID; got != "release-prep" {
		t.Errorf("tasks attach to %q, want the existing feature", got)
	}
}
