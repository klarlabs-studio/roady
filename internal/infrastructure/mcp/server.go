package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"go.klarlabs.de/mcp"
	mcpserver "go.klarlabs.de/mcp/server"
)

// defaultHandlerTimeout is the maximum time a tool handler may run before
// the request context is cancelled.  AI-backed handlers (explain, query,
// decompose) are the main beneficiary — without this, a hung provider
// blocks the MCP connection indefinitely.
const defaultHandlerTimeout = 60 * time.Second

// maxCachedServices caps the number of cross-project service sets kept in
// memory.  When the cap is reached the oldest entry is evicted.
const maxCachedServices = 8

type Server struct {
	mcpServer *mcp.Server
	services  *wiring.AppServices
	initSvc   *application.InitService
	specSvc   *application.SpecService
	planSvc   *application.PlanService
	driftSvc  *application.DriftService
	policySvc *application.PolicyService
	taskSvc   *application.TaskService
	gitSvc    *application.GitService
	auditSvc  *application.EventSourcedAuditService
	root      string

	// svcCache caches AppServices built for cross-project paths so that
	// repeated requests don't rebuild the entire service stack (which
	// involves replaying the event log, loading AI config, etc.).
	svcCache   sync.Map // map[string]*wiring.AppServices
	svcCacheMu sync.Mutex
	svcKeys    []string // insertion-order keys for LRU eviction

	// confirm asks the user to approve a gated operation; nil asks through
	// MCP elicitation. Tests replace it.
	confirm confirmFunc
}

var (
	Version     = "dev"
	BuildCommit = "unknown"
	BuildDate   = "unknown"
)

// maxResponseItems caps list-style responses to prevent unbounded JSON
// serialization from exhausting memory or stalling the client.
const maxResponseItems = 500

// mcpErr returns a user-friendly error for MCP clients.
// Internal details are omitted — only the friendly message is returned.
// mcpErr reports a tool-execution failure in the form the MCP spec intends:
// a normal result carrying isError, with the message as content the model can
// read and act on.
//
// Returning a Go error instead makes the library treat it as a JSON-RPC
// protocol fault — it replaces the text with "internal error" and logs the
// real message to stderr, where an agent never sees it. Every one of Roady's
// carefully-worded messages was being thrown away that way; an agent calling
// a tool without a rate configured was told "internal error" and had no idea
// what to fix. Protocol errors are for malformed requests, not for a task
// that legitimately could not be done.
//
// The returned value is a result, so callers pass it as the *success* return
// and leave the error nil.
func mcpErr(friendly string) *mcpserver.StructuredResult {
	return &mcpserver.StructuredResult{
		IsError: true,
		Content: []mcpserver.Content{{Type: "text", Text: friendly}},
	}
}

// specHealth reports whether the project's spec currently parses.
//
// It exists because tools disagreed about whether a project was broken.
// roady_status reads plan.json and state.json and never unmarshals the spec,
// so it answered normally while spec.yaml held merge-conflict markers — the
// server looked healthy and exactly one tool looked flaky. A caller reasonably
// concluded roady was broken and stopped using it (#87).
//
// Returning the parse error rather than a boolean keeps the remedy in the same
// place as the symptom: the message names the file, line and reason, the same
// way the CLI does.
func specHealth(svc *wiring.AppServices) error {
	if svc == nil || svc.Spec == nil {
		return nil // nothing loaded; not this function's failure to report
	}
	_, err := svc.Spec.LockStatus()
	return err
}

// withSpecWarning prefixes a tool's answer with a spec-is-unparseable notice.
//
// The answer itself is still returned: status computed from plan.json is
// genuinely useful even when the spec is broken, and withholding it would
// trade one confusing outcome for another. What must not happen is returning
// it as though nothing were wrong.
func withSpecWarning(answer string, err error) string {
	if err == nil {
		return answer
	}
	return "WARNING: this project's spec.yaml does not currently parse, so any " +
		"tool that reads it will fail until it is repaired. Run `roady spec validate` " +
		"for the details.\nCause: " + err.Error() + "\n\n" + answer
}

