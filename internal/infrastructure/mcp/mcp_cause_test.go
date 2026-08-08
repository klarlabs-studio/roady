package mcp

import (
	"errors"
	"strings"
	"testing"
)

// A tool failure must carry the diagnosis, not just a category. roady's errors
// name the file, line and reason; discarding them left an MCP caller unable to
// tell a corrupt spec from an uninitialised project (#84).
func TestMcpErrCauseCarriesTheDiagnosis(t *testing.T) {
	tests := []struct {
		name     string
		friendly string
		err      error
		want     []string
		absent   string
	}{
		{
			name:     "cause is appended",
			friendly: "Failed to add feature.",
			err:      errors.New("failed to unmarshal spec: yaml: line 11: could not find expected ':'"),
			want:     []string{"Failed to add feature.", "yaml: line 11", "could not find expected"},
		},
		{
			name:     "nil error degrades to the friendly message alone",
			friendly: "Failed to add feature.",
			err:      nil,
			want:     []string{"Failed to add feature."},
			absent:   "Cause:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := mcpErrCause(tc.friendly, tc.err)
			if !res.IsError {
				t.Fatal("IsError = false; a failure must be reported as one")
			}
			if len(res.Content) == 0 {
				t.Fatal("no content; the caller would see an empty failure")
			}
			text := res.Content[0].Text
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("text = %q, want it to contain %q", text, w)
				}
			}
			if tc.absent != "" && strings.Contains(text, tc.absent) {
				t.Errorf("text = %q, want it not to contain %q", text, tc.absent)
			}
		})
	}
}
