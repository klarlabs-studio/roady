package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/billing"
	"github.com/felixgeelhaar/roady/pkg/domain/debt"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/org"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/domain/team"
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
	mcpServer   *mcp.Server
	services    *wiring.AppServices
	initSvc     *application.InitService
	specSvc     *application.SpecService
	planSvc     *application.PlanService
	driftSvc    *application.DriftService
	policySvc   *application.PolicyService
	taskSvc     *application.TaskService
	billingSvc  *application.BillingService
	gitSvc      *application.GitService
	syncSvc     *application.SyncService
	auditSvc    *application.EventSourcedAuditService
	depSvc      *application.DependencyService
	debtSvc     *application.DebtService
	forecastSvc *application.ForecastService
	orgSvc      *application.OrgService
	pluginSvc   *application.PluginService
	teamSvc     *application.TeamService
	root        string

	// svcCache caches AppServices built for cross-project paths so that
	// repeated requests don't rebuild the entire service stack (which
	// involves replaying the event log, loading AI config, etc.).
	svcCache   sync.Map // map[string]*wiring.AppServices
	svcCacheMu sync.Mutex
	svcKeys    []string // insertion-order keys for LRU eviction
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

// capSlice returns at most maxResponseItems elements from a string slice.
func capSlice(s []string) []string {
	if len(s) > maxResponseItems {
		return s[:maxResponseItems]
	}
	return s
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
			Init:       s.initSvc,
			Spec:       s.specSvc,
			Plan:       s.planSvc,
			Drift:      s.driftSvc,
			Policy:     s.policySvc,
			Task:       s.taskSvc,
			Billing:    s.billingSvc,
			Git:        s.gitSvc,
			Sync:       s.syncSvc,
			Audit:      s.auditSvc,
			Forecast:   s.forecastSvc,
			Dependency: s.depSvc,
			Debt:       s.debtSvc,
			Team:       s.teamSvc,
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
		services:    services,
		initSvc:     services.Init,
		specSvc:     services.Spec,
		planSvc:     services.Plan,
		driftSvc:    services.Drift,
		policySvc:   services.Policy,
		taskSvc:     services.Task,
		billingSvc:  services.Billing,
		gitSvc:      services.Git,
		syncSvc:     services.Sync,
		auditSvc:    services.Audit,
		depSvc:      services.Dependency,
		debtSvc:     services.Debt,
		forecastSvc: services.Forecast,
		orgSvc:      application.NewOrgService(root),
		pluginSvc:   application.NewPluginService(services.Workspace.Repo),
		teamSvc:     services.Team,
		root:        root,
	}

	s.registerTools()
	s.registerApps()
	s.registerSchemaResource()
	return s, nil
}

