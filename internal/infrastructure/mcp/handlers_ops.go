package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// capSlice returns at most maxResponseItems elements from a string slice.
func capSlice(s []string) []string {
	if len(s) > maxResponseItems {
		return s[:maxResponseItems]
	}
	return s
}

type InitArgs struct {
	Name        string `json:"name" jsonschema:"description=The name of the project"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type GetSpecArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type GetPlanArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type GetStateArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type GeneratePlanArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type ApprovePlanArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type ExplainSpecArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type ExplainDriftArgs struct {
	Patch       bool   `json:"patch,omitempty" jsonschema:"description=Ask for a unified diff that closes the drift instead of an explanation. Intent and staleness drift are excluded: they are decisions about what to build, not changes a diff can make."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type AcceptDriftArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type CheckPolicyArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type GitSyncArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type SuggestPrioritiesArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type ReviewSpecArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type GetSnapshotArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

// TasksArgs filters the task list behind roady_task list.
type TasksArgs struct {
	Status      string `json:"status,omitempty" jsonschema:"description=Which tasks to return: ready (unlocked + pending), in_progress, blocked, unassigned, or all. Defaults to ready.,enum=ready,enum=in_progress,enum=blocked,enum=unassigned,enum=all"`
	Assignee    string `json:"assignee,omitempty" jsonschema:"description=Only return tasks assigned to this person or agent. Matched case-insensitively. Ignored when status is unassigned."`
	Limit       int    `json:"limit,omitempty" jsonschema:"description=Maximum tasks to return. Defaults to 50 and is capped at 200."`
	Offset      int    `json:"offset,omitempty" jsonschema:"description=Number of tasks to skip, for paging through a large plan."`
	Detail      bool   `json:"detail,omitempty" jsonschema:"description=Include each task's full description. Off by default because descriptions dominate the payload."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

// AuditTrailArgs selects what to build an evidence trail about. Exactly one
// of TaskID, Agent, or Session identifies the subject; combining TaskID with
// Agent narrows to what that agent did to that task.
type AuditTrailArgs struct {
	TaskID      string `json:"task_id,omitempty" jsonschema:"description=Build the trail for this task."`
	Agent       string `json:"agent,omitempty" jsonschema:"description=Only include events from this agent (e.g. claude-code)."`
	Session     string `json:"session_id,omitempty" jsonschema:"description=Only include events from this session ID."`
	Since       string `json:"since,omitempty" jsonschema:"description=Only include events since this point: 7d, 2w, or an absolute date like 2026-07-01."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type SmartDecomposeArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type SpecAnalyzeArgs struct {
	Dir         string `json:"dir" jsonschema:"description=Directory of markdown documents to analyze, relative to the project or absolute. Example: docs/"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleSpecAnalyze(ctx context.Context, args SpecAnalyzeArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}

	dir := strings.TrimSpace(args.Dir)
	if dir == "" {
		return mcpErr("A directory is required: pass dir, e.g. \"docs/\"."), nil
	}
	// Resolve against the project rather than the server's working directory,
	// which is routinely a different repository entirely.
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(s.rootFor(args.ProjectPath), dir)
	}

	productSpec, err := svc.Spec.AnalyzeDirectory(dir)
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to analyze %s: %v", dir, err)), nil
	}

	features := make([]map[string]any, 0, len(productSpec.Features))
	for _, f := range productSpec.Features {
		features = append(features, map[string]any{
			"id":           f.ID,
			"title":        f.Title,
			"requirements": len(f.Requirements),
		})
	}
	return map[string]any{
		"title":    productSpec.Title,
		"features": features,
		"count":    len(productSpec.Features),
		"hint":     "The spec is written to .roady/spec.yaml. Run roady_plan with action generate to turn it into tasks.",
	}, nil
}

type PlanMutateArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handlePlanPrune(ctx context.Context, args PlanMutateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	before, _ := svc.Plan.GetPlan()
	countBefore := 0
	if before != nil {
		countBefore = len(before.Tasks)
	}
	if err := svc.Plan.PrunePlan(); err != nil {
		return mcpErr(fmt.Sprintf("Failed to prune the plan: %v", err)), nil
	}
	after, _ := svc.Plan.GetPlan()
	countAfter := 0
	if after != nil {
		countAfter = len(after.Tasks)
	}
	return map[string]any{
		"pruned":         countBefore - countAfter,
		"tasks_retained": countAfter,
		"message":        fmt.Sprintf("Pruned %d task(s); %d retained.", countBefore-countAfter, countAfter),
	}, nil
}

func (s *Server) handlePlanReject(ctx context.Context, args PlanMutateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if err := svc.Plan.RejectPlan(); err != nil {
		return mcpErr(fmt.Sprintf("Failed to reject the plan: %v", err)), nil
	}
	return "Plan rejected. It cannot be executed until it is approved again.", nil
}

func (s *Server) handleAuditVerify(ctx context.Context, args AuditVerifyArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	// Deliberately the same verifier the CLI uses. EventSourcedAuditService
	// carries a second implementation that still checks the log as a strict
	// linear sequence, so it reports tampering for the branch-and-merge shape
	// concurrent appends legitimately produce — the case AuditService was
	// fixed for in 0.14.0. Two verifiers disagreeing about whether an audit
	// chain is intact is worse than having one, especially in the subsystem
	// whose entire value is being trustworthy.
	violations, err := svc.Workspace.Audit.VerifyIntegrity()
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to verify the audit chain: %v", err)), nil
	}
	// The chain cannot see a truncated tail; the committed log can.
	ref := args.Baseline
	if ref == "" {
		ref = application.DefaultAuditBaseline
	}
	eventsPath, err := svc.Workspace.Repo.ResolvePath(storage.EventsFile)
	if err != nil {
		return mcpErrCause("Failed to locate the audit log.", err), nil
	}
	baseline, err := svc.Workspace.Audit.VerifyAgainstCommitted(svc.Workspace.Repo.Root(), eventsPath, ref)
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to compare the audit log with %s: %v", ref, err)), nil
	}
	for _, v := range baseline.Violations {
		violations = append(violations, v.Message)
	}
	// Reported as data rather than an error: a broken chain is a finding the
	// caller must act on, not a failed call.
	return map[string]any{
		"intact":     len(violations) == 0,
		"violations": violations,
		"count":      len(violations),
		"baseline":   baseline,
	}, nil
}

// AuditVerifyArgs adds the committed baseline to the usual project selectors.
type AuditVerifyArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
	Baseline    string `json:"baseline,omitempty" jsonschema:"description=Git revision whose committed events.jsonl must still be fully present (default: HEAD; use the protected branch in CI)"`
}

func (s *Server) handleSpecValidate(ctx context.Context, args PlanMutateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	productSpec, err := svc.Spec.GetSpec()
	if err != nil || productSpec == nil {
		return mcpErr("Failed to load the spec. Ensure the project is initialized."), nil
	}
	problems := productSpec.Validate()
	messages := make([]string, 0, len(problems))
	for _, p := range problems {
		messages = append(messages, p.Error())
	}
	return map[string]any{
		"valid":    len(messages) == 0,
		"problems": messages,
		"count":    len(messages),
	}, nil
}

type SpecImportArgs struct {
	Path        string `json:"path" jsonschema:"description=Markdown file to import as the spec, relative to the project or absolute."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleSpecImport(ctx context.Context, args SpecImportArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	path := strings.TrimSpace(args.Path)
	if path == "" {
		return mcpErr("A path is required: pass path, e.g. \"docs/spec.md\"."), nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.rootFor(args.ProjectPath), path)
	}
	productSpec, err := svc.Spec.ImportFromMarkdown(path)
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to import %s: %v", path, err)), nil
	}
	return map[string]any{
		"title":    productSpec.Title,
		"features": len(productSpec.Features),
	}, nil
}

func (s *Server) handleStateRebuild(ctx context.Context, args PlanMutateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	rebuilt, result, err := application.NewStateRebuildService(svc.Workspace.Repo).Rebuild()
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to rebuild state: %v", err)), nil
	}
	out := map[string]any{"tasks": len(rebuilt.TaskStates)}
	if result != nil {
		out["result"] = result
	}
	return out, nil
}

