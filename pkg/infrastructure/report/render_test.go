package report

import (
	"strings"
	"testing"
	"time"

	domainreport "github.com/felixgeelhaar/roady/pkg/domain/report"
)

func sampleReport() *domainreport.Report {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	return &domainreport.Report{
		Project:     "checkout",
		GeneratedAt: time.Date(2026, 8, 1, 9, 30, 0, 0, time.UTC),
		Since:       &since,
		Progress: domainreport.Progress{
			Total: 10, Done: 4, Verified: 1, InProgress: 2, Blocked: 1, Ready: 1, Pending: 1, Percent: 50,
		},
		Forecast: &domainreport.Forecast{
			Velocity: 1.25, EstimatedDays: 8, LowDays: 5, HighDays: 14, Trend: "steady", DataPoints: 9,
		},
		Assignments: []domainreport.Assignment{
			{Owner: "alice", Active: 2, Done: 1, OpenTasks: []domainreport.TaskLine{
				{ID: "task-1", Title: "Wire payment intent", Status: "in_progress"},
			}},
			{Unassigned: true, Active: 1, OpenTasks: []domainreport.TaskLine{
				{ID: "task-9", Title: "Refund flow", Status: "pending"},
			}},
		},
		Risks: []domainreport.Risk{
			{Severity: "high", Kind: "drift/code", Component: "auth", Message: "done task has no file"},
			{Severity: "low", Kind: "debt/neglect", Component: "billing", Message: "stale", DaysPending: 21},
		},
		Changes: []domainreport.Change{
			{At: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC), Action: "task.completed", Actor: "alice", Summary: "task-3 completed"},
		},
	}
}

func TestMarkdownIncludesEverySection(t *testing.T) {
	out := Markdown(sampleReport())

	for _, want := range []string{
		"# checkout — progress report",
		"50% complete, 2 risks open",
		"covering changes since 2026-07-01",
		"## Progress",
		"## Forecast",
		"## Risks",
		"## Who is on what",
		"## What changed",
		"**alice**",
		"Unassigned",
		"task-3 completed",
		"(open 21 days)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, out)
		}
	}
}

func TestMarkdownEmptyProject(t *testing.T) {
	out := Markdown(&domainreport.Report{GeneratedAt: time.Now()})

	for _, want := range []string{
		"# Project — progress report",
		"No plan yet",
		"No plan has been generated yet.",
		"None open.",
		"No recorded activity in this window.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markdown missing %q\n---\n%s", want, out)
		}
	}
}

func TestMarkdownUnreliableForecastSaysSo(t *testing.T) {
	r := sampleReport()
	r.Forecast = &domainreport.Forecast{Velocity: 1, DataPoints: 1, EstimatedDays: 99}

	out := Markdown(r)

	if !strings.Contains(out, "Not enough completed work") {
		t.Errorf("expected an explicit low-confidence note, got:\n%s", out)
	}
	if strings.Contains(out, "99") {
		t.Error("an unreliable forecast must not print a completion estimate")
	}
}

func TestMarkdownEscapesPipesInTables(t *testing.T) {
	r := sampleReport()
	r.Risks = []domainreport.Risk{{Severity: "high", Kind: "drift/code", Component: "a", Message: "foo | bar"}}

	out := Markdown(r)

	if !strings.Contains(out, `foo \| bar`) {
		t.Errorf("pipe in a message must be escaped so the table survives:\n%s", out)
	}
}

func TestFormatDays(t *testing.T) {
	tests := []struct {
		days float64
		want string
	}{
		{days: 0, want: "0 days"},
		{days: -3, want: "0 days"},
		{days: 0.5, want: "under a day"},
		{days: 6, want: "6 days"},
		{days: 21, want: "3 weeks"},
		{days: 90, want: "3.0 months"},
	}

	for _, tt := range tests {
		if got := formatDays(tt.days); got != tt.want {
			t.Errorf("formatDays(%v) = %q, want %q", tt.days, got, tt.want)
		}
	}
}

func TestHTMLRendersAndIsSelfContained(t *testing.T) {
	out, err := HTML(sampleReport())
	if err != nil {
		t.Fatalf("HTML() failed: %v", err)
	}

	for _, want := range []string{
		"<title>checkout — progress report</title>",
		"50% complete, 2 risks open",
		"1.25 tasks/day",
		"Unassigned",
		"task-3 completed",
		"sev-high",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("html missing %q", want)
		}
	}

	// The page must not reach out to the network or run anything.
	for _, forbidden := range []string{"<script", "http://", "https://", "src=", "@import"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("html must be self-contained and script-free, found %q", forbidden)
		}
	}
}

func TestHTMLEscapesUntrustedContent(t *testing.T) {
	r := sampleReport()
	r.Project = `<script>alert(1)</script>`
	r.Risks = []domainreport.Risk{{
		Severity: "high",
		Kind:     "drift/code",
		Message:  `<img src=x onerror="alert(1)">`,
	}}
	r.Assignments = []domainreport.Assignment{{
		Owner:     `<b>bob</b>`,
		OpenTasks: []domainreport.TaskLine{{ID: "t1", Title: `<i>title</i>`, Status: "pending"}},
	}}

	out, err := HTML(r)
	if err != nil {
		t.Fatalf("HTML() failed: %v", err)
	}

	// Task titles, owners, and drift messages all originate outside Roady —
	// from spec docs, git config, and scanned code — so none of it can be
	// trusted to be inert markup.
	for _, forbidden := range []string{
		"<script>alert(1)</script>",
		`<img src=x`,
		"<b>bob</b>",
		"<i>title</i>",
	} {
		if strings.Contains(out, forbidden) {
			t.Errorf("unescaped untrusted content in output: %q", forbidden)
		}
	}

	if !strings.Contains(out, "&lt;script&gt;") {
		t.Error("expected the project name to be HTML-escaped")
	}
}

func TestSeverityClass(t *testing.T) {
	tests := []struct{ severity, want string }{
		{"critical", "sev-high"},
		{"high", "sev-high"},
		{"medium", "sev-medium"},
		{"low", "sev-low"},
		{"", "sev-low"},
	}

	for _, tt := range tests {
		if got := severityClass(tt.severity); got != tt.want {
			t.Errorf("severityClass(%q) = %q, want %q", tt.severity, got, tt.want)
		}
	}
}
