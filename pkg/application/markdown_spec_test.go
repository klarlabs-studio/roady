package application

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

func parseDoc(t *testing.T, md string) *spec.ProductSpec {
	t.Helper()
	return parseMarkdownSpec("docs/prd.md", strings.Split(md, "\n"))
}

func reqTitles(f spec.Feature) []string {
	var out []string
	for _, r := range f.Requirements {
		out = append(out, r.Title)
	}
	return out
}

// The PRD that the old importer turned into "Implement Overview" and
// "Implement Features".
const invoicePRD = `# Invoice Tool PRD

## Overview
A small CLI that generates PDF invoices for freelancers.

## Features

### Client management
Users can store clients with name, address and VAT ID.
- Must validate EU VAT IDs against format rules
- Clients are stored in a local SQLite database

### Invoice generation
- Generate a PDF invoice from a YAML line-item file
- Invoice numbers must be sequential and gap-free per year
- Support reverse-charge note for EU B2B clients

### Export
- Export all invoices of a year as CSV for the tax advisor`

func TestMarkdownImportFindsFeaturesAndRequirements(t *testing.T) {
	s := parseDoc(t, invoicePRD)
	if s.Title != "Invoice Tool PRD" || s.Description != "A small CLI that generates PDF invoices for freelancers." {
		t.Errorf("title/description: %q / %q", s.Title, s.Description)
	}
	if len(s.Features) != 3 {
		t.Fatalf("expected 3 features, got %d: %+v", len(s.Features), s.Features)
	}
	total := 0
	for _, f := range s.Features {
		total += len(f.Requirements)
		if f.ID == "overview" || f.ID == "features" {
			t.Errorf("%q is a section of the document, not a feature", f.ID)
		}
	}
	if total != 6 {
		t.Fatalf("expected 6 requirements, got %d", total)
	}
	vat := s.Features[0].Requirements[0]
	if vat.Priority != "high" || vat.Source.Doc != "docs/prd.md" || vat.Source.Line != 10 {
		t.Errorf("requirement priority/source: %+v", vat)
	}
	if s.Features[0].Description != "Users can store clients with name, address and VAT ID." {
		t.Errorf("feature description: %q", s.Features[0].Description)
	}
}

func TestMarkdownImportStructure(t *testing.T) {
	s := parseDoc(t, "# Cal\n\n## 1. Overview\nShared calendar.\n\n## 2. Goals\n- Fewer double bookings\n\n## 3. Features\n\n### 3.1 Scheduling\nMove events.\n\n#### Acceptance criteria\n- [ ] Users **must** drag an event\n  - saved within 1 second\n  - conflicts highlighted\n- [x] Recurring events should support weekly rules\n- **Time zones**: shown in local time\n\n### 3.2 Sync\n- Sync runs every 5 minutes; users may trigger it\n- Outlook support is nice to have\n\n```yaml\n- not: a requirement\n```\n\n## 4. Non-functional requirements\n- GDPR: data stays in the EU\n\n## 5. Open questions\n- Outlook?\n")

	if s.Description != "Shared calendar." {
		t.Errorf("overview should become the description, got %q", s.Description)
	}
	if len(s.Features) != 2 || s.Features[0].ID != "scheduling" || s.Features[0].Title != "Scheduling" {
		t.Fatalf("expected numbered headings stripped and two features, got %+v", s.Features)
	}
	sched := s.Features[0]
	if got := reqTitles(sched); strings.Join(got, "|") != "Users must drag an event|Recurring events should support weekly rules|Time zones" {
		t.Fatalf("acceptance criteria should become the feature's requirements, got %v", got)
	}
	if !strings.Contains(sched.Requirements[0].Description, "saved within 1 second") {
		t.Errorf("nested bullets belong to their item: %q", sched.Requirements[0].Description)
	}
	prio := map[string]string{}
	for _, f := range s.Features {
		for _, r := range f.Requirements {
			prio[r.Title] = r.Priority
		}
	}
	if prio["Users must drag an event"] != "high" || prio["Recurring events should support weekly rules"] != "medium" ||
		prio["Outlook support is nice to have"] != "low" || prio["Sync runs every 5 minutes; users may trigger it"] != "" {
		t.Errorf("priorities: %v", prio)
	}
	if len(s.Features[1].Requirements) != 2 {
		t.Errorf("fenced code is not requirements: %v", reqTitles(s.Features[1]))
	}
	if len(s.Constraints) != 1 || !strings.Contains(s.Constraints[0].Description, "EU") {
		t.Errorf("non-functional requirements become constraints: %+v", s.Constraints)
	}
}

func TestMarkdownImportEdgeCases(t *testing.T) {
	// A leaf "Features" list names features; prose-only sections stay features
	// with a description, as before.
	s := parseDoc(t, "# P\n\n## Features\n- Login\n- Billing\n\n## Reporting\nMonthly PDF reports.\n")
	var ids []string
	for _, f := range s.Features {
		ids = append(ids, f.ID)
	}
	if strings.Join(ids, ",") != "login,billing,reporting" {
		t.Fatalf("features: %v", ids)
	}

	// Duplicate titles still get unique ids.
	s = parseDoc(t, "# P\n\n## A\n- Export data\n\n## B\n- Export data\n")
	if s.Features[0].Requirements[0].ID == s.Features[1].Requirements[0].ID {
		t.Fatal("requirement ids must be unique across the spec")
	}
}

func TestMarkdownImportCleansLeadInTitles(t *testing.T) {
	s := parseDoc(t, "# P\n\n## Now\n- **`ROADY_MCP_TOOLS` trims what is advertised.** All ~70 tools were registered.\n")
	if got := s.Features[0].Requirements[0].Title; got != "ROADY_MCP_TOOLS trims what is advertised" {
		t.Errorf("title = %q", got)
	}
}