// mcpErrCause is mcpErr with the diagnosis attached.
//
// The friendly sentence tells a caller what failed; the wrapped error tells
// them why, and roady's errors are specific enough to act on — "failed to
// unmarshal spec: yaml: line 11: could not find expected ':'" names the file,
// the line and the reason. Discarding it left the MCP caller with "Ensure the
// project is initialized with a valid spec" for a spec that was initialized
// and merely had conflict markers in it, while the CLI printed the exact
// cause for the same operation in the same process (#84).
//
// The reporter concluded roady itself was broken and hand-edited a backlog
// file instead. One line of detail would have pointed at the real problem.
func mcpErrCause(friendly string, err error) *mcpserver.StructuredResult {
	if err == nil {
		return mcpErr(friendly)
	}
	return mcpErr(friendly + " Cause: " + err.Error())
}

// requirePrompt guards the prompt-building handlers. It replaces the old
// requireAI nil-check: the provider is gone, but a services struct built
// against an unreadable project can still arrive without a PromptService,
// and a handler that dereferences it panics rather than answering.
func requirePrompt(svc *wiring.AppServices) *mcpserver.StructuredResult {
	if svc == nil || svc.Prompt == nil {
		return mcpErr("Project services are unavailable. Ensure the path points at an initialized Roady project.")
	}
	return nil
}

// servicesForPath returns services for the requested project scope.
// When pathOverride is empty (or matches the server root) AND project is empty,
// the server's default services are returned. Otherwise a fresh AppServices set
// is built and cached per (path, project) key — sub-projects under the same
// repo get their own cached entries.
//
// Cross-project services are cached to avoid rebuilding the full stack
// (event replay, AI config loading, etc.) on every request.
func (s *Server) servicesForPath(pathOverride, project string) (*wiring.AppServices, error) {
	usingDefault := (pathOverride == "" || pathOverride == s.root) && project == ""
	if usingDefault {
		if s.services != nil {
			return s.services, nil
		}
		// Fallback for servers constructed without services (e.g. tests).
		return &wiring.AppServices{
			Init:   s.initSvc,
			Spec:   s.specSvc,
			Plan:   s.planSvc,
			Drift:  s.driftSvc,
			Policy: s.policySvc,
			Task:   s.taskSvc,
			Git:    s.gitSvc,
			Audit:  s.auditSvc,
		}, nil
	}

	root := pathOverride
	if root == "" {
		root = s.root
	}
	cacheKey := root + "\x00" + project

	// Check cache first.
	if cached, ok := s.svcCache.Load(cacheKey); ok {
		return cached.(*wiring.AppServices), nil
	}

	// Build fresh services (expensive — involves event replay, AI config, etc.).
	svc, err := wiring.BuildAppServicesForProject(root, project)
	if err != nil && svc == nil {
		return nil, err
	}

	// Atomically store; if another goroutine raced us, use theirs.
	if existing, loaded := s.svcCache.LoadOrStore(cacheKey, svc); loaded {
		return existing.(*wiring.AppServices), nil
	}

	// We won the race — track the key for LRU eviction.
	s.svcCacheMu.Lock()
	if len(s.svcKeys) >= maxCachedServices {
		evict := s.svcKeys[0]
		s.svcKeys = s.svcKeys[1:]
		s.svcCache.Delete(evict)
	}
	s.svcKeys = append(s.svcKeys, cacheKey)
	s.svcCacheMu.Unlock()

	return svc, nil
}

