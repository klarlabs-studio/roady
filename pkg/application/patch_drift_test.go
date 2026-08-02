package application

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/prompt"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func newPromptSvc(t *testing.T) *PromptService {
	t.Helper()
	repo := storage.NewFilesystemRepository(t.TempDir())
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return NewPromptService(repo)
}

func TestPatchDriftBuildsADiffRequest(t *testing.T) {
	report := &drift.Report{Issues: []drift.Issue{
		{Type: drift.DriftTypeCode, Category: drift.CategoryImplementation, Severity: drift.SeverityHigh,
			ComponentID: "task-1", Message: "done task has no file", Path: "internal/auth/signup.go"},
	}}

	req, err := newPromptSvc(t).PatchDrift(t.Context(), report)
	if err != nil {
		t.Fatalf("PatchDrift: %v", err)
	}

	if req.Operation != prompt.OpPatchDrift {
		t.Errorf("operation = %q", req.Operation)
	}
	if !strings.Contains(req.Prompt, "internal/auth/signup.go") {
		t.Error("the file an issue points at should be in the prompt")
	}
	if !strings.Contains(req.ExpectedFormat, "unified diff") {
		t.Errorf("expected a diff format, got %q", req.ExpectedFormat)
	}
	if !strings.Contains(req.Prompt, "never the reverse") {
		t.Error("the prompt must forbid rewriting intent to match code")
	}
}

// TestPatchDriftRefusesDecisionDrift is the safety property. Intent and
// staleness drift are decisions; offering a patch invites a model to rewrite
// the spec to match the code, which is the failure Roady exists to catch.
func TestPatchDriftRefusesDecisionDrift(t *testing.T) {
	for _, issue := range []drift.Issue{
		{Type: drift.DriftTypeSpec, Category: drift.CategoryMismatch, Severity: drift.SeverityMedium, Message: "spec changed"},
		{Type: drift.DriftTypePlan, Category: drift.CategoryStale, Severity: drift.SeverityHigh, Message: "plan is stale"},
		// A requirement missing from the plan is fixed by regenerating the
		// plan, not by a diff. Treating it as patchable invited a model to
		// edit plan.json to match the code.
		{Type: drift.DriftTypePlan, Category: drift.CategoryMissing, Severity: drift.SeverityHigh, Message: "requirement missing from plan"},
		{Type: drift.DriftTypePolicy, Category: drift.CategoryViolation, Severity: drift.SeverityHigh, Message: "WIP exceeded"},
	} {
		_, err := newPromptSvc(t).PatchDrift(t.Context(), &drift.Report{Issues: []drift.Issue{issue}})
		if err == nil {
			t.Errorf("%s/%s should not be patchable", issue.Type, issue.Category)
			continue
		}
		if !strings.Contains(err.Error(), "not patchable") {
			t.Errorf("unexpected error: %v", err)
		}
	}
}

func TestPatchDriftListsDecisionDriftAsContext(t *testing.T) {
	report := &drift.Report{Issues: []drift.Issue{
		{Type: drift.DriftTypeCode, Severity: drift.SeverityHigh, ComponentID: "t1", Message: "missing file"},
		{Type: drift.DriftTypeSpec, Severity: drift.SeverityMedium, Message: "spec changed since lock"},
	}}

	req, err := newPromptSvc(t).PatchDrift(t.Context(), report)
	if err != nil {
		t.Fatalf("PatchDrift: %v", err)
	}
	if !strings.Contains(req.Prompt, "Not for patching") {
		t.Error("decision drift should still appear as context")
	}
	if !strings.Contains(req.Prompt, "spec changed since lock") {
		t.Error("the advisory issue text should be present")
	}
}

func TestPatchDriftEmptyReport(t *testing.T) {
	if _, err := newPromptSvc(t).PatchDrift(t.Context(), &drift.Report{}); err == nil {
		t.Error("expected an error for an empty report")
	}
	if _, err := newPromptSvc(t).PatchDrift(t.Context(), nil); err == nil {
		t.Error("expected an error for a nil report")
	}
}