type InitArgs struct {
	Name        string `json:"name" jsonschema:"description=The name of the project"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type UpdatePlanArgs struct {
	Tasks       []planning.Task `json:"tasks" jsonschema:"description=The list of tasks to define the plan"`
	ProjectPath string          `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string          `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

// Args structs for handlers that previously used struct{}

type GetSpecArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetPlanArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetStateArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GeneratePlanArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type DetectDriftArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type ApprovePlanArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type ExplainSpecArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type ExplainDriftArgs struct {
	Patch       bool   `json:"patch,omitempty" jsonschema:"description=Ask for a unified diff that closes the drift instead of an explanation. Intent and staleness drift are excluded: they are decisions about what to build, not changes a diff can make."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type AcceptDriftArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetUsageArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type CheckPolicyArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type ForecastArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GitSyncArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type SuggestPrioritiesArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type ReviewSpecArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetSnapshotArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetReadyTasksArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetBlockedTasksArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type GetInProgressTasksArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

// TasksArgs supersedes the three legacy roady_get_*_tasks tools by adding a
// single status enum parameter. Existing tools delegate to the same handler
// for backward compatibility.
type TasksArgs struct {
	Status      string `json:"status,omitempty" jsonschema:"description=Which tasks to return: ready (unlocked + pending), in_progress, blocked, unassigned, or all. Defaults to ready.,enum=ready,enum=in_progress,enum=blocked,enum=unassigned,enum=all"`
	Assignee    string `json:"assignee,omitempty" jsonschema:"description=Only return tasks assigned to this person or agent. Matched case-insensitively. Ignored when status is unassigned."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

// AuditTrailArgs selects what to build an evidence trail about. Exactly one
// of TaskID, Agent, or Session identifies the subject; combining TaskID with
// Agent narrows to what that agent did to that task.
type AuditTrailArgs struct {
	TaskID      string `json:"task_id,omitempty" jsonschema:"description=Build the trail for this task."`
	Agent       string `json:"agent,omitempty" jsonschema:"description=Only include events from this agent (e.g. claude-code)."`
	Session     string `json:"session_id,omitempty" jsonschema:"description=Only include events from this session ID."`
	Since       string `json:"since,omitempty" jsonschema:"description=Only include events since this point: 7d, 2w, or an absolute date like 2026-07-01."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

// CostEstimateArgs are the inputs for roady_cost_estimate. Operation defaults
// to generate_plan when omitted; project_path is server root unless
// overridden.
type CostEstimateArgs struct {
	Operation   string `json:"operation,omitempty" jsonschema:"description=AI operation to estimate. Defaults to generate_plan.,enum=generate_plan,enum=smart_decompose,enum=review_spec,enum=explain_drift,enum=query"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type SmartDecomposeArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type WorkspacePushArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type WorkspacePullArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type SyncArgs struct {
	PluginPath  string `json:"plugin_path" jsonschema:"description=Path to the syncer plugin binary"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) registerTools() {
	// Tool: roady_init
	s.tool("roady_init").
		Description("Initialize a new roady project in the current directory").
		UIResource("ui://roady/init").
		Handler(s.handleInit)

	// Tool: roady_get_spec
	s.tool("roady_get_spec").
		Description("Retrieve the current product specification").
		UIResource("ui://roady/spec").
		OutputSchema(spec.ProductSpec{}).
		Handler(s.handleGetSpec)

	// Tool: roady_get_plan
	s.tool("roady_get_plan").
		Description("Retrieve the current execution plan").
		UIResource("ui://roady/plan").
		OutputSchema(planning.Plan{}).
		Handler(s.handleGetPlan)

	// Tool: roady_get_state
	s.tool("roady_get_state").
		Description("Retrieve the current execution state (task statuses)").
		UIResource("ui://roady/state").
		OutputSchema(planning.ExecutionState{}).
		Handler(s.handleGetState)

	// Tool: roady_generate_plan (Heuristic)
	s.tool("roady_generate_plan").
		Description("Generate a basic plan from the spec using 1:1 heuristic (resets custom tasks unless they match features)").
		UIResource("ui://roady/plan").
		Handler(s.handleGeneratePlan)

	// Tool: roady_update_plan (Smart Injection)
	s.tool("roady_update_plan").
		Description("Update the plan with a specific list of tasks (Smart Injection). Use this to propose complex architectures.").
		UIResource("ui://roady/plan").
		Handler(s.handleUpdatePlan)

	// Tool: roady_detect_drift
	s.tool("roady_detect_drift").
		Description("Detect discrepancies between the current Spec and Plan").
		UIResource("ui://roady/drift").
		OutputSchema(drift.Report{}).
		Handler(s.handleDetectDrift)

	// Tool: roady_accept_drift
	s.tool("roady_accept_drift").
		Description("Accept the current drift by locking the spec snapshot").
		UIResource("ui://roady/drift").
		Handler(s.handleAcceptDrift)

	// Tool: roady_status
	s.tool("roady_status").
		Description("Get a high-level summary of the project status").
		UIResource("ui://roady/status").
		Handler(s.handleStatus)

	// Tool: roady_check_policy
	s.tool("roady_check_policy").
		Description("Check if the current plan complies with execution policies (e.g., WIP limits)").
		UIResource("ui://roady/policy").
		Handler(s.handleCheckPolicy)

	// Tool: roady_transition_task
	s.tool("roady_transition_task").
		Description("Transition a task to a new state (e.g., start, complete, block, stop)").
		UIResource("ui://roady/state").
		Handler(s.handleTransitionTask)

	// Tool: roady_explain_spec
	s.tool("roady_explain_spec").
		Description("Provide an AI-generated architectural walkthrough of the current specification").
		UIResource("ui://roady/spec").
		Handler(s.handleExplainSpec)

	// Tool: roady_approve_plan
	s.tool("roady_approve_plan").
		Description("Approve the current plan for execution").
		UIResource("ui://roady/plan").
		Handler(s.handleApprovePlan)

	// Tool: roady_get_usage
	s.tool("roady_get_usage").
		Description("Retrieve project usage and telemetry statistics").
		UIResource("ui://roady/usage").
		OutputSchema(domain.UsageStats{}).
		Handler(s.handleGetUsage)

	// Tool: roady_explain_drift
	s.tool("roady_explain_drift").
		Description("Provide an AI-generated explanation and resolution steps for current project drift").
		UIResource("ui://roady/drift").
		Handler(s.handleExplainDrift)

	// Tool: roady_add_feature
	s.tool("roady_add_feature").
		Description("Add a new feature to the product specification and sync to docs/backlog.md").
		UIResource("ui://roady/spec").
		Handler(s.handleAddFeature)

	// Tool: roady_forecast (Horizon 5)
	s.tool("roady_forecast").
		Description("Predict project completion based on current task velocity").
		UIResource("ui://roady/forecast").
		OutputSchema(forecastResp{}).
		Handler(s.handleForecast)

	// Tool: roady_org_status (Horizon 4)
	s.tool("roady_org_status").
		Description("Get a status overview of all Roady projects in the directory tree").
		UIResource("ui://roady/org").
		OutputSchema(org.OrgMetrics{}).
		Handler(s.handleOrgStatus)

	// Tool: roady_git_sync (Horizon 5)
	s.tool("roady_git_sync").
		Description("Synchronize task statuses by scanning git commit messages for markers").
		UIResource("ui://roady/git-sync").
		Handler(s.handleGitSync)

	// Tool: roady_sync (External Plugins)
	s.tool("roady_sync").
		Description("Sync the plan with an external system via a plugin binary").
		UIResource("ui://roady/sync").
		Handler(s.handleSync)

	// Tool: roady_deps_list (Horizon 5)
	s.tool("roady_deps_list").
		Description("List all cross-repository dependencies").
		UIResource("ui://roady/deps").
		Handler(s.handleDepsList)

	// Tool: roady_deps_scan (Horizon 5)
	s.tool("roady_deps_scan").
		Description("Scan health status of all dependent repositories").
		UIResource("ui://roady/deps").
		OutputSchema(application.ScanResult{}).
		Handler(s.handleDepsScan)

	// Tool: roady_deps_graph (Horizon 5)
	s.tool("roady_deps_graph").
		Description("Get dependency graph summary with optional cycle detection").
		UIResource("ui://roady/deps").
		Handler(s.handleDepsGraph)

	// Tool: roady_debt_report (Horizon 5)
	s.tool("roady_debt_report").
		Description("Generate comprehensive debt report with category breakdown and top debtors").
		UIResource("ui://roady/debt").
		OutputSchema(debt.DebtReport{}).
		Handler(s.handleDebtReport)

	// Tool: roady_debt_summary (Horizon 5)
	s.tool("roady_debt_summary").
		Description("Quick overview of debt status including health level and top debtor").
		UIResource("ui://roady/debt").
		OutputSchema(application.DebtSummary{}).
		Handler(s.handleDebtSummary)

	// Tool: roady_drift_recurring (v0.10.0 - canonical name for sticky drift)
	s.tool("roady_drift_recurring").
		Description("Return drift items that have remained unresolved for more than 7 days. Canonical name; supersedes roady_sticky_drift.").
		UIResource("ui://roady/debt").
		Handler(s.handleStickyDrift)

	// Tool: roady_audit_trail
	s.tool("roady_audit_trail").
		Description("Evidence trail for a task, agent, or session: hash-chain integrity, findings, the task's evidence and its doc:line spec citation, who acted, and every recorded event. Attests to a tamper-evident record of what was asserted, not to who acted -- actor and agent are caller-supplied and unauthenticated.").
		UIResource("ui://roady/status").
		Handler(s.handleAuditTrail)

	// Tool: roady_debt_trend (Horizon 5)
	s.tool("roady_debt_trend").
		Description("Analyze drift trend over time").
		UIResource("ui://roady/debt").
		OutputSchema(events.DriftTrend{}).
		Handler(s.handleDebtTrend)

	// Tool: roady_org_policy (v0.7.0)
	s.tool("roady_org_policy").
		Description("Get merged policy for a project (org defaults + project overrides)").
		UIResource("ui://roady/org").
		Handler(s.handleOrgPolicy)

	// Tool: roady_org_detect_drift (v0.7.0)
	s.tool("roady_org_detect_drift").
		Description("Detect drift across all projects in the directory tree").
		UIResource("ui://roady/org").
		Handler(s.handleOrgDetectDrift)

	// Tool: roady_plugin_list (v0.7.0)
	s.tool("roady_plugin_list").
		Description("List all registered plugins with their status").
		UIResource("ui://roady/plugins").
		Handler(s.handlePluginList)

	// Tool: roady_plugin_validate (v0.7.0)
	s.tool("roady_plugin_validate").
		Description("Validate a registered plugin by loading and initializing it").
		UIResource("ui://roady/plugins").
		Handler(s.handlePluginValidate)

	// Tool: roady_plugin_status (v0.7.0)
	s.tool("roady_plugin_status").
		Description("Check health status of one or all plugins").
		UIResource("ui://roady/plugins").
		Handler(s.handlePluginStatus)

	// Tool: roady_messaging_list (v0.7.0)
	s.tool("roady_messaging_list").
		Description("List configured messaging adapters").
		UIResource("ui://roady/messaging").
		Handler(s.handleMessagingList)

	// Tool: roady_query (v0.8.0)
	s.tool("roady_query").
		Description("Ask a natural language question about the project and get an AI-generated answer").
		UIResource("ui://roady/status").
		Handler(s.handleQuery)

	// Tool: roady_suggest_priorities (v0.8.0)
	s.tool("roady_suggest_priorities").
		Description("AI-powered priority suggestions based on spec analysis and task dependencies").
		UIResource("ui://roady/plan").
		OutputSchema(planning.PrioritySuggestions{}).
		Handler(s.handleSuggestPriorities)

	// Tool: roady_review_spec (v0.8.0)
	s.tool("roady_review_spec").
		Description("Perform an AI-powered quality review of the current specification, returning a score and structured findings").
		UIResource("ui://roady/spec").
		OutputSchema(spec.SpecReview{}).
		Handler(s.handleReviewSpec)

	// Tool: roady_assign_task (v0.8.0)
	s.tool("roady_assign_task").
		Description("Assign a task to a person or agent without changing its status").
		UIResource("ui://roady/state").
		Handler(s.handleAssignTask)

	// Tool: roady_get_snapshot (v0.6.0 - Coordinator)
	s.tool("roady_get_snapshot").
		Description("Get a consistent project snapshot with progress, categorized task counts, and task lists").
		UIResource("ui://roady/status").
		OutputSchema(snapshotResp{}).
		Handler(s.handleGetSnapshot)

	// Tool: roady_cost_estimate (v0.10.0 - pre-flight cost projection)
	// Tool: roady_tasks (v0.10.0 - unified task listing)
	// Supersedes roady_get_ready_tasks, roady_get_blocked_tasks, and
	// roady_get_in_progress_tasks. Takes a status enum (ready, in_progress,
	// blocked, all). The legacy tools below remain registered as deprecation
	// aliases that delegate to the same handler.
	s.tool("roady_tasks").
		Description("List tasks by status. Pass status=ready (default), in_progress, blocked, or all. Supersedes roady_get_*_tasks.").
		UIResource("ui://roady/status").
		Handler(s.handleTasks)

	// Tool: roady_workspace_push (v0.8.0)
	s.tool("roady_workspace_push").
		Description("Commit and push .roady/ workspace state to git remote").
		UIResource("ui://roady/workspace").
		Handler(s.handleWorkspacePush)

	// Tool: roady_workspace_pull (v0.8.0)
	s.tool("roady_workspace_pull").
		Description("Pull remote .roady/ workspace changes and merge with conflict detection").
		UIResource("ui://roady/workspace").
		Handler(s.handleWorkspacePull)

	// Tool: roady_plan_decompose (v0.10.0 - canonical name)
	s.tool("roady_plan_decompose").
		Description("AI-powered context-aware task decomposition using codebase structure analysis. Canonical name; supersedes roady_smart_decompose.").
		UIResource("ui://roady/plan").
		Handler(s.handleSmartDecompose)

	// Tool: roady_team_list (v0.8.0)
	s.tool("roady_team_list").
		Description("List all team members and their roles").
		UIResource("ui://roady/team").
		Handler(s.handleTeamList)

	// Tool: roady_team_add (v0.8.0)
	s.tool("roady_team_add").
		Description("Add or update a team member with a role (admin, member, viewer)").
		UIResource("ui://roady/team").
		Handler(s.handleTeamAdd)

	// Tool: roady_team_remove (v0.8.0)
	s.tool("roady_team_remove").
		Description("Remove a team member").
		UIResource("ui://roady/team").
		Handler(s.handleTeamRemove)

	// Billing tools
	// Tool: roady_rate_list
	s.tool("roady_rate_list").
		Description("List all billing rates").
		UIResource("ui://roady/billing").
		Handler(s.handleRateList)

	// Tool: roady_rate_add
	s.tool("roady_rate_add").
		Description("Add a new billing rate").
		UIResource("ui://roady/billing").
		Handler(s.handleRateAdd)

	// Tool: roady_task_log_time
	s.tool("roady_task_log_time").
		Description("Log time to a task for billing").
		UIResource("ui://roady/billing").
		Handler(s.handleTaskLogTime)

	// Tool: roady_cost_report
	s.tool("roady_cost_report").
		Description("Generate a cost report for time tracking").
		UIResource("ui://roady/billing").
		OutputSchema(billing.CostReport{}).
		Handler(s.handleCostReport)

	// Tool: roady_cost_budget
	s.tool("roady_cost_budget").
		Description("Show budget status based on budget_hours in policy").
		UIResource("ui://roady/billing").
		OutputSchema(billing.BudgetStatus{}).
		Handler(s.handleCostBudget)

	// Tool: roady_rate_remove
	s.tool("roady_rate_remove").
		Description("Remove a billing rate").
		UIResource("ui://roady/billing").
		Handler(s.handleRateRemove)

	// Tool: roady_rate_set_default
	s.tool("roady_rate_set_default").
		Description("Set the default billing rate").
		UIResource("ui://roady/billing").
		Handler(s.handleRateSetDefault)

	// Tool: roady_rate_tax
	s.tool("roady_rate_tax").
		Description("Configure tax settings for billing").
		UIResource("ui://roady/billing").
		Handler(s.handleRateTax)
}

func (s *Server) handleForecast(ctx context.Context, args ForecastArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	forecast, err := svc.Forecast.GetForecast()
	if err != nil {
		return mcpErr("Unable to generate forecast. Ensure a plan exists and tasks have been transitioned."), nil
	}
	if forecast == nil {
		return "No plan found. Generate a plan first.", nil
	}

	// Build a JSON-friendly response with burndown data for the UI. The
	// forecastResp/burndownPt/windowPt types are declared in output_schemas.go
	// so roady_forecast can advertise them as its outputSchema.
	resp := forecastResp{
		Remaining:      forecast.RemainingTasks,
		Completed:      forecast.CompletedTasks,
		Total:          forecast.TotalTasks,
		Velocity:       forecast.Velocity,
		EstimatedDays:  forecast.EstimatedDays,
		CompletionRate: forecast.CompletionRate(),
		Trend:          string(forecast.Trend.Direction),
		TrendSlope:     forecast.Trend.Slope,
		Confidence:     forecast.Trend.Confidence,
		CILow:          forecast.ConfidenceInterval.Low,
		CIExpected:     forecast.ConfidenceInterval.Expected,
		CIHigh:         forecast.ConfidenceInterval.High,
		DataPoints:     forecast.DataPoints,
	}

	for _, bp := range forecast.Burndown {
		resp.Burndown = append(resp.Burndown, burndownPt{
			Date:      bp.Date.Format("2006-01-02"),
			Actual:    bp.Actual,
			Projected: bp.Projected,
		})
	}

	for _, w := range forecast.Trend.Windows {
		resp.Windows = append(resp.Windows, windowPt{
			Days:     w.Days,
			Velocity: w.Velocity,
			Count:    w.Count,
		})
	}

	return resp, nil
}

func (s *Server) handleOrgStatus(ctx context.Context, args GetSpecArgs) (any, error) {
	orgSvc := s.orgSvc
	if orgSvc == nil {
		return "Org service not available.", nil
	}
	metrics, err := orgSvc.AggregateMetrics()
	if err != nil {
		return mcpErr("Failed to aggregate org metrics."), nil
	}
	return metrics, nil
}

func (s *Server) handleGitSync(ctx context.Context, args GitSyncArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	results, err := svc.Git.SyncMarkers(10)
	if err != nil {
		return mcpErr("Failed to sync git markers. Ensure you are in a git repository with commit history."), nil
	}
	return results, nil
}

func (s *Server) handleSync(ctx context.Context, args SyncArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	results, err := svc.Sync.SyncWithPlugin(args.PluginPath)
	if err != nil {
		return mcpErr("Failed to sync with plugin. Ensure the plugin binary exists and is executable."), nil
	}
	return results, nil
}

func (s *Server) handleExplainSpec(ctx context.Context, args ExplainSpecArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
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

func (s *Server) handleQuery(ctx context.Context, args QueryArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
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

func (s *Server) handleSuggestPriorities(ctx context.Context, args SuggestPrioritiesArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
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
		return mcpErr("Failed to load project at the given path."), nil
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
		return mcpErr("Failed to load project at the given path."), nil
	}
	if bad := requirePrompt(svc); bad != nil {
		return bad, nil
	}
	report, err := svc.Drift.DetectDrift(ctx)
	if err != nil {
		return mcpErr("Failed to detect drift. Ensure both spec and plan exist."), nil
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
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Drift.AcceptDrift(); err != nil {
		return mcpErr("Failed to accept drift. Ensure a spec exists."), nil
	}
	return "Drift accepted and spec snapshot locked.", nil
}

type AddFeatureArgs struct {
	Title       string `json:"title" jsonschema:"description=The title of the new feature"`
	Description string `json:"description" jsonschema:"description=A detailed description of the feature"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleAddFeature(ctx context.Context, args AddFeatureArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	spec, err := svc.Spec.AddFeature(args.Title, args.Description)
	if err != nil {
		return mcpErr("Failed to add feature. Ensure the project is initialized with a valid spec."), nil
	}
	return fmt.Sprintf("Successfully added feature '%s'. Intent synced to docs/backlog.md. Total features: %d", args.Title, len(spec.Features)), nil
}

func (s *Server) handleGetUsage(ctx context.Context, args GetUsageArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	usage, err := svc.Plan.GetUsage()
	if err != nil {
		return mcpErr("Failed to retrieve usage data. Ensure the project is initialized."), nil
	}
	return usage, nil
}

func (s *Server) handleApprovePlan(ctx context.Context, args ApprovePlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	err = svc.Plan.ApprovePlan()
	if err != nil {
		return mcpErr("Failed to approve plan. Ensure a plan has been generated."), nil
	}
	return "Plan approved successfully", nil
}

type QueryArgs struct {
	Question    string `json:"question" jsonschema:"description=A natural language question about the project"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type TransitionTaskArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=The ID of the task to transition"`
	Event       string `json:"event" jsonschema:"description=The transition event (start, complete, block, stop, unblock, reopen)"`
	Evidence    string `json:"evidence,omitempty" jsonschema:"description=Optional evidence for the transition (e.g. commit hash)"`
	Actor       string `json:"actor,omitempty" jsonschema:"description=The actor performing the transition (defaults to ai-agent)"`
	SessionID   string `json:"session_id,omitempty" jsonschema:"description=Identifier for the agent session performing this transition. Recorded in the audit trail so work can later be traced to a specific run."`
	Agent       string `json:"agent,omitempty" jsonschema:"description=Name of the agent performing this transition (e.g. claude-code, codex, cursor). Recorded in the audit trail."`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type AssignTaskArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=The ID of the task to assign"`
	Assignee    string `json:"assignee" jsonschema:"description=The person or agent to assign the task to"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
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
	Status      string   `json:"status,omitempty" jsonschema:"description=Filter by status (comma-separated: pending,blocked,in_progress,done,verified)"`
	Priority    string   `json:"priority,omitempty" jsonschema:"description=Filter by priority (comma-separated: high,medium,low)"`
	Ready       FlexBool `json:"ready,omitempty" jsonschema:"description=Show only tasks ready to start (unlocked + pending)"`
	Blocked     FlexBool `json:"blocked,omitempty" jsonschema:"description=Show only blocked tasks"`
	Active      FlexBool `json:"active,omitempty" jsonschema:"description=Show only in-progress tasks"`
	Limit       FlexInt  `json:"limit,omitempty" jsonschema:"description=Limit number of tasks returned"`
	JSON        FlexBool `json:"json,omitempty" jsonschema:"description=Return structured JSON output instead of text"`
	ProjectPath string   `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string   `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleAssignTask(ctx context.Context, args AssignTaskArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	err = svc.Task.AssignTask(ctx, args.TaskID, args.Assignee)
	if err != nil {
		return mcpErr(fmt.Sprintf("Failed to assign task '%s' to '%s': %v", args.TaskID, args.Assignee, err)), nil
	}
	return fmt.Sprintf("Task %s assigned to %s", args.TaskID, args.Assignee), nil
}

func (s *Server) handleTransitionTask(ctx context.Context, args TransitionTaskArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
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

func (s *Server) handleInit(ctx context.Context, args InitArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	err = svc.Init.InitializeProject(args.Name)
	if err != nil {
		return mcpErr("Failed to initialize project. Check directory permissions and ensure the name is valid."), nil
	}
	return fmt.Sprintf("Project %s initialized successfully", args.Name), nil
}

func (s *Server) handleGetSpec(ctx context.Context, args GetSpecArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	spec, err := svc.Spec.GetSpec()
	if err != nil {
		return mcpErr("Failed to load spec. Ensure the project is initialized with 'roady init'."), nil
	}
	return spec, nil
}

func (s *Server) handleGetPlan(ctx context.Context, args GetPlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	plan, err := svc.Plan.GetPlan()
	if err != nil {
		return mcpErr("Failed to load plan. Generate a plan first with 'roady plan generate'."), nil
	}
	return plan, nil
}

func (s *Server) handleGetState(ctx context.Context, args GetStateArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	state, err := svc.Plan.GetState()
	if err != nil {
		return mcpErr("Failed to load execution state. Ensure a plan has been generated."), nil
	}
	return state, nil
}

func (s *Server) handleGeneratePlan(ctx context.Context, args GeneratePlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	plan, err := svc.Plan.GeneratePlan(ctx)
	if err != nil {
		return mcpErr("Failed to generate plan. Ensure a spec exists with at least one feature."), nil
	}
	return fmt.Sprintf("Plan generated with %d tasks. Plan ID: %s", len(plan.Tasks), plan.ID), nil
}

func (s *Server) handleUpdatePlan(ctx context.Context, args UpdatePlanArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	plan, err := svc.Plan.UpdatePlan(args.Tasks)
	if err != nil {
		return mcpErr("Failed to update plan. Ensure the task list is valid and a spec exists."), nil
	}
	return fmt.Sprintf("Plan updated with %d tasks. Plan ID: %s", len(plan.Tasks), plan.ID), nil
}

func (s *Server) handleDetectDrift(ctx context.Context, args DetectDriftArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	report, err := svc.Drift.DetectDrift(ctx)
	if err != nil {
		return mcpErr("Failed to detect drift. Ensure both spec and plan exist."), nil
	}
	return report, nil
}

func (s *Server) handleStatus(ctx context.Context, args StatusArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	plan, err := svc.Plan.GetPlan()
	if err != nil {
		return mcpErr("Failed to load plan. Generate a plan first with 'roady plan generate'."), nil
	}
	if plan == nil {
		return "No plan found. Run roady_generate_plan first.", nil
	}

	state, err := svc.Plan.GetState()
	if err != nil {
		return mcpErr("Failed to load execution state. Ensure a plan has been generated."), nil
	}
	if state == nil {
		return "No execution state found.", nil
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

		jsonBytes, err := json.Marshal(output)
		if err != nil {
			return mcpErr("Failed to format status output."), nil
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

	return statusStr, nil
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

func (s *Server) handleCheckPolicy(ctx context.Context, args CheckPolicyArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	vioations, err := svc.Policy.CheckCompliance()
	if err != nil {
		return mcpErr("Failed to check policy compliance. Ensure a policy.yaml and plan exist."), nil
	}
	if len(vioations) == 0 {
		return "No policy violations found.", nil
	}
	return vioations, nil
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

func (s *Server) StartHTTP(addr string) error {
	return s.ServeHTTP(context.Background(), addr)
}

func (s *Server) StartWebSocket(addr string) error {
	return s.ServeWebSocket(context.Background(), addr)
}

func (s *Server) ServeStdio(ctx context.Context) error {
	return mcp.ServeStdio(ctx, s.mcpServer, s.serveMiddleware())
}

func (s *Server) ServeHTTP(ctx context.Context, addr string) error {
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

func (s *Server) StartGRPC(addr string) error {
	return s.ServeGRPC(context.Background(), addr)
}

func (s *Server) ServeGRPC(ctx context.Context, addr string) error {
	return mcp.ServeGRPCWithMiddleware(ctx, s.mcpServer, addr,
		nil,
		s.serveMiddleware(),
	)
}

// Dependency MCP handlers

func (s *Server) handleDepsList(ctx context.Context, args GetSpecArgs) (any, error) {
	deps, err := s.depSvc.ListDependencies()
	if err != nil {
		return mcpErr("Failed to list dependencies. Ensure .roady/deps.yaml exists."), nil
	}
	return deps, nil
}

func (s *Server) handleDepsScan(ctx context.Context, args GetSpecArgs) (any, error) {
	result, err := s.depSvc.ScanDependentRepos(nil)
	if err != nil {
		return mcpErr("Failed to scan dependent repositories. Check that dependency paths are valid."), nil
	}
	return result, nil
}

type DepsGraphArgs struct {
	CheckCycles bool `json:"check_cycles,omitempty" jsonschema:"description=Whether to check for cyclic dependencies"`
}

func (s *Server) handleDepsGraph(ctx context.Context, args DepsGraphArgs) (any, error) {
	summary, err := s.depSvc.GetDependencySummary()
	if err != nil {
		return mcpErr("Failed to get dependency summary. Ensure .roady/deps.yaml exists."), nil
	}

	response := map[string]any{
		"summary": summary,
	}

	if args.CheckCycles {
		hasCycle, err := s.depSvc.CheckForCycles()
		if err != nil {
			return mcpErr("Failed to check for dependency cycles."), nil
		}
		response["has_cycle"] = hasCycle
	}

	return response, nil
}

// Debt MCP handlers

func (s *Server) handleDebtReport(ctx context.Context, args GetSpecArgs) (any, error) {
	report, err := s.debtSvc.GetDebtReport(ctx)
	if err != nil {
		return mcpErr("Failed to generate debt report. Ensure drift detection has been run."), nil
	}
	return report, nil
}

func (s *Server) handleDebtSummary(ctx context.Context, args GetSpecArgs) (any, error) {
	summary, err := s.debtSvc.GetDebtSummary(ctx)
	if err != nil {
		return mcpErr("Failed to get debt summary. Ensure drift detection has been run."), nil
	}
	return summary, nil
}

// handleAuditTrail answers "which agent worked on this, and what proves it".
// The trail was CLI-only until now, which left the agents this feature was
// built for unable to ask.
func (s *Server) handleAuditTrail(ctx context.Context, args AuditTrailArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
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
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	now := time.Now()
	if n, ok := strings.CutSuffix(value, "d"); ok {
		days, convErr := strconv.Atoi(n)
		if convErr != nil || days <= 0 {
			return time.Time{}, fmt.Errorf("invalid since %q: expected a form like 7d, 2w, or 2026-07-01", value)
		}
		return now.AddDate(0, 0, -days), nil
	}
	if n, ok := strings.CutSuffix(value, "w"); ok {
		weeks, convErr := strconv.Atoi(n)
		if convErr != nil || weeks <= 0 {
			return time.Time{}, fmt.Errorf("invalid since %q: expected a form like 7d, 2w, or 2026-07-01", value)
		}
		return now.AddDate(0, 0, -weeks*7), nil
	}

	parsed, parseErr := time.Parse("2006-01-02", value)
	if parseErr != nil {
		return time.Time{}, fmt.Errorf("invalid since %q: expected a form like 7d, 2w, or 2026-07-01", value)
	}
	return parsed, nil
}

func (s *Server) handleStickyDrift(ctx context.Context, args GetSpecArgs) (any, error) {
	items, err := s.debtSvc.GetStickyDrift()
	if err != nil {
		return mcpErr("Failed to get sticky drift items. Ensure drift history exists."), nil
	}
	return items, nil
}

type DebtTrendArgs struct {
	Days int `json:"days,omitempty" jsonschema:"description=Analysis window in days (default: 30)"`
}

func (s *Server) handleDebtTrend(ctx context.Context, args DebtTrendArgs) (any, error) {
	days := args.Days
	if days <= 0 {
		days = 30
	}
	trend, err := s.debtSvc.GetDriftTrend(days)
	if err != nil {
		return mcpErr("Failed to get debt trend. Ensure drift history exists."), nil
	}
	return trend, nil
}

// Coordinator-based snapshot and task query handlers (v0.6.0)

func (s *Server) handleGetSnapshot(ctx context.Context, args GetSnapshotArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	snapshot, err := svc.Plan.GetProjectSnapshot(ctx)
	if err != nil {
		return mcpErr("Failed to get project snapshot. Ensure a plan and state exist."), nil
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
		return mcpErr("Failed to load project at the given path."), nil
	}

	status := args.Status
	if status == "" {
		status = "ready"
	}

	switch status {
	case "ready":
		tasks, err := svc.Plan.GetReadyTasks(ctx)
		if err != nil {
			return mcpErr("Failed to get ready tasks. Ensure a plan and state exist."), nil
		}
		return filterByAssignee(tasks, args.Assignee), nil
	case "in_progress":
		tasks, err := svc.Plan.GetInProgressTasks(ctx)
		if err != nil {
			return mcpErr("Failed to get in-progress tasks. Ensure a plan and state exist."), nil
		}
		return filterByAssignee(tasks, args.Assignee), nil
	case "blocked":
		tasks, err := svc.Plan.GetBlockedTasks(ctx)
		if err != nil {
			return mcpErr("Failed to get blocked tasks. Ensure a plan and state exist."), nil
		}
		return filterByAssignee(tasks, args.Assignee), nil
	case "unassigned":
		tasks, err := svc.Plan.GetTasksByOwner(ctx, "")
		if err != nil {
			return mcpErr("Failed to get unassigned tasks. Ensure a plan and state exist."), nil
		}
		return tasks, nil
	case "all":
		ready, err := svc.Plan.GetReadyTasks(ctx)
		if err != nil {
			return mcpErr("Failed to load tasks. Ensure a plan and state exist."), nil
		}
		inProgress, err := svc.Plan.GetInProgressTasks(ctx)
		if err != nil {
			return mcpErr("Failed to load tasks. Ensure a plan and state exist."), nil
		}
		blocked, err := svc.Plan.GetBlockedTasks(ctx)
		if err != nil {
			return mcpErr("Failed to load tasks. Ensure a plan and state exist."), nil
		}
		return map[string]any{
			"ready":       filterByAssignee(ready, args.Assignee),
			"in_progress": filterByAssignee(inProgress, args.Assignee),
			"blocked":     filterByAssignee(blocked, args.Assignee),
		}, nil
	default:
		return mcpErr("Invalid status. Use ready, in_progress, blocked, unassigned, or all."), nil
	}
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

func (s *Server) handleGetReadyTasks(ctx context.Context, args GetReadyTasksArgs) (any, error) {
	return s.handleTasks(ctx, TasksArgs{Status: "ready", ProjectPath: args.ProjectPath})
}

func (s *Server) handleGetBlockedTasks(ctx context.Context, args GetBlockedTasksArgs) (any, error) {
	return s.handleTasks(ctx, TasksArgs{Status: "blocked", ProjectPath: args.ProjectPath})
}

func (s *Server) handleGetInProgressTasks(ctx context.Context, args GetInProgressTasksArgs) (any, error) {
	return s.handleTasks(ctx, TasksArgs{Status: "in_progress", ProjectPath: args.ProjectPath})
}

// --- Workspace Sync Handlers ---

func (s *Server) handleWorkspacePush(ctx context.Context, args WorkspacePushArgs) (any, error) {
	root := s.root
	auditSvc := s.auditSvc
	if args.ProjectPath != "" && args.ProjectPath != s.root {
		overrideSvc, err := s.servicesForPath(args.ProjectPath, args.Project)
		if err != nil {
			return mcpErr("Failed to load project at the given path."), nil
		}
		root = args.ProjectPath
		auditSvc = overrideSvc.Audit
	}
	svc := application.NewWorkspaceSyncService(root, auditSvc)
	result, err := svc.Push(ctx)
	if err != nil {
		return mcpErr(fmt.Sprintf("workspace push failed: %s", err)), nil
	}
	return result, nil
}

func (s *Server) handleWorkspacePull(ctx context.Context, args WorkspacePullArgs) (any, error) {
	root := s.root
	auditSvc := s.auditSvc
	if args.ProjectPath != "" && args.ProjectPath != s.root {
		overrideSvc, err := s.servicesForPath(args.ProjectPath, args.Project)
		if err != nil {
			return mcpErr("Failed to load project at the given path."), nil
		}
		root = args.ProjectPath
		auditSvc = overrideSvc.Audit
	}
	svc := application.NewWorkspaceSyncService(root, auditSvc)
	result, err := svc.Pull(ctx)
	if err != nil {
		return mcpErr(fmt.Sprintf("workspace pull failed: %s", err)), nil
	}
	return result, nil
}

// --- Smart Decompose Handler ---

func (s *Server) handleSmartDecompose(ctx context.Context, args SmartDecomposeArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
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

// --- Team Handlers ---

type TeamAddArgs struct {
	Name        string `json:"name" jsonschema:"description=The name of the team member"`
	Role        string `json:"role" jsonschema:"description=The role: admin, member, or viewer"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

type TeamRemoveArgs struct {
	Name        string `json:"name" jsonschema:"description=The name of the team member to remove"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleTeamList(ctx context.Context, args GetSpecArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	cfg, err := svc.Team.ListMembers()
	if err != nil {
		return mcpErr("failed to list team members"), nil
	}
	return cfg, nil
}

func (s *Server) handleTeamAdd(ctx context.Context, args TeamAddArgs) (any, error) {
	if args.Name == "" {
		return mcpErr("name is required"), nil
	}
	if args.Role == "" {
		return mcpErr("role is required"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Team.AddMember(args.Name, teamRole(args.Role)); err != nil {
		return mcpErr(fmt.Sprintf("failed to add member: %s", err)), nil
	}
	return fmt.Sprintf("Member %s added with role %s", args.Name, args.Role), nil
}

func (s *Server) handleTeamRemove(ctx context.Context, args TeamRemoveArgs) (any, error) {
	if args.Name == "" {
		return mcpErr("name is required"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Team.RemoveMember(args.Name); err != nil {
		return mcpErr(fmt.Sprintf("failed to remove member: %s", err)), nil
	}
	return fmt.Sprintf("Member %s removed", args.Name), nil
}

type RateListArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleRateList(ctx context.Context, args RateListArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	config, err := svc.Billing.ListRates()
	if err != nil {
		return mcpErr(fmt.Sprintf("failed to list rates: %s", err)), nil
	}
	return config, nil
}

type RateAddArgs struct {
	ID          string  `json:"id" jsonschema:"description=Rate ID (e.g., senior, junior)"`
	Name        string  `json:"name" jsonschema:"description=Rate name"`
	HourlyRate  float64 `json:"hourly_rate" jsonschema:"description=Hourly rate amount"`
	IsDefault   bool    `json:"is_default" jsonschema:"description=Set as default rate"`
	ProjectPath string  `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string  `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleRateAdd(ctx context.Context, args RateAddArgs) (any, error) {
	if args.ID == "" || args.Name == "" {
		return mcpErr("id and name are required"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	rate := billing.Rate{
		ID:         args.ID,
		Name:       args.Name,
		HourlyRate: args.HourlyRate,
		IsDefault:  args.IsDefault,
	}
	if err := svc.Billing.AddRate(rate); err != nil {
		return mcpErr(fmt.Sprintf("failed to add rate: %s", err)), nil
	}
	return fmt.Sprintf("Rate %s added: %s - $%.2f/hr", args.ID, args.Name, args.HourlyRate), nil
}

type TaskLogTimeArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=Task ID"`
	Minutes     int    `json:"minutes" jsonschema:"description=Minutes to log"`
	RateID      string `json:"rate_id" jsonschema:"description=Rate ID (optional)"`
	Description string `json:"description" jsonschema:"description=Description (optional)"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleTaskLogTime(ctx context.Context, args TaskLogTimeArgs) (any, error) {
	if args.TaskID == "" || args.Minutes <= 0 {
		return mcpErr("task_id and minutes are required"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Billing.LogTime(args.TaskID, args.RateID, args.Minutes, args.Description); err != nil {
		return mcpErr(fmt.Sprintf("failed to log time: %s", err)), nil
	}
	return fmt.Sprintf("Logged %d minutes to task %s", args.Minutes, args.TaskID), nil
}

type CostReportArgs struct {
	TaskID      string `json:"task_id" jsonschema:"description=Filter by task ID (optional)"`
	Period      string `json:"period" jsonschema:"description=Filter by period (optional)"`
	Format      string `json:"format" jsonschema:"description=Output format: text, json, csv, markdown"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleCostReport(ctx context.Context, args CostReportArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	opts := application.CostReportOpts{
		TaskID: args.TaskID,
		Period: args.Period,
		Format: args.Format,
	}
	report, err := svc.Billing.GetCostReport(opts)
	if err != nil {
		return mcpErr(fmt.Sprintf("failed to generate cost report: %s", err)), nil
	}
	if report == nil {
		return "No time entries found", nil
	}
	return report, nil
}

type CostBudgetArgs struct {
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleCostBudget(ctx context.Context, args CostBudgetArgs) (any, error) {
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	status, err := svc.Billing.GetBudgetStatus()
	if err != nil {
		return mcpErr(fmt.Sprintf("failed to get budget status: %s", err)), nil
	}
	if status == nil {
		return "No budget configured. Set budget_hours in policy.yaml.", nil
	}
	return status, nil
}

type RateRemoveArgs struct {
	ID          string `json:"id" jsonschema:"description=Rate ID to remove"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleRateRemove(ctx context.Context, args RateRemoveArgs) (any, error) {
	if args.ID == "" {
		return mcpErr("rate id is required"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Billing.RemoveRate(args.ID); err != nil {
		return mcpErr(fmt.Sprintf("failed to remove rate: %s", err)), nil
	}
	return fmt.Sprintf("Rate %s removed", args.ID), nil
}

type RateSetDefaultArgs struct {
	ID          string `json:"id" jsonschema:"description=Rate ID to set as default"`
	ProjectPath string `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleRateSetDefault(ctx context.Context, args RateSetDefaultArgs) (any, error) {
	if args.ID == "" {
		return mcpErr("rate id is required"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Billing.SetDefaultRate(args.ID); err != nil {
		return mcpErr(fmt.Sprintf("failed to set default rate: %s", err)), nil
	}
	return fmt.Sprintf("Rate %s set as default", args.ID), nil
}

type RateTaxArgs struct {
	Name        string  `json:"name" jsonschema:"description=Tax name (e.g., VAT, Sales Tax)"`
	Percent     float64 `json:"percent" jsonschema:"description=Tax percentage (e.g., 20 for 20%%)"`
	Included    bool    `json:"included" jsonschema:"description=Tax is included in rate"`
	ProjectPath string  `json:"project_path,omitempty" jsonschema:"description=Path to the roady project directory (default: server root)"`
	Project     string  `json:"project,omitempty" jsonschema:"description=Sub-project name under .roady/projects/<name>/ (default: root project)"`
}

func (s *Server) handleRateTax(ctx context.Context, args RateTaxArgs) (any, error) {
	if args.Name == "" {
		return mcpErr("tax name is required"), nil
	}
	if args.Percent < 0 || args.Percent > 100 {
		return mcpErr("tax percent must be between 0 and 100"), nil
	}
	svc, err := s.servicesForPath(args.ProjectPath, args.Project)
	if err != nil {
		return mcpErr("Failed to load project at the given path."), nil
	}
	if err := svc.Billing.SetTax(args.Name, args.Percent, args.Included); err != nil {
		return mcpErr(fmt.Sprintf("failed to set tax: %s", err)), nil
	}
	return fmt.Sprintf("Tax configured: %s at %.1f%%", args.Name, args.Percent), nil
}

func teamRole(s string) team.Role {
	return team.Role(s)
}

// orEmpty returns the slice or an empty slice if nil (for clean JSON output).
func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
