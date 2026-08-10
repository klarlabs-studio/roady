package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

// A tool result carrying no structured payload must omit structuredContent,
// not encode it as null.
//
// `null` matches no object schema, so a strict client rejects the entire
// result during validation — including the text content that explains what
// happened. The reporter of #92 hit this with an invalid status enum: the
// handler correctly returned "Invalid status", and the encoding threw that
// message away, leaving a schema-validation failure in its place. For the
// mutating tool in the same report it was worse — the caller could not tell
// whether the write had applied.
//
// The defect was in go.klarlabs.de/mcp (fixed in v1.24.1), but the assertion
// belongs here too: roady is what emits these results, and a future
// dependency change that reintroduced the null would otherwise be invisible
// until a client rejected it.
func TestErrorResultsOmitStructuredContent(t *testing.T) {
	tests := []struct {
		name   string
		result any
	}{
		{"mcpErr", mcpErr("Invalid status. Use ready, in_progress, blocked, unassigned, or all.")},
		{"mcpErrCause", mcpErrCause("Failed to load project at the given path.", errTest{})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.result)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := string(b)

			if strings.Contains(got, `"structuredContent":null`) {
				t.Errorf("emits null structuredContent, which fails strict client validation:\n  %s", got)
			}
			// The text is the whole point of an error result; it must survive.
			if !strings.Contains(got, `"text"`) {
				t.Errorf("error result carries no text content: %s", got)
			}
			if !strings.Contains(got, `"isError":true`) {
				t.Errorf("error result is not flagged as an error: %s", got)
			}
		})
	}
}

type errTest struct{}

func (errTest) Error() string { return "underlying cause" }
