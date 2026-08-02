// Package prompt models work that needs a language model, without Roady
// running one.
//
// Roady used to embed provider clients and call Anthropic, OpenAI, Gemini, or
// Ollama itself. That made sense before agents were the primary caller; it
// stopped making sense once they were. An agent invoking Roady already has a
// model, a key, a budget, and far more context about the task than Roady can
// reconstruct from files — so Roady calling a second model meant a second
// credential to configure, a second bill, and a worse answer.
//
// A Request is the whole job handed back to the caller: the system framing,
// the assembled prompt, the shape the answer should take, and which Roady
// tool to send the answer to. Roady still owns what it is good at — reading
// spec, plan, drift, and state, and assembling exactly the right context —
// and leaves inference to whoever asked.
package prompt

// Operation names the task a Request describes. Callers can branch on it
// without parsing prose.
type Operation string

const (
	OpDecomposeSpec     Operation = "decompose_spec"
	OpExplainSpec       Operation = "explain_spec"
	OpReviewSpec        Operation = "review_spec"
	OpSuggestPriorities Operation = "suggest_priorities"
	OpQueryProject      Operation = "query_project"
	OpExplainDrift      Operation = "explain_drift"
	OpPatchDrift        Operation = "patch_drift"
)

// Request is everything needed to run one inference and return the result to
// Roady.
type Request struct {
	// Operation identifies the task.
	Operation Operation `json:"operation"`

	// System is the framing to apply to the model.
	System string `json:"system,omitempty"`

	// Prompt is the assembled instruction, with project context already
	// interpolated. It is ready to send as-is.
	Prompt string `json:"prompt"`

	// ExpectedFormat describes the answer Roady can consume. Empty means
	// free prose, which is correct for explanations nobody parses.
	ExpectedFormat string `json:"expected_format,omitempty"`

	// WriteBack names the Roady tool that accepts the result, or is empty
	// when the answer is for the human and Roady stores nothing. Without
	// this a caller gets an answer and no idea what to do with it.
	WriteBack string `json:"write_back,omitempty"`

	// Guidance is a one-line instruction to the calling agent, so the
	// round trip is obvious from the response alone.
	Guidance string `json:"guidance,omitempty"`
}

// NeedsWriteBack reports whether the result is meant to come back to Roady
// rather than just being shown to a person.
func (r *Request) NeedsWriteBack() bool { return r.WriteBack != "" }