func (s *Server) handleSpecLock(ctx context.Context, args PlanMutateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	result, err := svc.Spec.WriteLock()
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return map[string]any{
		"spec_id":       result.SpecID,
		"lock_updated":  result.LockUpdated,
		"state_updated": result.StateUpdated,
		"changed":       result.Changed(),
	}, nil
}

func (s *Server) handleGitSync(ctx context.Context, args GitSyncArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	results, err := svc.Git.SyncMarkers(10)
	if err != nil {
		return mcpErrCause("Failed to sync git markers. Ensure you are in a git repository with commit history.", err), nil
	}
	return results, nil
}

func (s *Server) handleExplainSpec(ctx context.Context, args ExplainSpecArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	req, err := svc.Prompt.ExplainSpec(ctx)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return req, nil
}

func (s *Server) handleSuggestPriorities(ctx context.Context, args SuggestPrioritiesArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	req, err := svc.Prompt.SuggestPriorities(ctx)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return req, nil
}

func (s *Server) handleReviewSpec(ctx context.Context, args ReviewSpecArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	req, err := svc.Prompt.ReviewSpec(ctx)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return req, nil
}

func (s *Server) handleExplainDrift(ctx context.Context, args ExplainDriftArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	report, err := svc.Drift.DetectDrift(ctx)
	if err != nil {
		return mcpErrCause("Failed to detect drift. Ensure both spec and plan exist.", err), nil
	}
	build := svc.Prompt.ExplainDrift
	if args.Patch {
		build = svc.Prompt.PatchDrift
	}

	req, err := build(ctx, report)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return req, nil
}

