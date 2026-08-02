package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/dispatch"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// printBrief renders what a subagent is handed. The completion contract is the
// load-bearing part: work done by an agent that never lands as a recorded
// transition is invisible to the audit trail, so the brief has to name the
// exact call that closes the task.
func TestPrintBriefRendersTheCompletionContract(t *testing.T) {
	brief := &dispatch.Brief{
		TaskID:      "task-42",
		Title:       "Rotate the API key",
		Description: "Rotate and redeploy.",
		Feature:     "Credential hygiene",
		Requirement: "Keys rotate every 90 days.",
		Citation:    "docs/security.md:31",
		Acceptance:  "Keys rotate every 90 days.",
		Priority:    planning.PriorityHigh,
		Estimate:    "1d",
		DependsOn:   []string{"task-41"},
		Completion: dispatch.CompletionContract{
			Tool:             "roady_transition_task",
			CLI:              "ROADY_AGENT=claude-code roady task complete task-42 --evidence <commit>",
			EvidenceRequired: true,
			Instructions:     "Call roady_transition_task when done.",
		},
	}

	out := captureStdout(t, func() { printBrief(brief, false) })

	for _, want := range []string{
		"task-42", "Rotate the API key",
		"Credential hygiene",          // why the task exists
		"docs/security.md:31",         // how to check the work
		"Done when:",                  // acceptance
		"task-41",                     // satisfied dependency
		"roady_transition_task",       // the MCP call
		"roady task complete task-42", // the CLI equivalent
	} {
		if !strings.Contains(out, want) {
			t.Errorf("brief does not mention %q:\n%s", want, out)
		}
	}
}

// Gaps are surfaced rather than papered over: an agent working from a title
// alone should be visible to whoever dispatched it.
func TestPrintBriefWarnsAboutGaps(t *testing.T) {
	bare := &dispatch.Brief{
		TaskID: "task-7",
		Title:  "Do the thing",
		Completion: dispatch.CompletionContract{
			Tool: "roady_transition_task",
			CLI:  "roady task complete task-7",
		},
	}

	warnings := bare.Warnings()
	if len(warnings) != 3 {
		t.Fatalf("got %d warnings, want 3 (citation, acceptance, description): %v", len(warnings), warnings)
	}

	// The brief itself still renders; warnings go to stderr so a piped brief
	// stays a brief.
	out := captureStdout(t, func() { printBrief(bare, true) })
	if !strings.Contains(out, "task-7") {
		t.Errorf("brief body missing:\n%s", out)
	}
	for _, w := range warnings {
		if strings.Contains(out, w) {
			t.Errorf("warning leaked into stdout: %q", w)
		}
	}
}
