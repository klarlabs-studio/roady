package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/prompt"
)

// PromptService assembles the context a language model needs, and hands it
// back rather than running inference.
//
// It replaces AIPlanningService's provider calls. The prompt text is carried
// over unchanged — the value was always in knowing which parts of the spec,
// plan, and drift report matter for each question, and that judgement is
// still Roady's.
type PromptService struct {
	repo domain.WorkspaceRepository
}

func NewPromptService(repo domain.WorkspaceRepository) *PromptService {
	return &PromptService{repo: repo}
}

// ErrAIDisabled is returned when project policy forbids model-assisted work.
// The check is kept even though Roady no longer calls a model: a team that
// set allow_ai=false meant "do not use a model on this project", and that
// intent does not change because the inference moved to the caller.
var ErrAIDisabled = fmt.Errorf("model-assisted operations are disabled by project policy (allow_ai: false in .roady/policy.yaml)")

func (s *PromptService) checkPolicy() error {
	cfg, err := s.repo.LoadPolicy()
	if err != nil {
		return err
	}
	if cfg != nil && !cfg.AllowAI {
		return ErrAIDisabled
	}
	return nil
}

// ExplainSpec builds a request for a plain-language walkthrough of the spec.
func (s *PromptService) ExplainSpec(_ context.Context) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}

	spec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, fmt.Errorf("no spec found; run 'roady spec analyze' or 'roady init' first")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Provide a high-level architectural walkthrough and explanation of this "+
		"software specification. Explain 'What' we are building and 'Why' based on the "+
		"features and requirements.\n\nSpec: %s\n\nFeatures:\n", spec.Title)
	for _, f := range spec.Features {
		fmt.Fprintf(&b, "- %s: %s\n", f.Title, f.Description)
		for _, r := range f.Requirements {
			fmt.Fprintf(&b, "  * %s: %s\n", r.Title, r.Description)
		}
	}

	return &prompt.Request{
		Operation: prompt.OpExplainSpec,
		System:    "You are an expert technical lead. Provide a clear, concise, and professional explanation.",
		Prompt:    b.String(),
		Guidance:  "Answer this yourself and show the explanation to the user. Roady stores nothing.",
	}, nil
}

// ReviewSpec builds a request for a critique of the spec.
func (s *PromptService) ReviewSpec(_ context.Context) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}

	spec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, fmt.Errorf("no spec found; run 'roady spec analyze' or 'roady init' first")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Review this software specification for gaps, ambiguity, and risk.\n\n"+
		"Spec: %s\n%s\n\nFeatures:\n", spec.Title, spec.Description)
	for _, f := range spec.Features {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", f.ID, f.Title, f.Description)
		for _, r := range f.Requirements {
			fmt.Fprintf(&b, "  * [%s] %s: %s\n", r.ID, r.Title, r.Description)
		}
	}

	return &prompt.Request{
		Operation: prompt.OpReviewSpec,
		System:    "You are a meticulous staff engineer reviewing a specification before work begins.",
		Prompt:    b.String(),
		ExpectedFormat: `{"score": <0-100>, "summary": "...", "findings": [` +
			`{"category": "gap|ambiguity|risk|scope", "severity": "low|medium|high", ` +
			`"feature_id": "...", "title": "...", "suggestion": "..."}]}`,
		Guidance: "Answer this yourself and present the findings. Roady stores nothing.",
	}, nil
}