func NewServer(root string) (*Server, error) {
	services, err := wiring.BuildAppServices(root)
	if err != nil && services == nil {
		// Hard failure — can't build any services at all.
		return nil, fmt.Errorf("build services: %w", err)
	}
	// Soft failure (e.g. AI provider not configured) is tolerated —
	// services is non-nil but AI-dependent tools will return errors.

	info := mcp.ServerInfo{
		Name:    "roady",
		Version: Version,
	}

	s := &Server{
		mcpServer: mcp.NewServer(info,
			mcp.WithTitle("Roady MCP Server"),
			mcp.WithDescription("Roady exposes deterministic project state, plans, and drift analysis to MCP clients."),
			mcp.WithWebsiteURL("https://github.com/felixgeelhaar/roady"),
			mcp.WithBuildInfo(BuildCommit, BuildDate),
			mcp.WithInstructions("Use tools to read spec/plan, generate plans, detect drift, and transition tasks."),
		),
		services:  services,
		initSvc:   services.Init,
		specSvc:   services.Spec,
		planSvc:   services.Plan,
		driftSvc:  services.Drift,
		policySvc: services.Policy,
		taskSvc:   services.Task,
		gitSvc:    services.Git,
		auditSvc:  services.Audit,
		root:      root,
	}

	// The server is small enough to list whole; the old profiles are gone.
	// Say so rather than silently ignoring a setting someone relies on.
	if v := os.Getenv("ROADY_MCP_TOOLS"); v != "" {
		fmt.Fprintf(os.Stderr, "roady: ROADY_MCP_TOOLS=%q is ignored; every tool is always listed\n", v)
	}

	s.registerTools()
	s.registerSchemaResource()
	return s, nil
}

// Args structs for handlers that previously used struct{}

type DetectDriftArgs struct {
	Semantic    bool   `json:"semantic,omitempty" jsonschema:"description=Instead of structural drift, return the prompt for judging whether implementations still mean what their requirements say; record the judgements with roady_drift action record"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

// DispatchTaskArgs hands a ready task to a subagent.
type DispatchTaskArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=The ready task to hand over."`
	Agent       string `json:"agent" jsonschema:"description=Name of the subagent taking the task. Recorded as the owner and against the completion transition."`
	Session     string `json:"session_id,omitempty" jsonschema:"description=Session ID to record the subagent's work under, so its events group separately from the dispatcher's."`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"description=Build the brief without claiming the task."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) registerTools() {
	// One tool per CLI noun; actions are the CLI's verbs (see nouns.go).
	s.tool("roady_next").
		Description("The task you are on, or the next to start: why it exists (doc:line), what done means, last check, dependencies, what it unblocks. Read at session start and after compaction.").
		Handler(s.handleNext)

	s.tool("roady_capture").
		Description("Record features, requirements and tasks, one or many, upserted by id (omitted fields keep their value). A requirement gets task-<id>. All or nothing; each rejection is returned. dry_run previews. The CLI's add, edit, split and move are shortcuts for this.").
		Handler(s.handleCapture)

	s.tool("roady_task").
		Description("roady task: start, complete, block, unblock, stop, reopen, verify (re-runs the acceptance check), check (run it and record the result), dispatch (hand to a subagent), list.").
		Handler(s.handleTask)

	s.tool("roady_plan").
		Description("roady plan: get, generate, import (a plan file an agent wrote), prioritize and decompose (prompts for your model); approve, reject and prune happen only if the user confirms.").
		Handler(s.handlePlan)

	s.tool("roady_spec").
		Description("roady spec: get, add, explain and review (prompts), validate; analyze, import and lock happen only if the user confirms.").
		Handler(s.handleSpec)

	s.tool("roady_drift").
		Description("roady drift: detect, explain, semantic (prompt for whether implementations still mean their requirements), record (the judgements on it); accept happens only if the user confirms.").
		Handler(s.handleDrift)

	s.tool("roady_status").
		Description("Project progress and tasks, filterable by ready, blocked, active, status and priority.").
		Handler(s.handleStatus)

	s.tool("roady_query").
		Description("Project context for a question, returned as a prompt for your model.").
		Handler(s.handleQuery)

	s.tool("roady_audit").
		Description("roady audit: verify (hash chain and the committed baseline), trail (evidence for a task, agent or session).").
		Handler(s.handleAudit)

	s.tool("roady_state").
		Description("roady state: get; rebuild (from the event log) happens only if the user confirms.").
		Handler(s.handleState)

	s.tool("roady_policy").
		Description("roady policy check: does the plan comply with policy (WIP limits, evidence).").
		Handler(s.handlePolicy)

	s.tool("roady_git").
		Description("roady git sync: move tasks forward from [roady:<task-id>] commit markers.").
		Handler(s.handleGit)

	s.tool("roady_init").
		Description("roady init: create a roady project in the server root.").
		Handler(s.handleInit)
}

