// Package dispatch models handing one ready task to a subagent.
//
// An agent asked to "work on task-42" has to reconstruct why the task exists,
// what would count as finished, and how to report back. All three are already
// in Roady — the spec citation that motivated the task, the requirement text,
// and the transition that records completion — so a dispatch hands them over
// rather than leaving the agent to infer them.
//
// The completion contract is the part that matters. Work done by a subagent
// that never lands as a recorded transition is invisible to the audit trail:
// the task stays in progress, no evidence attaches, and nothing says which
// agent did it. The brief therefore names the exact call to make, with the
// session and agent already filled in.
package dispatch

import "github.com/felixgeelhaar/roady/pkg/domain/planning"

// Brief is everything a subagent needs to complete one task.
type Brief struct {
	TaskID      string `json:"task_id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	// Why the task exists: the feature it serves and the requirement text,
	// so the agent works from intent rather than from a title.
	Feature     string `json:"feature,omitempty"`
	Requirement string `json:"requirement,omitempty"`

	// Citation points at the document and line that motivated the task.
	// Without it an agent cannot check its work against the source.
	Citation string `json:"citation,omitempty"`

	// Acceptance states what finished means. Empty when the spec did not say,
	// which is worth surfacing rather than inventing.
	Acceptance string `json:"acceptance,omitempty"`

	Priority planning.TaskPriority `json:"priority,omitempty"`
	Estimate string                `json:"estimate,omitempty"`

	// DependsOn lists prerequisites, all already satisfied — a task is only
	// dispatchable when it is ready.
	DependsOn []string `json:"depends_on,omitempty"`

	// Completion is how the agent reports back.
	Completion CompletionContract `json:"completion"`
}

// CompletionContract is the exact call that closes the task.
//
// It carries the session and agent so the resulting audit entry attributes
// the work to whoever actually did it, rather than to the process that
// dispatched it.
type CompletionContract struct {
	// Tool is the MCP tool to call, or the CLI equivalent when not over MCP.
	Tool string `json:"tool"`
	// CLI is the shell form, for an agent not speaking MCP.
	CLI string `json:"cli"`
	// Arguments are the values the tool expects.
	Arguments map[string]string `json:"arguments"`
	// EvidenceRequired says the transition should carry proof — a commit
	// hash or a link. A task marked done with no evidence is the classic
	// audit finding, so the brief asks for it up front.
	EvidenceRequired bool `json:"evidence_required"`
	// Instructions is a one-line human-readable summary of the contract.
	Instructions string `json:"instructions"`
}

// HasCitation reports whether the task can be traced back to a source
// document.
func (b *Brief) HasCitation() bool { return b.Citation != "" }

// Warnings lists what the brief could not supply, so a dispatcher sees the
// gaps instead of discovering them through vague work.
func (b *Brief) Warnings() []string {
	var out []string
	if b.Citation == "" {
		out = append(out, "No source citation: this task cannot be traced to a document, so the agent cannot check its work against the original intent.")
	}
	if b.Acceptance == "" {
		out = append(out, "No acceptance criteria: the spec does not say what finished means for this task.")
	}
	if b.Description == "" && b.Requirement == "" {
		out = append(out, "No description or requirement text: the agent has only a title to work from.")
	}
	return out
}