// DecomposeSpec builds a request to turn the spec into a task DAG.
func (s *PromptService) DecomposeSpec(_ context.Context) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}

	spec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, err
	}
	if spec == nil {
		return nil, fmt.Errorf("no spec found; run 'roady spec analyze' or 'roady init' first")
	}

	var b strings.Builder
	b.WriteString("Decompose this specification into concrete, implementable tasks.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Each task must be independently completable.\n")
	b.WriteString("- Express ordering through depends_on, using task IDs.\n")
	b.WriteString("- Do not create cycles.\n")
	b.WriteString("- Set feature_id to the feature the task serves.\n\n")
	fmt.Fprintf(&b, "Spec: %s\n%s\n\nFeatures:\n", spec.Title, spec.Description)
	for _, f := range spec.Features {
		fmt.Fprintf(&b, "- [%s] %s: %s\n", f.ID, f.Title, f.Description)
		for _, r := range f.Requirements {
			fmt.Fprintf(&b, "  * [%s] %s: %s (priority: %s, estimate: %s)\n",
				r.ID, r.Title, r.Description, r.Priority, r.Estimate)
		}
	}

	return &prompt.Request{
		Operation: prompt.OpDecomposeSpec,
		System:    "You are a senior engineer breaking a specification into an executable plan.",
		Prompt:    b.String(),
		ExpectedFormat: `{"tasks": [{"id": "task-...", "title": "...", "description": "...", ` +
			`"priority": "low|medium|high", "estimate": "4h", "feature_id": "...", ` +
			`"depends_on": ["task-..."]}]}`,
		WriteBack: "roady_update_plan",
		Guidance:  "Produce the tasks yourself, then call roady_update_plan with them to store the plan.",
	}, nil
}

// SuggestPriorities builds a request to re-prioritise the current plan.
func (s *PromptService) SuggestPriorities(_ context.Context) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}

	plan, err := s.repo.LoadPlan()
	if err != nil {
		return nil, err
	}
	if plan == nil || len(plan.Tasks) == 0 {
		return nil, fmt.Errorf("no plan found; run 'roady plan generate' first")
	}

	state, _ := s.repo.LoadState()

	var b strings.Builder
	b.WriteString("Suggest priority changes for these tasks, considering dependency order, " +
		"what is already in flight, and what unblocks the most work.\n\nTasks:\n")
	for _, t := range plan.Tasks {
		status := "pending"
		if state != nil {
			status = string(state.GetTaskStatus(t.ID))
		}
		fmt.Fprintf(&b, "- [%s] %s (priority: %s, status: %s, depends_on: %v)\n",
			t.ID, t.Title, t.Priority, status, t.DependsOn)
	}

	return &prompt.Request{
		Operation:      prompt.OpSuggestPriorities,
		System:         "You are a delivery lead sequencing work to maximise flow.",
		Prompt:         b.String(),
		ExpectedFormat: `{"suggestions": [{"task_id": "...", "current_priority": "...", "suggested_priority": "low|medium|high", "reason": "..."}]}`,
		Guidance:       "Decide the priorities yourself and present them. Applying them is a plan edit.",
	}, nil
}

// QueryProject builds a request answering a free-form question about the
// project, with spec, plan, and state as context.
func (s *PromptService) QueryProject(_ context.Context, question string) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(question) == "" {
		return nil, fmt.Errorf("a question is required")
	}

	spec, _ := s.repo.LoadSpec()
	plan, _ := s.repo.LoadPlan()
	state, _ := s.repo.LoadState()

	var b strings.Builder
	fmt.Fprintf(&b, "Answer this question about the project:\n\n%s\n\n---\nProject context:\n", question)

	if spec != nil {
		fmt.Fprintf(&b, "\nSpec: %s\n%s\n", spec.Title, spec.Description)
		for _, f := range spec.Features {
			fmt.Fprintf(&b, "- %s: %s\n", f.Title, f.Description)
		}
	}
	if plan != nil {
		b.WriteString("\nTasks:\n")
		for _, t := range plan.Tasks {
			status := "pending"
			if state != nil {
				status = string(state.GetTaskStatus(t.ID))
			}
			fmt.Fprintf(&b, "- [%s] %s (%s)\n", t.ID, t.Title, status)
		}
	}

	return &prompt.Request{
		Operation: prompt.OpQueryProject,
		System:    "You answer questions about a software project using only the supplied context. Say so when the context does not contain the answer.",
		Prompt:    b.String(),
		Guidance:  "Answer this yourself from the context above.",
	}, nil
}