// rootFor resolves the project directory a tool call addresses, mirroring
// servicesForPath: an explicit project_path wins, otherwise the server root.
// Paths must never resolve against the process working directory, which is
// routinely a different repository from the one being addressed.
func (s *Server) rootFor(pathOverride string) string {
	if strings.TrimSpace(pathOverride) != "" {
		return pathOverride
	}
	return s.root
}

// --- Parity handlers: CLI-only operations an agent could not reach ---

type RecordSemanticDriftArgs struct {
	Judgements  []drift.SemanticJudgement `json:"judgements" jsonschema:"description=One per requirement: requirement_id, agrees, and an explanation when agrees is false"`
	ProjectPath string                    `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string                    `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleRecordSemanticDrift(ctx context.Context, args RecordSemanticDriftArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	// The questions are rebuilt from the project as it is now, so a judgement
	// can only attach to a requirement roady actually asked about; an id the
	// model invented is rejected rather than recorded.
	_, questions, err := svc.Prompt.SemanticDrift(ctx)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	report, err := svc.Drift.RecordSemanticDrift(ctx, args.Judgements, questions)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return map[string]any{
		"divergent": len(report.Issues),
		"issues":    report.Issues,
		"judged":    len(args.Judgements),
	}, nil
}

func (s *Server) handleQuery(ctx context.Context, args QueryArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	req, err := svc.Prompt.QueryProject(ctx, args.Question)
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return req, nil
}

type QueryArgs struct {
	Question    string `json:"question" jsonschema:"description=A natural language question about the project"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

type TransitionTaskArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=The ID of the task to transition"`
	Event       string `json:"event" jsonschema:"description=start, complete, block, unblock, stop, reopen or verify"`
	Evidence    string `json:"evidence,omitempty" jsonschema:"description=Optional evidence for the transition (e.g. commit hash)"`
	Actor       string `json:"actor,omitempty" jsonschema:"description=Who transitions (default ai-agent)"`
	SessionID   string `json:"session_id,omitempty" jsonschema:"description=Agent session ID; recorded in the audit trail"`
	Agent       string `json:"agent,omitempty" jsonschema:"description=Agent name (e.g. codex); recorded in the audit trail"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

// FlexBool accepts both boolean and string ("true"/"false") JSON values.
// MCP clients sometimes send string values for boolean fields.
type FlexBool bool

func (fb *FlexBool) UnmarshalJSON(data []byte) error {
	// Try bool first
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		*fb = FlexBool(b)
		return nil
	}
	// Try string
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*fb = FlexBool(s == "true" || s == "1" || s == "yes")
		return nil
	}
	return fmt.Errorf("expected boolean or string, got %s", string(data))
}

// FlexInt accepts both integer and string JSON values.
type FlexInt int

func (fi *FlexInt) UnmarshalJSON(data []byte) error {
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*fi = FlexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		var n int
		if _, err := fmt.Sscanf(s, "%d", &n); err == nil {
			*fi = FlexInt(n)
			return nil
		}
	}
	return fmt.Errorf("expected integer or string, got %s", string(data))
}

// StatusArgs defines filter parameters for roady_status tool
type StatusArgs struct {
	Status      string   `json:"status,omitempty" jsonschema:"description=Comma-separated: pending,blocked,in_progress,done,verified"`
	Priority    string   `json:"priority,omitempty" jsonschema:"description=Comma-separated: high,medium,low"`
	Ready       FlexBool `json:"ready,omitempty" jsonschema:"description=Only tasks ready to start"`
	Blocked     FlexBool `json:"blocked,omitempty" jsonschema:"description=Only blocked tasks"`
	Active      FlexBool `json:"active,omitempty" jsonschema:"description=Only in-progress tasks"`
	Limit       FlexInt  `json:"limit,omitempty" jsonschema:"description=Maximum tasks returned"`
	JSON        FlexBool `json:"json,omitempty" jsonschema:"description=Structured output instead of text"`
	Snapshot    FlexBool `json:"snapshot,omitempty" jsonschema:"description=Progress and task ids by lifecycle stage instead of the task list"`
	ProjectPath string   `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string   `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleTransitionTask(ctx context.Context, args TransitionTaskArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	actor := args.Actor
	if actor == "" {
		actor = "ai-agent"
	}

	// A caller that names its own session and agent overrides the ambient
	// process identity — an agent forwarding the run that spawned it knows
	// more than the server process does. Restored afterwards so one
	// request's identity never leaks into the next.
	if args.SessionID != "" || args.Agent != "" {
		previous := svc.Audit.Provenance()
		svc.Audit.SetProvenance(previous.WithSession(args.SessionID, args.Agent))
		defer svc.Audit.SetProvenance(previous)
	}

	err = svc.Task.TransitionTask(args.TaskID, args.Event, actor, args.Evidence)
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to transition task '%s' with event '%s': %v", args.TaskID, args.Event, err)), nil
	}
	return fmt.Sprintf("Task %s transitioned with event %s successfully", args.TaskID, args.Event), nil
}

func (s *Server) handleDetectDrift(ctx context.Context, args DetectDriftArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if args.Semantic {
		req, questions, err := svc.Prompt.SemanticDrift(ctx)
		if err != nil {
			return mcpErr(err.Error()), nil
		}
		return map[string]any{"request": req, "questions": questions}, nil
	}
	report, err := svc.Drift.DetectDrift(ctx)
	if err != nil {
		return mcpErrCause("Failed to detect drift. Ensure both spec and plan exist.", err), nil
	}
	return report, nil
}

func (s *Server) handleStatus(ctx context.Context, args StatusArgs) (any, error) {
	if args.Snapshot {
		return s.handleGetSnapshot(ctx, GetSnapshotArgs{ProjectPath: args.ProjectPath, Project: args.Project})
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	plan, err := svc.Plan.GetPlan()
	if err != nil {
		return mcpErrCause("Failed to load plan. Generate a plan first with 'roady plan generate'.", err), nil
	}
	if plan == nil {
		// Also warned here: "generate a plan" is bad advice when the spec is
		// unparseable, because generating one reads it and will fail too.
		return withSpecWarning("No plan found. Record one with roady_capture, or run `roady plan generate`.", specHealth(svc)), nil
	}

	state, err := svc.Plan.GetState()
	if err != nil {
		return mcpErrCause("Failed to load execution state. Ensure a plan has been generated.", err), nil
	}
	if state == nil {
		return withSpecWarning("No execution state found.", specHealth(svc)), nil
	}

	// Parse filters
	var statusFilters []planning.TaskStatus
	if args.Status != "" {
		for _, s := range strings.Split(args.Status, ",") {
			if parsed, err := planning.ParseTaskStatus(strings.TrimSpace(s)); err == nil {
				statusFilters = append(statusFilters, parsed)
			}
		}
	}

	var priorityFilters []planning.TaskPriority
	if args.Priority != "" {
		for _, p := range strings.Split(args.Priority, ",") {
			if parsed, err := planning.ParseTaskPriority(strings.TrimSpace(p)); err == nil {
				priorityFilters = append(priorityFilters, parsed)
			}
		}
	}

	// Filter tasks
	var filtered []planning.Task
	for _, t := range plan.Tasks {
		status := planning.StatusPending
		if res, ok := state.TaskStates[t.ID]; ok {
			status = res.Status
		}

		// Apply shortcut flags
		if bool(args.Ready) {
			if status != planning.StatusPending || !isTaskUnlockedByDeps(t, state) {
				continue
			}
		}
		if bool(args.Blocked) && status != planning.StatusBlocked {
			continue
		}
		if bool(args.Active) && status != planning.StatusInProgress {
			continue
		}

		// Apply status filter
		if len(statusFilters) > 0 && !containsStatus(statusFilters, status) {
			continue
		}

		// Apply priority filter
		if len(priorityFilters) > 0 && !containsPriority(priorityFilters, t.Priority) {
			continue
		}

		filtered = append(filtered, t)
	}

	// Apply limit (default to maxResponseItems to prevent unbounded responses).
	limit := int(args.Limit)
	if limit <= 0 {
		limit = maxResponseItems
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	// Count all tasks by status (for summary)
	counts := make(map[string]int)
	for _, t := range plan.Tasks {
		status := "pending"
		if res, ok := state.TaskStates[t.ID]; ok {
			status = string(res.Status)
		}
		counts[status]++
	}

	// JSON output
	if bool(args.JSON) {
		type taskOutput struct {
			ID       string `json:"id"`
			Title    string `json:"title"`
			Status   string `json:"status"`
			Priority string `json:"priority"`
			Owner    string `json:"owner,omitempty"`
			Unlocked bool   `json:"unlocked,omitempty"`
		}

		tasks := make([]taskOutput, 0, len(filtered))
		for _, t := range filtered {
			status := planning.StatusPending
			owner := ""
			if res, ok := state.TaskStates[t.ID]; ok {
				status = res.Status
				owner = res.Owner
			}
			tasks = append(tasks, taskOutput{
				ID:       t.ID,
				Title:    t.Title,
				Status:   string(status),
				Priority: string(t.Priority),
				Owner:    owner,
				Unlocked: status == planning.StatusPending && isTaskUnlockedByDeps(t, state),
			})
		}

		output := map[string]any{
			"total_tasks":    len(plan.Tasks),
			"filtered_count": len(filtered),
			"counts":         counts,
			"tasks":          tasks,
		}
		// A machine reader gets the same signal the text path prints, rather
		// than a clean object that implies a healthy project.
		if err := specHealth(svc); err != nil {
			output["spec_valid"] = false
			output["spec_error"] = err.Error()
		} else {
			output["spec_valid"] = true
		}

		jsonBytes, err := json.Marshal(output)
		if err != nil {
			return mcpErrCause("Failed to format status output.", err), nil
		}
		return string(jsonBytes), nil
	}

	// Text output
	statusStr := fmt.Sprintf("Tasks: %d total\n- Done: %d\n- In Progress: %d\n- Pending: %d\n- Blocked: %d",
		len(plan.Tasks), counts["done"]+counts["verified"], counts["in_progress"], counts["pending"], counts["blocked"])

	if len(statusFilters) > 0 || len(priorityFilters) > 0 || bool(args.Ready) || bool(args.Blocked) || bool(args.Active) {
		statusStr += fmt.Sprintf("\n\nFiltered Tasks: %d", len(filtered))
		for _, t := range filtered {
			status := planning.StatusPending
			if res, ok := state.TaskStates[t.ID]; ok {
				status = res.Status
			}
			statusStr += fmt.Sprintf("\n- [%s] %s (%s)", status, t.Title, t.Priority)
		}
	}

	return withSpecWarning(statusStr, specHealth(svc)), nil
}

// isTaskUnlockedByDeps checks if all dependencies are complete
func isTaskUnlockedByDeps(task planning.Task, state *planning.ExecutionState) bool {
	for _, depID := range task.DependsOn {
		if state == nil {
			return false
		}
		if res, ok := state.TaskStates[depID]; ok {
			if !res.Status.IsComplete() {
				return false
			}
		} else {
			return false
		}
	}
	return true
}

// containsStatus checks if a status is in the list
func containsStatus(statuses []planning.TaskStatus, s planning.TaskStatus) bool {
	for _, status := range statuses {
		if status == s {
			return true
		}
	}
	return false
}

// containsPriority checks if a priority is in the list
func containsPriority(priorities []planning.TaskPriority, p planning.TaskPriority) bool {
	for _, priority := range priorities {
		if priority == p {
			return true
		}
	}
	return false
}

// serveMiddleware returns the standard middleware stack applied to every
// transport.  Recover catches handler panics so a single buggy tool call
// cannot crash the entire MCP process.  Timeout prevents hung AI/plugin
// calls from blocking the connection indefinitely.
func (s *Server) serveMiddleware() mcp.ServeOption {
	return mcp.WithMiddleware(
		mcp.Recover(),
		mcp.Timeout(defaultHandlerTimeout),
	)
}

func (s *Server) Start() error {
	return s.StartStdio()
}

func (s *Server) StartStdio() error {
	return s.ServeStdio(context.Background())
}

func (s *Server) ServeStdio(ctx context.Context) error {
	return mcp.ServeStdio(ctx, s.mcpServer, s.serveMiddleware())
}

// Dependency MCP handlers

// Debt MCP handlers

// handleDispatchTask prepares a task for handoff.
//
// The completion contract is the point: work a subagent does that never lands
// as a recorded transition is invisible to the audit trail, so the brief names
// the exact call and fills in the agent and session.
func (s *Server) handleDispatchTask(ctx context.Context, args DispatchTaskArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if svc.Dispatch == nil {
		return mcpErr("Dispatch is unavailable. Ensure the path points at an initialized Roady project."), nil
	}

	brief, err := svc.Dispatch.Dispatch(ctx, args.TaskID, application.DispatchOptions{
		Agent:   args.Agent,
		Session: args.Session,
		Start:   !args.DryRun,
	})
	if err != nil {
		return mcpErr(err.Error()), nil
	}
	return brief, nil
}

// Coordinator-based snapshot and task query handlers (v0.6.0)

// --- Workspace Sync Handlers ---

// --- Smart Decompose Handler ---

// --- Team Handlers ---

// TaskCheckArgs selects the task whose acceptance check to run.
type TaskCheckArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=The ID of the task whose check to run"`
	Actor       string `json:"actor,omitempty" jsonschema:"description=Who is running the check (defaults to ai-agent)"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleTaskCheck(ctx context.Context, args TaskCheckArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	actor := args.Actor
	if actor == "" {
		actor = "ai-agent"
	}
	// No confirmation over MCP: a manual check is a person's judgement, and an
	// agent confirming its own manual check would defeat it.
	result, err := svc.Task.RunCheck(ctx, args.TaskID, actor, application.CheckOptions{})
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to run the check for task '%s': %v", args.TaskID, err)), nil
	}
	return result, nil
}

// CaptureArgs is a capture document plus the usual project selectors.
type CaptureArgs struct {
	Features    []application.CaptureFeature `json:"features,omitempty" jsonschema:"description=Features to add or update; each may carry requirements"`
	Tasks       []application.CaptureTask    `json:"tasks,omitempty" jsonschema:"description=Tasks to add or update"`
	DryRun      bool                         `json:"dry_run,omitempty" jsonschema:"description=Report what would change without writing"`
	Actor       string                       `json:"actor,omitempty" jsonschema:"description=Who is capturing (defaults to ai-agent)"`
	ProjectPath string                       `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string                       `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleCapture(ctx context.Context, args CaptureArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	actor := args.Actor
	if actor == "" {
		actor = "ai-agent"
	}
	result, err := svc.Capture.Capture(application.CaptureDoc{Features: args.Features, Tasks: args.Tasks},
		application.CaptureOptions{Actor: actor, DryRun: args.DryRun, Origin: planning.OriginAI})
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to capture: %v", err)), nil
	}
	// A rejected capture is a result the caller must act on, not a failed
	// call: it lists every item and why, and nothing was written.
	return result, nil
}

// PlanImportArgs names a plan file to import.
type PlanImportArgs struct {
	Path        string `json:"path" jsonschema:"required,description=Plan file to import (absolute, or relative to the project)"`
	Format      string `json:"format,omitempty" jsonschema:"description=auto (default), kiro, execplan or markdown"`
	FeatureID   string `json:"feature_id,omitempty" jsonschema:"description=Attach the tasks to this existing feature instead of creating one for the plan"`
	Parallel    bool   `json:"parallel,omitempty" jsonschema:"description=Leave steps independent instead of chaining each to the one before"`
	IncludeDone bool   `json:"include_done,omitempty" jsonschema:"description=Import steps already checked off"`
	DryRun      bool   `json:"dry_run,omitempty" jsonschema:"description=Report what would change without writing"`
	Actor       string `json:"actor,omitempty" jsonschema:"description=Who is importing (defaults to ai-agent)"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handlePlanImport(ctx context.Context, args PlanImportArgs) (any, error) {
	if strings.TrimSpace(args.Path) == "" {
		return mcpErr("path is required: the plan file to import."), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	if svc.Capture == nil {
		return mcpErr("Project services are unavailable. Ensure the path points at an initialized Roady project."), nil
	}
	root := s.rootFor(args.ProjectPath)
	path := args.Path
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	current, _ := svc.Spec.GetSpec()
	imp, err := application.ImportPlanFile(path, current, application.PlanImportOptions{
		Format: args.Format, FeatureID: args.FeatureID, Parallel: args.Parallel, IncludeDone: args.IncludeDone, Root: root,
	})
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to import plan: %v", err)), nil
	}
	actor := args.Actor
	if actor == "" {
		actor = "ai-agent"
	}
	result, err := svc.Capture.Capture(imp.Doc, application.CaptureOptions{Actor: actor, DryRun: args.DryRun, Origin: planning.OriginAI})
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to import plan: %v", err)), nil
	}
	return map[string]any{
		"format":       imp.Format,
		"title":        imp.Title,
		"steps":        len(imp.Doc.Tasks),
		"skipped_done": imp.Skipped,
		"result":       result,
	}, nil
}

// NextArgs selects whose brief to build.
type NextArgs struct {
	Owner       string `json:"owner,omitempty" jsonschema:"description=Whose task to brief on (defaults to ai-agent, the owner MCP transitions record)"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project in .roady/projects (default: root)"`
}

func (s *Server) handleNext(ctx context.Context, args NextArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErrCause("Failed to load project at the given path.", err), nil
	}
	owner := args.Owner
	if owner == "" {
		owner = "ai-agent"
	}
	brief, err := svc.Task.Brief(ctx, owner)
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to build the brief: %v", err)), nil
	}
	return map[string]any{"brief": brief.Render(), "detail": brief}, nil
}

func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
	// Stateless Streamable HTTP (MCP 2026-07-28), mcp-go's default since
	// 1.26: no initialize handshake, and every request carries Mcp-Method and
	// MCP-Protocol-Version. Clients connect with server/discover (the SDK's
	// Connect).
	return mcp.ServeHTTPWithMiddleware(ctx, s.mcpServer, addr,
		[]mcp.HTTPOption{mcp.WithDefaultCORS()},
		s.serveMiddleware(),
	)
}

func (s *Server) ServeWebSocket(ctx context.Context, addr string) error {
	return mcp.ServeWebSocketWithMiddleware(ctx, s.mcpServer, addr,
		nil,
		s.serveMiddleware(),
	)
}

func (s *Server) ServeGRPC(ctx context.Context, addr string) error {
	return mcp.ServeGRPCWithMiddleware(ctx, s.mcpServer, addr,
		nil,
		s.serveMiddleware(),
	)
}
