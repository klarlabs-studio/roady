package application

import (
	"testing"
)

func taskTitles(imp *PlanImport) []string {
	var got []string
	for _, ct := range imp.Doc.Tasks {
		got = append(got, *ct.Title)
	}
	return got
}

// A plan that groups its checklist under sub-headings of a steps section, and
// also has a bulleted list elsewhere, imported the bulleted list and dropped
// every checklist item: the section was the nearest heading only, so "### Docs"
// under "## Tasks" was not a steps section, while "## Milestones" was (#138).
func TestPlanImport_ChecklistUnderSubHeadings(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, "plan.md", `# Release prep

## Rules

- **Rule one.** Text.

## Milestones

- **M1 — first milestone.** Some description.

## Tasks

### Docs

- [ ] Write the changelog

### Ship

- [ ] Tag the release
- [ ] Publish the notes

### Notes

- [ ] Not a step: a narrative sub-heading stays narrative

## Ideas

- [ ] Not a step either: a checklist outside the steps section loses to one inside it
`)
	imp, err := ImportPlanFile(path, nil, PlanImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := taskTitles(imp)
	if len(got) != 3 {
		t.Fatalf("got tasks %q, want the three checklist items under Tasks", got)
	}
	if imp.SkippedBullets != 2 {
		t.Errorf("skipped bullets = %d, want the rule and the milestone reported", imp.SkippedBullets)
	}
	for _, title := range []string{"Write the changelog", "Tag the release", "Publish the notes"} {
		taskByTitle(t, imp, title)
	}
}

// When a plan marks its steps as checklist items, those are the steps: plain
// bullets beside them (a milestones list, a list of rules) are commentary. The
// flat form of the same document imported both lists.
func TestPlanImport_ChecklistWinsOverBullets(t *testing.T) {
	dir := t.TempDir()
	path := writePlan(t, dir, "plan.md", `# Release prep

## Milestones

- **M1 — first milestone.** Some description.
- **M2 — second milestone.** Some description.

## Tasks

- [ ] Write the changelog
- [ ] Tag the release
`)
	imp, err := ImportPlanFile(path, nil, PlanImportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := taskTitles(imp); len(got) != 2 {
		t.Fatalf("got tasks %q, want only the two checklist items", got)
	}
}
