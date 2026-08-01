// Package report renders a domain report into formats a person can read
// without installing anything: Markdown for commits, pull requests, and chat;
// HTML for email and static publishing.
package report

import (
	"fmt"
	"strconv"
	"strings"

	domainreport "github.com/felixgeelhaar/roady/pkg/domain/report"
)

// Markdown renders a report as CommonMark. The output is deliberately plain —
// no HTML fallbacks, no emoji — so it survives being pasted into a PR
// description, a Slack message, or an email.
func Markdown(r *domainreport.Report) string {
	var b strings.Builder

	title := r.Project
	if title == "" {
		title = "Project"
	}

	fmt.Fprintf(&b, "# %s — progress report\n\n", title)
	fmt.Fprintf(&b, "**%s**\n\n", r.Headline())
	fmt.Fprintf(&b, "Generated %s", r.GeneratedAt.Format("2006-01-02 15:04 MST"))
	if r.Since != nil {
		fmt.Fprintf(&b, " · covering changes since %s", r.Since.Format("2006-01-02"))
	}
	b.WriteString("\n\n")

	writeMarkdownProgress(&b, r.Progress)
	writeMarkdownForecast(&b, r.Forecast)
	writeMarkdownRisks(&b, r.Risks)
	writeMarkdownAssignments(&b, r.Assignments)
	writeMarkdownChanges(&b, r.Changes)

	return b.String()
}

func writeMarkdownProgress(b *strings.Builder, p domainreport.Progress) {
	b.WriteString("## Progress\n\n")

	if p.Total == 0 {
		b.WriteString("No plan has been generated yet.\n\n")
		return
	}

	fmt.Fprintf(b, "%s of %s tasks complete (%s).\n\n",
		strconv.Itoa(p.Done+p.Verified), strconv.Itoa(p.Total), formatPercent(p.Percent))

	b.WriteString("| Status | Count |\n| --- | --- |\n")
	for _, row := range []struct {
		label string
		count int
	}{
		{"Verified", p.Verified},
		{"Done", p.Done},
		{"In progress", p.InProgress},
		{"Blocked", p.Blocked},
		{"Ready", p.Ready},
		{"Pending", p.Pending},
	} {
		if row.count > 0 {
			fmt.Fprintf(b, "| %s | %d |\n", row.label, row.count)
		}
	}
	b.WriteString("\n")
}

func writeMarkdownForecast(b *strings.Builder, f *domainreport.Forecast) {
	if f == nil {
		return
	}

	b.WriteString("## Forecast\n\n")

	// Showing a velocity figure derived from two data points invites false
	// confidence, so say so plainly instead.
	if !f.Reliable() {
		b.WriteString("Not enough completed work yet to forecast a completion date.\n\n")
		return
	}

	fmt.Fprintf(b, "At the current pace of %.2f tasks/day, completion is expected in about **%s**",
		f.Velocity, formatDays(f.EstimatedDays))
	if f.HighDays > 0 {
		fmt.Fprintf(b, " (range %s to %s)", formatDays(f.LowDays), formatDays(f.HighDays))
	}
	b.WriteString(".\n\n")

	if f.Trend != "" {
		fmt.Fprintf(b, "Velocity trend: %s.\n\n", f.Trend)
	}
}

func writeMarkdownRisks(b *strings.Builder, risks []domainreport.Risk) {
	b.WriteString("## Risks\n\n")

	if len(risks) == 0 {
		b.WriteString("None open.\n\n")
		return
	}

	b.WriteString("| Severity | Kind | Component | Detail |\n| --- | --- | --- | --- |\n")
	for _, risk := range risks {
		detail := risk.Message
		if risk.DaysPending > 0 {
			detail += fmt.Sprintf(" (open %d days)", risk.DaysPending)
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n",
			risk.Severity, risk.Kind, orDash(risk.Component), escapePipes(detail))
	}
	b.WriteString("\n")
}

func writeMarkdownAssignments(b *strings.Builder, assignments []domainreport.Assignment) {
	b.WriteString("## Who is on what\n\n")

	if len(assignments) == 0 {
		b.WriteString("No tasks yet.\n\n")
		return
	}

	for _, a := range assignments {
		name := a.Owner
		if a.Unassigned {
			name = "Unassigned"
		}

		fmt.Fprintf(b, "**%s** — %d active, %d blocked, %d done\n\n", name, a.Active, a.Blocked, a.Done)
		for _, task := range a.OpenTasks {
			fmt.Fprintf(b, "- `%s` %s _(%s)_\n", task.ID, task.Title, task.Status)
		}
		if len(a.OpenTasks) == 0 {
			b.WriteString("- no open tasks\n")
		}
		b.WriteString("\n")
	}
}

func writeMarkdownChanges(b *strings.Builder, changes []domainreport.Change) {
	b.WriteString("## What changed\n\n")

	if len(changes) == 0 {
		b.WriteString("No recorded activity in this window.\n\n")
		return
	}

	for _, c := range changes {
		fmt.Fprintf(b, "- %s — %s", c.At.Format("2006-01-02"), c.Summary)
		if c.Actor != "" {
			fmt.Fprintf(b, " (%s)", c.Actor)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func formatPercent(v float64) string {
	if v == float64(int(v)) {
		return strconv.Itoa(int(v)) + "%"
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + "%"
}

// formatDays renders a day count the way a person would say it.
func formatDays(days float64) string {
	switch {
	case days <= 0:
		return "0 days"
	case days < 1:
		return "under a day"
	case days < 14:
		return fmt.Sprintf("%.0f days", days)
	case days < 60:
		return fmt.Sprintf("%.0f weeks", days/7)
	default:
		return fmt.Sprintf("%.1f months", days/30)
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// escapePipes keeps a message containing a pipe from breaking the table row
// it sits in.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
