package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/prompt"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
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
		WriteBack: "roady_plan_update",
		Guidance:  "Produce the tasks yourself, then call roady_plan_update with them to store the plan.",
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
		Guidance:  "Answer this yourself. To record that the drift is intentional, use roady_drift_accept.",
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

// SemanticDrift builds the question every other drift check cannot ask.
//
// The structural detectors decide by comparing artifacts: a task is missing,
// an id is orphaned, a file does not exist. None of them can tell whether code
// that exists still does what the requirement asked for — "sessions expire
// after 30 minutes" is structurally satisfied by an implementation that
// expires them after thirty days. Answering that needs a reader.
//
// So Roady assembles the pairing and hands it over: the requirement's own
// words, where the work landed, and the doc:line to check against. It does not
// judge. The caller's model has the working tree in view and Roady does not,
// which is the same reason PatchDrift returns a request rather than a diff.
//
// Only requirements something claims to implement are asked about. A
// requirement with no task is structural drift the other detectors already
// report, and asking a model about absent code invites a confident answer
// about nothing.
func (s *PromptService) SemanticDrift(_ context.Context) (*prompt.Request, []drift.SemanticQuestion, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, nil, err
	}

	productSpec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, nil, fmt.Errorf("load spec: %w", err)
	}
	if productSpec == nil {
		return nil, nil, fmt.Errorf("no spec found; run 'roady spec analyze' first")
	}
	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return nil, nil, fmt.Errorf("no plan found; run 'roady plan generate' first")
	}
	state, _ := s.repo.LoadState()

	questions := buildSemanticQuestions(productSpec, plan, state)
	if len(questions) == 0 {
		return nil, nil, fmt.Errorf("no implemented requirements to judge: semantic drift compares a requirement against the work that claims to satisfy it, and nothing here does yet")
	}

	var b strings.Builder
	b.WriteString("For each requirement below, decide whether the implementation still means what the requirement says. " +
		"Read the cited source and the listed paths before answering — structural presence is not agreement. " +
		"Answer only about the requirements listed; do not invent ids.\n\n")
	for _, q := range questions {
		fmt.Fprintf(&b, "- requirement_id: %s\n  feature: %s\n  requirement: %s\n", q.RequirementID, q.FeatureID, q.Requirement)
		if q.Citation != "" {
			fmt.Fprintf(&b, "  stated at: %s\n", q.Citation)
		}
		fmt.Fprintf(&b, "  implemented by: %s (%s)\n", q.TaskID, q.Status)
		if len(q.Paths) > 0 {
			fmt.Fprintf(&b, "  paths: %s\n", strings.Join(q.Paths, ", "))
		}
		if len(q.Evidence) > 0 {
			fmt.Fprintf(&b, "  evidence: %s\n", strings.Join(q.Evidence, ", "))
		}
	}

	return &prompt.Request{
		Operation: prompt.OpSemanticDrift,
		System: "You judge whether an implementation still satisfies a written requirement. " +
			"You are the reader Roady cannot be: it compares artifacts, you compare meaning. " +
			"Say a requirement is satisfied only if the behaviour matches what it asks for, and when it does not, say concretely what differs.",
		Prompt: b.String(),
		ExpectedFormat: `A JSON array of judgements: ` +
			`[{"requirement_id": "...", "agrees": true|false, "explanation": "required when agrees is false"}]`,
		WriteBack: "roady_drift_record_semantic",
		Guidance: "Run this yourself against the working tree, then send the judgements to roady_drift_record_semantic. " +
			"Divergences become drift issues; agreement records nothing.",
	}, questions, nil
}

// buildSemanticQuestions pairs each requirement with the task claiming to
// implement it. The heuristic planner names tasks task-<requirement-id>, which
// is the same mapping the dispatch brief uses to recover intent.
func buildSemanticQuestions(productSpec *spec.ProductSpec, plan *planning.Plan, state *planning.ExecutionState) []drift.SemanticQuestion {
	tasks := make(map[string]planning.Task, len(plan.Tasks))
	for _, t := range plan.Tasks {
		tasks[t.ID] = t
	}

	questions := make([]drift.SemanticQuestion, 0)
	for _, feature := range productSpec.Features {
		// A feature with no requirements is the shape `spec analyze` produces
		// from a document whose headings carry the intent and whose bullets
		// become prose. The heuristic planner names that task task-<feature-id>,
		// so semantic drift asks about the feature itself rather than finding
		// nothing to ask about — which is what it did until this was run
		// against a spec built the ordinary way.
		if len(feature.Requirements) == 0 {
			text := strings.TrimSpace(feature.Description)
			if text == "" {
				text = strings.TrimSpace(feature.Title)
			}
			q := drift.SemanticQuestion{
				RequirementID: feature.ID,
				FeatureID:     feature.ID,
				Requirement:   text,
			}
			if !feature.Source.IsZero() {
				q.Citation = fmt.Sprintf("%s:%d", feature.Source.Doc, feature.Source.Line)
			}
			applyTaskContext(&q, tasks, state, "task-"+feature.ID)
			if q.Answerable() {
				questions = append(questions, q)
			}
			continue
		}

		for _, req := range feature.Requirements {
			text := strings.TrimSpace(req.Description)
			if text == "" {
				text = strings.TrimSpace(req.Title)
			}

			q := drift.SemanticQuestion{
				RequirementID: req.ID,
				FeatureID:     feature.ID,
				Requirement:   text,
			}
			if !req.Source.IsZero() {
				q.Citation = fmt.Sprintf("%s:%d", req.Source.Doc, req.Source.Line)
			}

			applyTaskContext(&q, tasks, state, "task-"+req.ID)

			if q.Answerable() {
				questions = append(questions, q)
			}
		}
	}
	return questions
}

// applyTaskContext attaches the task claiming to implement a requirement, and
// what execution recorded about it.
func applyTaskContext(q *drift.SemanticQuestion, tasks map[string]planning.Task, state *planning.ExecutionState, taskID string) {
	task, ok := tasks[taskID]
	if !ok {
		return
	}
	q.TaskID = task.ID
	if state == nil {
		return
	}
	if result, has := state.TaskStates[task.ID]; has {
		q.Status = string(result.Status)
		if result.Path != "" {
			q.Paths = []string{result.Path}
		}
		q.Evidence = result.Evidence
	}
}
