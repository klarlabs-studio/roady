package drift

import (
	"strings"
	"testing"
)

func TestSemanticQuestionAnswerable(t *testing.T) {
	tests := []struct {
		name string
		q    SemanticQuestion
		want bool
	}{
		{"complete", SemanticQuestion{RequirementID: "r1", Requirement: "Sessions expire after 30 minutes.", TaskID: "task-r1"}, true},
		// Nothing claims to implement it — that is structural drift, already
		// reported, and asking a model invites a confident answer about nothing.
		{"no task", SemanticQuestion{RequirementID: "r1", Requirement: "Sessions expire."}, false},
		{"no requirement text", SemanticQuestion{RequirementID: "r1", TaskID: "task-r1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.Answerable(); got != tt.want {
				t.Errorf("Answerable() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A divergence reported without a reason cannot be acted on.
func TestSemanticJudgementValid(t *testing.T) {
	if err := (SemanticJudgement{RequirementID: "r1", Agrees: true}).Valid(); err != nil {
		t.Errorf("an agreeing judgement needs no explanation: %v", err)
	}
	if err := (SemanticJudgement{RequirementID: "r1", Agrees: false}).Valid(); err == nil {
		t.Error("divergence without an explanation was accepted")
	}
	if err := (SemanticJudgement{Agrees: true}).Valid(); err == nil {
		t.Error("a judgement naming no requirement was accepted")
	}
}

func TestIssuesFromReportsOnlyDivergence(t *testing.T) {
	questions := []SemanticQuestion{
		{RequirementID: "r1", FeatureID: "auth", Requirement: "Sessions expire after 30 minutes.",
			TaskID: "task-r1", Paths: []string{"internal/auth/session.go"}, Citation: "docs/auth.md:12"},
		{RequirementID: "r2", FeatureID: "auth", Requirement: "Passwords are hashed.", TaskID: "task-r2"},
	}
	judgements := []SemanticJudgement{
		{RequirementID: "r1", Agrees: false, Explanation: "The timeout is 30 days, not 30 minutes."},
		{RequirementID: "r2", Agrees: true},
	}

	issues := IssuesFrom(judgements, questions)

	if len(issues) != 1 {
		t.Fatalf("got %d issues, want 1 (agreement is not drift)", len(issues))
	}
	got := issues[0]
	if got.Category != CategorySemantic {
		t.Errorf("Category = %q, want %q", got.Category, CategorySemantic)
	}
	if got.ComponentID != "r1" {
		t.Errorf("ComponentID = %q, want r1", got.ComponentID)
	}
	if got.Path != "internal/auth/session.go" {
		t.Errorf("Path = %q, want the implementation path", got.Path)
	}
	if !strings.Contains(got.Message, "30 days") {
		t.Errorf("the explanation did not survive into the message: %q", got.Message)
	}
	// The hint must say the verdict came from a model, not from comparing
	// artifacts, so nobody treats it as mechanically established.
	if !strings.Contains(got.Hint, "language model") {
		t.Errorf("hint does not disclose how the verdict was reached: %q", got.Hint)
	}
}

// A model returning ids it invented is a known failure; those cannot be
// attached to anything and must not become issues.
func TestIssuesFromSkipsUnaskedRequirements(t *testing.T) {
	questions := []SemanticQuestion{{RequirementID: "r1", Requirement: "x", TaskID: "t"}}
	judgements := []SemanticJudgement{
		{RequirementID: "invented", Agrees: false, Explanation: "made up"},
		{RequirementID: "r1", Agrees: false, Explanation: "real"},
	}

	issues := IssuesFrom(judgements, questions)

	if len(issues) != 1 || issues[0].ComponentID != "r1" {
		t.Fatalf("got %d issues %v, want only the one that was asked about", len(issues), issues)
	}
}

func TestIssuesFromSkipsUnexplainedDivergence(t *testing.T) {
	questions := []SemanticQuestion{{RequirementID: "r1", Requirement: "x", TaskID: "t"}}
	judgements := []SemanticJudgement{{RequirementID: "r1", Agrees: false}}

	if issues := IssuesFrom(judgements, questions); len(issues) != 0 {
		t.Errorf("got %d issues, want 0: an unexplained divergence is not actionable", len(issues))
	}
}

func TestIssuesFromEmptyInput(t *testing.T) {
	if issues := IssuesFrom(nil, nil); len(issues) != 0 {
		t.Errorf("got %d issues from no judgements", len(issues))
	}
}