// ExplainDrift builds a request explaining a drift report in plain language.
func (s *PromptService) ExplainDrift(_ context.Context, report *drift.Report) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}
	if report == nil || len(report.Issues) == 0 {
		return nil, fmt.Errorf("no drift issues to explain")
	}

	// Most severe first, so the explanation leads with what matters.
	issues := make([]drift.Issue, len(report.Issues))
	copy(issues, report.Issues)
	sort.SliceStable(issues, func(i, j int) bool {
		return driftSeverityRank(issues[i].Severity) < driftSeverityRank(issues[j].Severity)
	})

	var b strings.Builder
	b.WriteString("Explain this drift between the project's intent and its current state. " +
		"For each issue say what it means in practice and what to do about it.\n\nIssues:\n")
	for _, i := range issues {
		fmt.Fprintf(&b, "- [%s/%s] %s: %s\n", i.Severity, i.Type, i.ComponentID, i.Message)
		if i.Hint != "" {
			fmt.Fprintf(&b, "  hint: %s\n", i.Hint)
		}
	}

	return &prompt.Request{
		Operation: prompt.OpExplainDrift,
		System:    "You explain divergence between a plan and reality to the engineer who has to fix it.",
		Prompt:    b.String(),
		Guidance:  "Answer this yourself. To record that the drift is intentional, use roady_accept_drift.",
	}, nil
}

// PatchDrift builds a request asking for a patch that closes the drift,
// rather than prose explaining it.
//
// The distinction is what the caller does next. ExplainDrift produces
// something a person reads; this produces something applied and reviewed, so
// it names the file each issue points at and asks for a unified diff. Roady
// frames the question and does not answer it — the caller's model has the
// working tree in view and Roady does not.
func (s *PromptService) PatchDrift(_ context.Context, report *drift.Report) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}
	if report == nil || len(report.Issues) == 0 {
		return nil, fmt.Errorf("no drift issues to patch")
	}

	issues := make([]drift.Issue, len(report.Issues))
	copy(issues, report.Issues)
	sort.SliceStable(issues, func(i, j int) bool {
		return driftSeverityRank(issues[i].Severity) < driftSeverityRank(issues[j].Severity)
	})

	// Only code drift is closable by a diff. Everything else is a planning
	// decision: intent drift means the spec moved, plan drift means the task
	// list needs regenerating, staleness means the plan was abandoned. Handing
	// any of those to a model asking for a patch invites it to rewrite the
	// specification or the plan to match the code — the exact failure Roady
	// exists to catch.
	var patchable, advisory []drift.Issue
	for _, i := range issues {
		if i.Type == drift.DriftTypeCode {
			patchable = append(patchable, i)
			continue
		}
		advisory = append(advisory, i)
	}

	if len(patchable) == 0 {
		return nil, fmt.Errorf("the drift found is not patchable: only code drift can be closed by a diff. Spec drift means intent moved ('roady drift accept' or update the spec); plan drift means the task list is out of date ('roady plan generate'); staleness means the plan was abandoned. Use 'roady drift explain' to review it")
	}

	var b strings.Builder
	b.WriteString("Produce a patch that closes this drift between the project's intent and its code.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Return a unified diff and nothing else.\n")
	b.WriteString("- Change code to match the stated intent, never the reverse.\n")
	b.WriteString("- Leave anything you cannot close confidently; a partial patch beats a wrong one.\n\n")
	b.WriteString("Issues:\n")
	for _, i := range patchable {
		fmt.Fprintf(&b, "- [%s/%s] %s: %s\n", i.Severity, i.Type, i.ComponentID, i.Message)
		if i.Path != "" {
			fmt.Fprintf(&b, "  file: %s\n", i.Path)
		}
		if i.Hint != "" {
			fmt.Fprintf(&b, "  hint: %s\n", i.Hint)
		}
	}

	if len(advisory) > 0 {
		b.WriteString("\nNot for patching — these are decisions, listed for context only:\n")
		for _, i := range advisory {
			fmt.Fprintf(&b, "- [%s/%s] %s\n", i.Severity, i.Type, i.Message)
		}
	}

	return &prompt.Request{
		Operation:      prompt.OpPatchDrift,
		System:         "You write minimal, reviewable patches. You change code to match intent, never intent to match code.",
		Prompt:         b.String(),
		ExpectedFormat: "A unified diff (git apply compatible), and nothing else.",
		Guidance:       "Produce the diff yourself and open it as a pull request. Once merged, re-run 'roady drift detect' to confirm it closed.",
	}, nil
}

func driftSeverityRank(s drift.Severity) int {
	switch s {
	case drift.SeverityCritical:
		return 0
	case drift.SeverityHigh:
		return 1
	case drift.SeverityMedium:
		return 2
	default:
		return 3
	}
}
