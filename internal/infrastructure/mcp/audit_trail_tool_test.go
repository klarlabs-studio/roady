package mcp

import (
	"testing"
	"time"
)

func TestParseTrailSince(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(time.Time) bool
	}{
		{name: "empty means all history", input: "", check: func(v time.Time) bool { return v.IsZero() }},
		{name: "days", input: "7d", check: func(v time.Time) bool { return !v.IsZero() && v.Before(time.Now()) }},
		{name: "weeks", input: "2w", check: func(v time.Time) bool { return !v.IsZero() && v.Before(time.Now()) }},
		{name: "absolute date", input: "2026-07-01", check: func(v time.Time) bool { return v.Year() == 2026 && v.Month() == 7 }},
		{name: "whitespace tolerated", input: "  7d ", check: func(v time.Time) bool { return !v.IsZero() }},
		{name: "zero rejected", input: "0d", wantErr: true},
		{name: "negative rejected", input: "-3d", wantErr: true},
		{name: "garbage rejected", input: "soon", wantErr: true},
		{name: "impossible date rejected", input: "2026-13-45", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTrailSince(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.check(got) {
				t.Errorf("parseTrailSince(%q) = %v, which failed its check", tt.input, got)
			}
		})
	}
}

// TestAuditTrailRequiresASubject: a trail over "everything" is not evidence of
// anything, so the tool asks rather than guessing.
func TestAuditTrailRequiresASubject(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	res, err := server.handleAuditTrail(t.Context(), AuditTrailArgs{})

	assertToolError(t, res, err, "task_id")
}

func TestAuditTrailRejectsBadSince(t *testing.T) {
	server := setupCoordinatorTestServer(t)

	res, err := server.handleAuditTrail(t.Context(), AuditTrailArgs{TaskID: "task-1", Since: "whenever"})

	assertToolError(t, res, err, "invalid since")
}
