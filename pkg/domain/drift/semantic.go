package drift

import "fmt"

// CategorySemantic marks a requirement whose implementation matches
// structurally but no longer means what the requirement says.
//
// Every other category is decidable by comparing artifacts: a task is missing,
// an id is orphaned, a file does not exist. None of them can tell whether code
// that exists still does what the requirement asked for — a requirement saying
// "sessions expire after 30 minutes" is structurally satisfied by any
// implementation at all, including one that expires them after thirty days.
// That question needs a reader, so Roady frames it and a language model the
// caller already has answers it.
const CategorySemantic DriftCategory = "SEMANTIC"

// SemanticQuestion is one requirement paired with everything needed to judge
// whether its implementation still agrees with it.
//
// It carries no opinion. The judgement is the caller's, and Roady supplies
// only what it knows and the caller would otherwise have to reconstruct: what
// was asked for, where the work landed, and how to check that against the
// original document.
type SemanticQuestion struct {
	// RequirementID identifies the requirement being asked about.
	RequirementID string `json:"requirement_id"`
	// FeatureID is the feature the requirement belongs to.
	FeatureID string `json:"feature_id"`
	// Requirement is the natural-language text to judge against.
	Requirement string `json:"requirement"`

	// TaskID is the task that claimed to implement it, empty when none did.
	TaskID string `json:"task_id,omitempty"`
	// Status is that task's execution status.
	Status string `json:"status,omitempty"`
	// Paths are where the implementation is recorded to live.
	Paths []string `json:"paths,omitempty"`
	// Evidence is what was attached when the task was completed.
	Evidence []string `json:"evidence,omitempty"`
	// Citation is the doc:line the requirement came from, so the caller can
	// check the implementation against the source rather than a paraphrase.
	Citation string `json:"citation,omitempty"`
}

// Answerable reports whether there is enough here to ask the question at all.
// A requirement nothing claims to implement is structural drift, already
// reported by the other detectors; asking a model about it wastes a call and
// invites a confident answer about nothing.
func (q SemanticQuestion) Answerable() bool {
	return q.Requirement != "" && q.TaskID != ""
}

// SemanticJudgement is the caller's answer for one requirement.
type SemanticJudgement struct {
	RequirementID string `json:"requirement_id"`
	// Agrees is the verdict: does the implementation still mean what the
	// requirement says?
	Agrees bool `json:"agrees"`
	// Explanation is why. Required when Agrees is false — a divergence
	// reported without a reason cannot be acted on.
	Explanation string `json:"explanation,omitempty"`
}

// Valid reports whether a judgement can be recorded, and why not when it
// cannot.
func (j SemanticJudgement) Valid() error {
	if j.RequirementID == "" {
		return fmt.Errorf("judgement names no requirement")
	}
	if !j.Agrees && j.Explanation == "" {
		return fmt.Errorf("judgement for %q reports divergence without explaining it", j.RequirementID)
	}
	return nil
}

// IssuesFrom turns divergent judgements into drift issues, matching each to
// the question it answers so the issue carries the path and feature.
//
// Judgements that agree produce nothing: this reports drift, not a tally of
// everything checked. A judgement naming a requirement that was never asked
// about is skipped rather than recorded, because Roady cannot attach it to
// anything and a model returning ids it invented is a known failure.
func IssuesFrom(judgements []SemanticJudgement, questions []SemanticQuestion) []Issue {
	asked := make(map[string]SemanticQuestion, len(questions))
	for _, q := range questions {
		asked[q.RequirementID] = q
	}

	issues := make([]Issue, 0, len(judgements))
	for _, j := range judgements {
		if j.Agrees {
			continue
		}
		q, ok := asked[j.RequirementID]
		if !ok {
			continue
		}
		if err := j.Valid(); err != nil {
			continue
		}

		path := q.Citation
		if len(q.Paths) > 0 {
			path = q.Paths[0]
		}

		issues = append(issues, Issue{
			ID:          "drift-semantic-" + j.RequirementID,
			Type:        DriftTypeCode,
			Category:    CategorySemantic,
			Severity:    SeverityHigh,
			ComponentID: q.RequirementID,
			Message: fmt.Sprintf(
				"The implementation of %q no longer matches what the requirement asks for: %s",
				q.RequirementID, j.Explanation),
			Path: path,
			Hint: "Judged by a language model against the requirement text, not by comparing artifacts. " +
				"Read the explanation before acting: either the implementation is wrong, or the requirement has moved and the spec should say so.",
		})
	}
	return issues
}