func (s *Server) handleAcceptDrift(ctx context.Context, args AcceptDriftArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if err := svc.Drift.AcceptDrift(); err != nil {
		return mcpErr("Failed to accept drift. Ensure a spec exists."), nil
	}
	return "Drift accepted and spec snapshot locked.", nil
}

type AddFeatureArgs struct {
	Title       string `json:"title" jsonschema:"description=The title of the new feature"`
	Description string `json:"description" jsonschema:"description=A detailed description of the feature"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleAddFeature(ctx context.Context, args AddFeatureArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	result, err := svc.Spec.AddFeature(args.Title, args.Description)
	if err != nil {
		return mcpErrCause("Failed to add feature. Ensure the project is initialized with a valid spec.", err), nil
	}

	msg := fmt.Sprintf("Successfully added feature '%s'. Total features: %d", args.Title, len(result.Spec.Features))
	for _, w := range result.Warnings {
		msg += fmt.Sprintf(". Warning: %s", w)
	}
	return msg, nil
}

func (s *Server) handleApprovePlan(ctx context.Context, args ApprovePlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	err = svc.Plan.ApprovePlan()
	if err != nil {
		return mcpErrCause("Failed to approve plan. Ensure a plan has been generated.", err), nil
	}
	return "Plan approved successfully", nil
}

func (s *Server) handleInit(ctx context.Context, args InitArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	err = svc.Init.InitializeProject(args.Name)
	if err != nil {
		return mcpErrCause("Failed to initialize project. Check directory permissions and ensure the name is valid.", err), nil
	}
	return fmt.Sprintf("Project %s initialized successfully", args.Name), nil
}

func (s *Server) handleGetSpec(ctx context.Context, args GetSpecArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	spec, err := svc.Spec.GetSpec()
	if err != nil {
		return mcpErrCause("Failed to load spec. Ensure the project is initialized with 'roady init'.", err), nil
	}
	return spec, nil
}

func (s *Server) handleGetPlan(ctx context.Context, args GetPlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	plan, err := svc.Plan.GetPlan()
	if err != nil {
		return mcpErrCause("Failed to load plan. Generate a plan first with 'roady plan generate'.", err), nil
	}
	return plan, nil
}

func (s *Server) handleGetState(ctx context.Context, args GetStateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	state, err := svc.Plan.GetState()
	if err != nil {
		return mcpErrCause("Failed to load execution state. Ensure a plan has been generated.", err), nil
	}
	return state, nil
}

func (s *Server) handleGeneratePlan(ctx context.Context, args GeneratePlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	plan, err := svc.Plan.GeneratePlan(ctx)
	if err != nil {
		return mcpErrCause("Failed to generate plan. Ensure a spec exists with at least one feature.", err), nil
	}
	return fmt.Sprintf("Plan generated with %d tasks. Plan ID: %s", len(plan.Tasks), plan.ID), nil
}

func (s *Server) handleCheckPolicy(ctx context.Context, args CheckPolicyArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	violations, err := svc.Policy.CheckCompliance()
	if err != nil {
		return mcpErrCause("Failed to check policy compliance. Ensure a policy.yaml and plan exist.", err), nil
	}
	if len(violations) == 0 {
		return "No policy violations found.", nil
	}
	return violations, nil
}

// handleAuditTrail answers "which agent worked on this, and what proves it".
// The trail was CLI-only until now, which left the agents this feature was
// built for unable to ask.
func (s *Server) handleAuditTrail(ctx context.Context, args AuditTrailArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if svc.AuditTrail == nil {
		return mcpErr("Audit trail is unavailable. Ensure the path points at an initialized Roady project."), nil
	}

	if args.TaskID == "" && args.Agent == "" && args.Session == "" {
		return mcpErr("Specify task_id, agent, or session_id to identify the subject of the trail."), nil
	}

	since, err := parseTrailSince(args.Since)
	if err != nil {
		return mcpErr(err.Error()), nil
	}

	trail, err := svc.AuditTrail.BuildTrail(ctx, application.TrailQuery{
		TaskID:    args.TaskID,
		Agent:     args.Agent,
		SessionID: args.Session,
		Since:     since,
	})
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to build the audit trail: %s", err)), nil
	}
	return trail, nil
}

// parseTrailSince accepts a relative window (7d, 2w) or an absolute date.
// Empty means the whole history.
func parseTrailSince(value string) (time.Time, error) {
	return application.ParseSince(value, time.Now())
}

func (s *Server) handleGetSnapshot(ctx context.Context, args GetSnapshotArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	snapshot, err := svc.Plan.GetProjectSnapshot(ctx)
	if err != nil {
		return mcpErrCause("Failed to get project snapshot. Ensure a plan and state exist.", err), nil
	}

	totalTasks := 0
	if snapshot.Plan != nil {
		totalTasks = len(snapshot.Plan.Tasks)
	}

	return snapshotResp{
		Progress:      snapshot.Progress,
		UnlockedTasks: capSlice(orEmpty(snapshot.UnlockedTasks)),
		BlockedTasks:  capSlice(orEmpty(snapshot.BlockedTasks)),
		InProgress:    capSlice(orEmpty(snapshot.InProgress)),
		Completed:     capSlice(orEmpty(snapshot.Completed)),
		Verified:      capSlice(orEmpty(snapshot.Verified)),
		TotalTasks:    totalTasks,
		SnapshotTime:  snapshot.SnapshotTime.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// handleTasks is the unified task-listing handler introduced in v0.10.0.
// The legacy per-status handlers below delegate to it so the response shape
// stays identical and a single code path serves both old and new tool names.
func (s *Server) handleTasks(ctx context.Context, args TasksArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}

	status := args.Status
	if status == "" {
		status = "ready"
	}

	var (
		tasks []project.TaskSummary
		err2  error
	)

	switch status {
	case "ready":
		tasks, err2 = svc.Plan.GetReadyTasks(ctx)
	case "in_progress":
		tasks, err2 = svc.Plan.GetInProgressTasks(ctx)
	case "blocked":
		tasks, err2 = svc.Plan.GetBlockedTasks(ctx)
	case "unassigned":
		tasks, err2 = svc.Plan.GetTasksByOwner(ctx, "")
	case "all":
		// Every task in one list rather than three, so a single page and a
		// single set of counts describe the whole answer. Each task carries
		// its own status, so nothing is lost by flattening.
		tasks, err2 = svc.Plan.GetTaskSummaries(ctx)
	default:
		return mcpErr("Invalid status. Use ready, in_progress, blocked, unassigned, or all."), nil
	}

	if err2 != nil {
		return mcpErr(fmt.Sprintf("Failed to get %s tasks. Ensure a plan and state exist.", status)), nil
	}

	// "unassigned" already filtered on owner; applying it again would drop
	// everything the moment a caller passed both.
	if status != "unassigned" {
		tasks = filterByAssignee(tasks, args.Assignee)
	}

	return paginateTasks(status, tasks, args.Offset, args.Limit, args.Detail), nil
}

// filterByAssignee narrows tasks to those owned by assignee. An empty assignee
// means "no filter requested" and returns tasks unchanged — callers wanting
// unassigned tasks use status=unassigned instead.
func filterByAssignee(tasks []project.TaskSummary, assignee string) []project.TaskSummary {
	want := strings.ToLower(strings.TrimSpace(assignee))
	if want == "" {
		return tasks
	}

	filtered := make([]project.TaskSummary, 0, len(tasks))
	for _, t := range tasks {
		if strings.ToLower(strings.TrimSpace(t.Owner)) == want {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func (s *Server) handleSmartDecompose(ctx context.Context, args SmartDecomposeArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	req, err := svc.Prompt.DecomposeSpec(ctx)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return req, nil
}

// orEmpty returns the slice or an empty slice if nil (for clean JSON output).
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
