package report

import (
	"fmt"
	"strings"

	domainreport "github.com/felixgeelhaar/roady/pkg/domain/report"
)

// maxDigestRisks caps how many risks a digest names individually. A chat
// message that lists twenty risks gets ignored; three plus a count gets read.
const maxDigestRisks = 3

// Digest renders a short plain-text summary sized for a chat message or an
// email subject-plus-preview. It uses Slack-compatible single-asterisk bold,
// which degrades to literal asterisks elsewhere without becoming unreadable.
//
// Unlike Markdown, a digest is a nudge rather than a document: it says where
// things stand, what is at risk, and nothing else.
func Digest(r *domainreport.Report) string {
	var b strings.Builder

	title := r.Project
	if title == "" {
		title = "Project"
	}

	fmt.Fprintf(&b, "*%s* — %s\n", title, r.Headline())

	p := r.Progress
	if p.Total > 0 {
		fmt.Fprintf(&b, "%d/%d done · %d in progress · %d blocked · %d ready\n",
			p.Done+p.Verified, p.Total, p.InProgress, p.Blocked, p.Ready)
	}

	if f := r.Forecast; f.Reliable() {
		fmt.Fprintf(&b, "Pace %s tasks/day, ~%s remaining\n",
			trimVelocity(f.Velocity), formatDays(f.EstimatedDays))
	}

	if len(r.Risks) > 0 {
		b.WriteString("\n*Risks*\n")
		for i, risk := range r.Risks {
			if i == maxDigestRisks {
				fmt.Fprintf(&b, "…and %d more\n", len(r.Risks)-maxDigestRisks)
				break
			}
			fmt.Fprintf(&b, "• [%s] %s", risk.Severity, risk.Message)
			if risk.Component != "" {
				fmt.Fprintf(&b, " (%s)", risk.Component)
			}
			b.WriteString("\n")
		}
	}

	if unassigned := unassignedCount(r); unassigned > 0 {
		fmt.Fprintf(&b, "\n%d open task(s) have no owner.\n", unassigned)
	}

	return b.String()
}

func unassignedCount(r *domainreport.Report) int {
	for _, a := range r.Assignments {
		if a.Unassigned {
			return len(a.OpenTasks)
		}
	}
	return 0
}

func trimVelocity(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}
