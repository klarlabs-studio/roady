package drift

import "testing"

func TestReportExceeds(t *testing.T) {
	report := &Report{Issues: []Issue{
		{Severity: SeverityLow}, {Severity: SeverityMedium}, {Severity: SeverityHigh},
	}}

	tests := []struct {
		name      string
		threshold Severity
		want      int
	}{
		{name: "critical: nothing here reaches it", threshold: SeverityCritical, want: 0},
		{name: "high: one issue", threshold: SeverityHigh, want: 1},
		{name: "medium: two issues", threshold: SeverityMedium, want: 2},
		{name: "low: everything", threshold: SeverityLow, want: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := len(report.AtOrAbove(tt.threshold)); got != tt.want {
				t.Errorf("AtOrAbove(%s) = %d issues, want %d", tt.threshold, got, tt.want)
			}
		})
	}
}

func TestAtOrAboveEmptyReport(t *testing.T) {
	if got := (&Report{}).AtOrAbove(SeverityLow); len(got) != 0 {
		t.Errorf("expected none, got %d", len(got))
	}
}

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		in      string
		want    Severity
		wantErr bool
	}{
		{in: "low", want: SeverityLow},
		{in: "MEDIUM", want: SeverityMedium},
		{in: " high ", want: SeverityHigh},
		{in: "critical", want: SeverityCritical},
		{in: "", wantErr: true},
		{in: "catastrophic", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseSeverity(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseSeverity(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestUnknownSeverityDoesNotSilentlyPass guards the dangerous direction: an
// issue whose severity string is unrecognised must still count, or a typo in
// a severity would quietly remove it from every gate.
func TestUnknownSeverityDoesNotSilentlyPass(t *testing.T) {
	report := &Report{Issues: []Issue{{Severity: Severity("bogus")}}}

	if got := len(report.AtOrAbove(SeverityLow)); got != 1 {
		t.Errorf("an unrecognised severity should still be reported at the lowest threshold, got %d", got)
	}
}
