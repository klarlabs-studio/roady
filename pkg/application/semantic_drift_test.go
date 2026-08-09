package application_test

import (
	"context"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func semanticFixture(t *testing.T) *storage.FilesystemRepository {
	t.Helper()
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID: "s", Version: "0.1.0",
		Features: []spec.Feature{{
			ID: "auth", Title: "Auth",
			Requirements: []spec.Requirement{
				{ID: "session-timeout", Title: "Session timeout", Description: "Sessions expire after 30 minutes of inactivity.",
					Source: spec.Source{Doc: "docs/auth.md", Line: 12}},
				{ID: "unimplemented", Title: "Nothing claims this", Description: "Audit every login."},
			},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePlan(&planning.Plan{
		ID: "p", SpecID: "s", ApprovalStatus: planning.ApprovalApproved,
		Tasks: []planning.Task{{ID: "task-session-timeout", Title: "Implement session timeout", FeatureID: "auth"}},
	}); err != nil {
		t.Fatal(err)
	}
	state := planning.NewExecutionState("p")
	state.TaskStates["task-session-timeout"] = planning.TaskResult{
		Status: planning.StatusDone, Path: "internal/auth/session.go", Evidence: []string{"abc1234"},
	}
	if err := repo.SaveState(state); err != nil {
		t.Fatal(err)
	}
	return repo
}

// Roady frames the question and does not answer it: the request must carry the
// requirement's own words, where the work landed, and where to check it.
func TestSemanticDriftFramesTheQuestion(t *testing.T) {
	repo := semanticFixture(t)

	req, questions, err := application.NewPromptService(repo).SemanticDrift(context.Background())
	if err != nil {
		t.Fatalf("SemanticDrift: %v", err)
	}

	// Only the implemented requirement is asked about. A requirement nothing
	// implements is structural drift the other detectors already report.
	if len(questions) != 1 || questions[0].RequirementID != "session-timeout" {
		t.Fatalf("questions = %v, want only session-timeout", questions)
	}

	for _, want := range []string{
		"Sessions expire after 30 minutes", // the requirement's own words
		"docs/auth.md:12",                  // where to check it
		"internal/auth/session.go",         // where the work landed
		"task-session-timeout",
	} {
		if !strings.Contains(req.Prompt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, req.Prompt)
		}
	}
	if req.WriteBack != "roady_drift_record_semantic" {
		t.Errorf("WriteBack = %q; the answer would have nowhere to go", req.WriteBack)
	}
	if req.ExpectedFormat == "" {
		t.Error("no ExpectedFormat: the caller cannot know what shape to return")
	}
}

func TestSemanticDriftNeedsSomethingImplemented(t *testing.T) {
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "s", Features: []spec.Feature{
		{ID: "f", Requirements: []spec.Requirement{{ID: "r", Description: "Something."}}},
	}}); err != nil {
		t.Fatal(err)
	}
	if err := repo.SavePlan(&planning.Plan{ID: "p", SpecID: "s"}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := application.NewPromptService(repo).SemanticDrift(context.Background()); err == nil {
		t.Error("expected a refusal when nothing implements anything")
	}
}

// The judgement is the caller's; Roady records divergence and ignores assent.
func TestRecordSemanticDriftStoresOnlyDivergence(t *testing.T) {
	repo := semanticFixture(t)
	svc := application.NewDriftService(repo, application.NewAuditService(repo), storage.NewCodebaseInspector(), application.NewPolicyService(repo))

	_, questions, err := application.NewPromptService(repo).SemanticDrift(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	report, err := svc.RecordSemanticDrift(context.Background(), []drift.SemanticJudgement{
		{RequirementID: "session-timeout", Agrees: false, Explanation: "The timeout is 30 days, not 30 minutes."},
	}, questions)
	if err != nil {
		t.Fatalf("RecordSemanticDrift: %v", err)
	}
	if len(report.Issues) != 1 {
		t.Fatalf("got %d issues, want 1", len(report.Issues))
	}
	if report.Issues[0].Category != drift.CategorySemantic {
		t.Errorf("Category = %q, want SEMANTIC", report.Issues[0].Category)
	}

	agreed, err := svc.RecordSemanticDrift(context.Background(), []drift.SemanticJudgement{
		{RequirementID: "session-timeout", Agrees: true},
	}, questions)
	if err != nil {
		t.Fatal(err)
	}
	if len(agreed.Issues) != 0 {
		t.Errorf("agreement produced %d issues, want 0", len(agreed.Issues))
	}
}
