package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// One tool per CLI noun, with the CLI's verbs as actions: `roady plan approve`
// is roady_plan {action: approve}. Every project operation is reachable over
// MCP, and knowing one surface means knowing the other. Operations that
// configure the host rather than the project (setup, hook, mcp, completion,
// the interactive config wizard, doctor) are CLI-only.
//
// Decisions and destructive operations run only after the user confirms in
// their client (see confirm): an agent may ask to approve a plan or accept
// drift, but a person decides.

// scope is the project selector every noun tool takes.
type scope struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

// NounActions lists each noun tool's actions: the CLI's verbs for that noun.
// It is the one place the surface is declared, so a parity test can hold the
// CLI and MCP to each other.
var NounActions = map[string][]string{
	"roady_task":   {"start", "complete", "block", "unblock", "stop", "reopen", "verify", "check", "dispatch", "list", "renew"},
	"roady_plan":   {"get", "generate", "approve", "reject", "prune", "prioritize", "decompose", "import"},
	"roady_spec":   {"get", "add", "analyze", "explain", "import", "lock", "review", "validate"},
	"roady_drift":  {"detect", "accept", "explain", "semantic", "record"},
	"roady_audit":  {"verify", "trail"},
	"roady_state":  {"get", "rebuild"},
	"roady_policy": {"check"},
	"roady_git":    {"sync"},
	"roady_goal":   {"list", "add", "edit", "render"},
}

// SingleTools are CLI commands without verbs, served as one tool each.
var SingleTools = []string{"roady_init", "roady_next", "roady_status", "roady_query", "roady_capture"}

// GatedActions need the user's confirmation (approval.go).
var GatedActions = map[string][]string{
	"roady_plan":  {"approve", "reject", "prune"},
	"roady_spec":  {"analyze", "import", "lock"},
	"roady_drift": {"accept"},
	"roady_state": {"rebuild"},
}

// actionError reports an unknown action with the ones that exist.
func actionError(tool, action string) (any, error) {
	names := append([]string(nil), NounActions[tool]...)
	sort.Strings(names)
	return mcpErr(fmt.Sprintf("%s has no action %q; use one of: %s", tool, action, strings.Join(names, ", "))), nil
}

// --- roady_task ---------------------------------------------------------------

// TaskArgs is `roady task <action>`.
type TaskArgs struct {
	Action    string `json:"action" jsonschema:"required,description=start|complete|block|unblock|stop|reopen|verify|check|dispatch|list|renew (keep your claim; starting a task claims it with an expiring lease)"`
	TaskID    string `json:"task_id,omitempty" jsonschema:"description=The task (all actions but list)"`
	Evidence  string `json:"evidence,omitempty" jsonschema:"description=Proof for complete/verify, e.g. a commit"`
	Agent     string `json:"agent,omitempty" jsonschema:"description=Acting agent; for dispatch, the subagent taking the task"`
	SessionID string `json:"session_id,omitempty" jsonschema:"description=Session ID recorded in the audit trail"`
	DryRun    bool   `json:"dry_run,omitempty" jsonschema:"description=dispatch: build the brief without claiming"`
	Status    string `json:"status,omitempty" jsonschema:"description=list: ready (default), in_progress, blocked, unassigned or all"`
	Owner     string `json:"owner,omitempty" jsonschema:"description=list: only this owner's tasks"`
	Limit     int    `json:"limit,omitempty" jsonschema:"description=list: page size"`
	Offset    int    `json:"offset,omitempty" jsonschema:"description=list: page start"`
	scope
}

var taskTransitions = map[string]bool{"start": true, "complete": true, "block": true, "unblock": true, "stop": true, "reopen": true, "verify": true}

func (s *Server) handleTask(ctx context.Context, a TaskArgs) (any, error) {
	switch {
	case taskTransitions[a.Action]:
		return s.handleTransitionTask(ctx, TransitionTaskArgs{TaskID: a.TaskID, Event: a.Action, Evidence: a.Evidence,
			SessionID: a.SessionID, Agent: a.Agent, ProjectPath: a.ProjectPath, Project: a.Project})
	case a.Action == "check":
		return s.handleTaskCheck(ctx, TaskCheckArgs{TaskID: a.TaskID, ProjectPath: a.ProjectPath, Project: a.Project})
	case a.Action == "dispatch":
		return s.handleDispatchTask(ctx, DispatchTaskArgs{TaskID: a.TaskID, Agent: a.Agent, Session: a.SessionID,
			DryRun: a.DryRun, ProjectPath: a.ProjectPath, Project: a.Project})
	case a.Action == "renew":
		return s.handleRenewClaim(a)
	case a.Action == "list":
		return s.handleTasks(ctx, TasksArgs{Status: a.Status, Assignee: a.Owner, Limit: a.Limit, Offset: a.Offset,
			ProjectPath: a.ProjectPath, Project: a.Project})
	}
	return actionError("roady_task", a.Action)
}

