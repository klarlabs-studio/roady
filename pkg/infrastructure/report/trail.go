package report

import (
	"fmt"
	"sort"
	"strings"

	domainaudit "github.com/felixgeelhaar/roady/pkg/domain/audit"
)

// TrailMarkdown renders an audit trail as a review document.
//
// The order is deliberate: findings first, then what the subject was, then
// the evidence, then the raw entries. A reviewer must not have to read to the
// bottom to discover the chain failed verification.
func TrailMarkdown(t *domainaudit.Trail) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Audit trail — %s `%s`\n\n", t.Subject.Kind, t.Subject.ID)
	fmt.Fprintf(&b, "Generated %s", t.GeneratedAt.Format("2006-01-02 15:04 MST"))
	if t.Subject.Since != nil {
		fmt.Fprintf(&b, " · covering %s onward", t.Subject.Since.Format("2006-01-02"))
	}
	b.WriteString("\n\n")

	writeTrailIntegrity(&b, t)
	writeTrailFindings(&b, t)
	writeTrailTask(&b, t)
	writeTrailActors(&b, t)
	writeTrailEntries(&b, t)
	writeTrailLimits(&b)

	return b.String()
}

func writeTrailIntegrity(b *strings.Builder, t *domainaudit.Trail) {
	b.WriteString("## Chain integrity\n\n")

	switch {
	case !t.Integrity.CheckedChain:
		b.WriteString("**Not checked** — chain verification was unavailable.\n\n")
	case t.Integrity.Verified:
		fmt.Fprintf(b, "**Verified.** The hash chain over %d recorded events is intact; no entry has been altered or removed since it was written.\n\n",
			t.Integrity.EventsInLog)
	default:
		b.WriteString("**FAILED.** The event log does not verify:\n\n")
		for _, p := range t.Integrity.Problems {
			fmt.Fprintf(b, "- %s\n", p)
		}
		b.WriteString("\n")
	}
}

func writeTrailFindings(b *strings.Builder, t *domainaudit.Trail) {
	findings := t.Findings()
	if len(findings) == 0 {
		return
	}

	b.WriteString("## Findings\n\n")
	for _, f := range findings {
		fmt.Fprintf(b, "- %s\n", f)
	}
	b.WriteString("\n")
}

func writeTrailTask(b *strings.Builder, t *domainaudit.Trail) {
	if t.Task == nil {
		return
	}

	b.WriteString("## Task\n\n")
	fmt.Fprintf(b, "| Field | Value |\n| --- | --- |\n")
	fmt.Fprintf(b, "| ID | `%s` |\n", t.Task.ID)
	fmt.Fprintf(b, "| Title | %s |\n", escapePipes(t.Task.Title))
	fmt.Fprintf(b, "| Status | %s |\n", orDash(t.Task.Status))
	fmt.Fprintf(b, "| Owner | %s |\n", orDash(t.Task.Owner))
	fmt.Fprintf(b, "| Origin | %s |\n", orDash(t.Task.Origin))

	if t.Task.SourceDoc != "" {
		fmt.Fprintf(b, "| Traces to | %s:%d |\n", t.Task.SourceDoc, t.Task.SourceLine)
	}
	if t.Task.StartedAt != nil {
		fmt.Fprintf(b, "| Started | %s |\n", t.Task.StartedAt.Format("2006-01-02 15:04 MST"))
	}
	if t.Task.CompletedAt != nil {
		fmt.Fprintf(b, "| Completed | %s |\n", t.Task.CompletedAt.Format("2006-01-02 15:04 MST"))
	}
	b.WriteString("\n")

	b.WriteString("### Evidence\n\n")
	if !t.HasEvidence() {
		b.WriteString("None recorded.\n\n")
		return
	}
	for _, e := range t.Task.Evidence {
		fmt.Fprintf(b, "- %s\n", e)
	}

	providers := make([]string, 0, len(t.Task.ExternalRefs))
	for p := range t.Task.ExternalRefs {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	for _, p := range providers {
		fmt.Fprintf(b, "- %s: %s\n", p, t.Task.ExternalRefs[p])
	}
	b.WriteString("\n")
}

func writeTrailActors(b *strings.Builder, t *domainaudit.Trail) {
	b.WriteString("## Who acted\n\n")

	if len(t.Actors) == 0 {
		b.WriteString("No recorded actors.\n\n")
		return
	}

	b.WriteString("| Actor | Agent | Session | Actions | First | Last |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, a := range t.Actors {
		fmt.Fprintf(b, "| %s | %s | %s | %d | %s | %s |\n",
			orDash(a.Actor), orDash(a.Agent), orDash(shortSession(a.SessionID)), a.Actions,
			a.FirstSeen.Format("2006-01-02 15:04"), a.LastSeen.Format("2006-01-02 15:04"))
	}
	b.WriteString("\n")
}

func writeTrailEntries(b *strings.Builder, t *domainaudit.Trail) {
	b.WriteString("## Recorded events\n\n")

	if len(t.Entries) == 0 {
		b.WriteString("None.\n\n")
		return
	}

	b.WriteString("| When | Action | Actor | Agent | Session |\n| --- | --- | --- | --- | --- |\n")
	for _, e := range t.Entries {
		action := e.Action
		if e.Detail != "" {
			action += " (" + e.Detail + ")"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
			e.At.Format("2006-01-02 15:04:05"), escapePipes(action),
			orDash(e.Actor), orDash(e.Agent), orDash(shortSession(e.SessionID)))
	}
	b.WriteString("\n")
}

// writeTrailLimits is not optional boilerplate. A document that looks like an
// attestation will be read as one, so the limit travels with it.
func writeTrailLimits(b *strings.Builder) {
	b.WriteString("## What this document attests\n\n")
	b.WriteString("This trail is assembled from Roady's hash-chained event log. When chain\n")
	b.WriteString("integrity verifies, the entries above are a complete and unaltered record of\n")
	b.WriteString("what was written at the time.\n\n")
	b.WriteString("**It is not proof of identity.** Actor, agent, and session values are asserted\n")
	b.WriteString("by the caller and are never authenticated. This document attests to what was\n")
	b.WriteString("claimed, not to who acted.\n")
}

// shortSession truncates a UUID-shaped session for table display while
// staying long enough to distinguish runs.
func shortSession(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12] + "…"
}
