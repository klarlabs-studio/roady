package report

import (
	"strings"
	"testing"
	"time"

	domainreport "github.com/felixgeelhaar/roady/pkg/domain/report"
)

func TestDigestIsCompact(t *testing.T) {
	out := Digest(sampleReport())

	for _, want := range []string{
		"*checkout* — 50% complete, 2 risks open",
		"5/10 done · 2 in progress · 1 blocked · 1 ready",
		"Pace 1.25 tasks/day",
		"*Risks*",
		"[high] done task has no file (auth)",
		"1 open task(s) have no owner.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("digest missing %q\n---\n%s", want, out)
		}
	}

	// A digest is a nudge, not a document — it must stay chat-sized.
	if lines := strings.Count(out, "\n"); lines > 12 {
		t.Errorf("digest should stay short, got %d lines:\n%s", lines, out)
	}
	if strings.Contains(out, "What changed") {
		t.Error("digest must not include the full change log")
	}
}

func TestDigestTruncatesLongRiskLists(t *testing.T) {
	r := sampleReport()
	r.Risks = nil
	for range 7 {
		r.Risks = append(r.Risks, domainreport.Risk{Severity: "low", Message: "issue"})
	}

	out := Digest(r)

	if !strings.Contains(out, "…and 4 more") {
		t.Errorf("expected the risk list to be capped at %d with a remainder count:\n%s", maxDigestRisks, out)
	}
	if strings.Count(out, "• [low]") != maxDigestRisks {
		t.Errorf("expected exactly %d listed risks, got %d", maxDigestRisks, strings.Count(out, "• [low]"))
	}
}

func TestDigestOmitsUnreliableForecast(t *testing.T) {
	r := sampleReport()
	r.Forecast = &domainreport.Forecast{Velocity: 5, DataPoints: 1, EstimatedDays: 2}

	if out := Digest(r); strings.Contains(out, "Pace") {
		t.Errorf("a forecast below the reliability bar must not appear in a digest:\n%s", out)
	}
}

func TestDigestEmptyProject(t *testing.T) {
	out := Digest(&domainreport.Report{GeneratedAt: time.Now()})

	if !strings.Contains(out, "No plan yet") {
		t.Errorf("expected an empty-project headline, got:\n%s", out)
	}
	if strings.Contains(out, "*Risks*") {
		t.Error("no risks section when there are none")
	}
}

func TestTrimVelocity(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{in: 1.25, want: "1.25"},
		{in: 2.0, want: "2"},
		{in: 1.50, want: "1.5"},
		{in: 0.1, want: "0.1"},
	}

	for _, tt := range tests {
		if got := trimVelocity(tt.in); got != tt.want {
			t.Errorf("trimVelocity(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