// --- roady_plan ---------------------------------------------------------------

// PlanArgs is `roady plan <action>`.
type PlanArgs struct {
	Action      string `json:"action" jsonschema:"required,description=get|generate|approve|reject|prune|prioritize|decompose|import. approve, reject and prune need the user's confirmation."`
	Path        string `json:"path,omitempty" jsonschema:"description=import: the plan file"`
	FeatureID   string `json:"feature_id,omitempty" jsonschema:"description=import: attach to this existing feature"`
	Parallel    bool   `json:"parallel,omitempty" jsonschema:"description=import: leave steps independent"`
	IncludeDone bool   `json:"include_done,omitempty" jsonschema:"description=import: include checked-off steps"`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"description=import: preview only"`
	scope
}

func (s *Server) handlePlan(ctx context.Context, a PlanArgs) (any, error) {
	m := PlanMutateArgs{ProjectPath: a.ProjectPath, Project: a.Project}
	switch a.Action {
	case "get":
		return s.handleGetPlan(ctx, GetPlanArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "generate":
		return s.handleGeneratePlan(ctx, GeneratePlanArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "approve":
		return s.gated(ctx, a.scope, "plan approve", "Approve the current plan so its tasks can start?", func() (any, error) {
			return s.handleApprovePlan(ctx, ApprovePlanArgs{ProjectPath: a.ProjectPath, Project: a.Project})
		})
	case "reject":
		return s.gated(ctx, a.scope, "plan reject", "Reject the current plan? Its tasks cannot start until it is approved again.", func() (any, error) {
			return s.handlePlanReject(ctx, m)
		})
	case "prune":
		return s.gated(ctx, a.scope, "plan prune", "Remove plan tasks that no longer match anything in the spec?", func() (any, error) {
			return s.handlePlanPrune(ctx, m)
		})
	case "prioritize":
		return s.handleSuggestPriorities(ctx, SuggestPrioritiesArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "decompose":
		return s.handleSmartDecompose(ctx, SmartDecomposeArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "import":
		return s.handlePlanImport(ctx, PlanImportArgs{Path: a.Path, FeatureID: a.FeatureID, Parallel: a.Parallel,
			IncludeDone: a.IncludeDone, DryRun: a.DryRun, ProjectPath: a.ProjectPath, Project: a.Project})
	}
	return actionError("roady_plan", a.Action)
}

// --- roady_spec ---------------------------------------------------------------

// SpecArgs is `roady spec <action>`.
type SpecArgs struct {
	Action      string `json:"action" jsonschema:"required,description=get|add|analyze|explain|import|lock|review|validate. analyze, import and lock need the user's confirmation."`
	Title       string `json:"title,omitempty" jsonschema:"description=add: feature title"`
	Description string `json:"description,omitempty" jsonschema:"description=add: feature description"`
	Dir         string `json:"dir,omitempty" jsonschema:"description=analyze: directory of markdown documents"`
	Path        string `json:"path,omitempty" jsonschema:"description=import: one markdown document"`
	scope
}

func (s *Server) handleSpec(ctx context.Context, a SpecArgs) (any, error) {
	m := PlanMutateArgs{ProjectPath: a.ProjectPath, Project: a.Project}
	switch a.Action {
	case "get":
		return s.handleGetSpec(ctx, GetSpecArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "add":
		return s.handleAddFeature(ctx, AddFeatureArgs{Title: a.Title, Description: a.Description, ProjectPath: a.ProjectPath, Project: a.Project})
	case "analyze":
		return s.gated(ctx, a.scope, "spec analyze", fmt.Sprintf("Regenerate the spec from the documents in %q? This rewrites spec.yaml.", a.Dir), func() (any, error) {
			return s.handleSpecAnalyze(ctx, SpecAnalyzeArgs{Dir: a.Dir, ProjectPath: a.ProjectPath, Project: a.Project})
		})
	case "import":
		return s.gated(ctx, a.scope, "spec import", fmt.Sprintf("Replace the spec with the one in %q?", a.Path), func() (any, error) {
			return s.handleSpecImport(ctx, SpecImportArgs{Path: a.Path, ProjectPath: a.ProjectPath, Project: a.Project})
		})
	case "lock":
		return s.gated(ctx, a.scope, "spec lock", "Make the current spec the drift baseline? Differences from the old baseline stop being reported as drift.", func() (any, error) {
			return s.handleSpecLock(ctx, m)
		})
	case "explain":
		return s.handleExplainSpec(ctx, ExplainSpecArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "review":
		return s.handleReviewSpec(ctx, ReviewSpecArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "validate":
		return s.handleSpecValidate(ctx, m)
	}
	return actionError("roady_spec", a.Action)
}

// --- roady_drift --------------------------------------------------------------

// DriftArgs is `roady drift <action>`.
type DriftArgs struct {
	Action     string                    `json:"action" jsonschema:"required,description=detect|accept|explain|semantic|record. accept needs the user's confirmation. semantic returns a prompt; record stores the judgements on it."`
	Judgements []drift.SemanticJudgement `json:"judgements,omitempty" jsonschema:"description=record: one per requirement: requirement_id, agrees, explanation"`
	scope
}

func (s *Server) handleDrift(ctx context.Context, a DriftArgs) (any, error) {
	switch a.Action {
	case "detect":
		return s.handleDetectDrift(ctx, DetectDriftArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "semantic":
		return s.handleDetectDrift(ctx, DetectDriftArgs{Semantic: true, ProjectPath: a.ProjectPath, Project: a.Project})
	case "record":
		return s.handleRecordSemanticDrift(ctx, RecordSemanticDriftArgs{Judgements: a.Judgements, ProjectPath: a.ProjectPath, Project: a.Project})
	case "explain":
		return s.handleExplainDrift(ctx, ExplainDriftArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "accept":
		return s.gated(ctx, a.scope, "drift accept", "Accept the current drift as intended? The spec becomes the new baseline and these differences stop being reported.", func() (any, error) {
			return s.handleAcceptDrift(ctx, AcceptDriftArgs{ProjectPath: a.ProjectPath, Project: a.Project})
		})
	}
	return actionError("roady_drift", a.Action)
}

// --- roady_audit, roady_state, roady_policy, roady_git --------------------------

// AuditArgs is `roady audit <action>`.
type AuditArgs struct {
	Action   string `json:"action" jsonschema:"required,description=verify|trail"`
	Baseline string `json:"baseline,omitempty" jsonschema:"description=verify: git revision whose log must still be present (default HEAD)"`
	TaskID   string `json:"task_id,omitempty" jsonschema:"description=trail: for this task"`
	Agent    string `json:"agent,omitempty" jsonschema:"description=trail: for this agent"`
	Session  string `json:"session_id,omitempty" jsonschema:"description=trail: for this session"`
	Since    string `json:"since,omitempty" jsonschema:"description=trail: 7d, 2w or a date"`
	scope
}

func (s *Server) handleAudit(ctx context.Context, a AuditArgs) (any, error) {
	switch a.Action {
	case "verify":
		return s.handleAuditVerify(ctx, AuditVerifyArgs{Baseline: a.Baseline, ProjectPath: a.ProjectPath, Project: a.Project})
	case "trail":
		return s.handleAuditTrail(ctx, AuditTrailArgs{TaskID: a.TaskID, Agent: a.Agent, Session: a.Session, Since: a.Since,
			ProjectPath: a.ProjectPath, Project: a.Project})
	}
	return actionError("roady_audit", a.Action)
}

// StateArgs is `roady state <action>`.
type StateArgs struct {
	Action string `json:"action" jsonschema:"required,description=get|rebuild. rebuild needs the user's confirmation."`
	scope
}

func (s *Server) handleState(ctx context.Context, a StateArgs) (any, error) {
	switch a.Action {
	case "get":
		return s.handleGetState(ctx, GetStateArgs{ProjectPath: a.ProjectPath, Project: a.Project})
	case "rebuild":
		return s.gated(ctx, a.scope, "state rebuild", "Rebuild state.json from the event log, replacing the current file?", func() (any, error) {
			return s.handleStateRebuild(ctx, PlanMutateArgs{ProjectPath: a.ProjectPath, Project: a.Project})
		})
	}
	return actionError("roady_state", a.Action)
}

// VerbArgs is a noun with a single verb (`roady policy check`, `roady git sync`).
type VerbArgs struct {
	Action string `json:"action,omitempty" jsonschema:"description=The one action (default)"`
	scope
}

func (s *Server) handlePolicy(ctx context.Context, a VerbArgs) (any, error) {
	if a.Action != "" && a.Action != "check" {
		return actionError("roady_policy", a.Action)
	}
	return s.handleCheckPolicy(ctx, CheckPolicyArgs{ProjectPath: a.ProjectPath, Project: a.Project})
}

func (s *Server) handleGit(ctx context.Context, a VerbArgs) (any, error) {
	if a.Action != "" && a.Action != "sync" {
		return actionError("roady_git", a.Action)
	}
	return s.handleGitSync(ctx, GitSyncArgs{ProjectPath: a.ProjectPath, Project: a.Project})
}

// --- roady_goal ---------------------------------------------------------------

// GoalArgs is `roady goal <action>`.
type GoalArgs struct {
	Action      string   `json:"action" jsonschema:"required,description=list|add|edit|render (write ROADMAP.md from the goals)"`
	GoalID      string   `json:"goal_id,omitempty" jsonschema:"description=edit: the goal; add: its id (default goal-<title>)"`
	Title       *string  `json:"title,omitempty" jsonschema:"description=add: required"`
	Description *string  `json:"description,omitempty"`
	Horizon     *string  `json:"horizon,omitempty" jsonschema:"description=now, next or later"`
	Status      *string  `json:"status,omitempty" jsonschema:"description=idea (needs no features), planned, shipped or out_of_scope"`
	Milestone   *string  `json:"milestone,omitempty" jsonschema:"description=Release or checkpoint"`
	Features    []string `json:"features,omitempty" jsonschema:"description=Feature ids to link to the goal"`
	DryRun      bool     `json:"dry_run,omitempty"`
	Force       bool     `json:"force,omitempty" jsonschema:"description=render: replace a ROADMAP.md edited by hand (needs the user's confirmation)"`
	scope
}

func (s *Server) handleGoal(ctx context.Context, a GoalArgs) (any, error) {
	svc, err := s.servicesForPath(a.ProjectPath, a.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	sp, err := svc.Workspace.Repo.LoadSpec()
	if err != nil {
		return mcpErrCause("Failed to load the spec.", err), nil
	}
	e := application.GoalEdit{ID: a.GoalID, Title: a.Title, Description: a.Description, Horizon: a.Horizon,
		Status: a.Status, Milestone: a.Milestone, Features: a.Features}
	var doc application.CaptureDoc
	switch a.Action {
	case "list":
		plan, _ := svc.Workspace.Repo.LoadPlan()
		state, _ := svc.Workspace.Repo.LoadState()
		return application.BuildRoadmap(sp, plan, state), nil
	case "render":
		return s.renderRoadmap(ctx, svc.Workspace.Repo, sp, a)
	case "add":
		doc, _, err = application.AddGoalDoc(sp, e)
	case "edit":
		doc, _, err = application.EditGoalDoc(sp, e)
	default:
		return actionError("roady_goal", a.Action)
	}
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	result, err := svc.Capture.Capture(doc, application.CaptureOptions{Actor: "ai-agent", DryRun: a.DryRun, Origin: planning.OriginAI})
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to record the goal: %v", err)), nil
	}
	return result, nil
}

// renderRoadmap writes ROADMAP.md. Replacing a file edited by hand throws
// away what the edit said, so force needs the user's confirmation.
func (s *Server) renderRoadmap(ctx context.Context, repo *storage.FilesystemRepository, sp *spec.ProductSpec, a GoalArgs) (any, error) {
	if repo.IsSubProject() {
		return mcpErr("A sub-project's roadmap has no default file; render it with `roady goal render --out <file>`."), nil
	}
	path := filepath.Join(repo.Root(), application.RoadmapFile)
	if a.DryRun {
		return application.RenderRoadmapMarkdown(sp), nil
	}
	write := func() (any, error) {
		res, err := application.WriteRoadmap(path, sp, a.Force)
		if err != nil {
			return mcpErr(err.Error()), nil
		}
		return res, nil
	}
	if a.Force {
		return s.gated(ctx, a.scope, "goal render --force", "Replace ROADMAP.md, including edits made to it by hand, with the roadmap rendered from the goals?", write)
	}
	return write()
}

// handleRenewClaim keeps the caller's claim on an in-progress task alive.
func (s *Server) handleRenewClaim(a TaskArgs) (any, error) {
	svc, err := s.servicesForPath(a.ProjectPath, a.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	actor := a.Agent
	if actor == "" {
		actor = "ai-agent"
	}
	if a.SessionID != "" || a.Agent != "" {
		previous := svc.Audit.Provenance()
		svc.Audit.SetProvenance(previous.WithSession(a.SessionID, a.Agent))
		defer svc.Audit.SetProvenance(previous)
	}
	lease, err := svc.Task.RenewClaim(a.TaskID, actor)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return lease, nil
}
