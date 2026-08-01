package sdk

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.klarlabs.de/fortify/retry"
	"go.klarlabs.de/mcp/client"
)

// Client is a typed Go client for the Roady MCP server.
type Client struct {
	mcp      *client.Client
	retryCfg retry.Config
	timeout  time.Duration
}

// NewClient creates a new SDK client wrapping the given MCP transport.
func NewClient(transport client.Transport, opts ...Option) *Client {
	o := defaultOptions()
	for _, fn := range opts {
		fn(&o)
	}
	return &Client{
		mcp:     client.New(transport, client.WithTimeout(o.timeout)),
		timeout: o.timeout,
		retryCfg: retry.Config{
			MaxAttempts:   o.maxAttempts,
			InitialDelay:  o.initialDelay,
			BackoffPolicy: retry.BackoffExponential,
		},
	}
}

// Initialize performs the MCP initialize handshake.
func (c *Client) Initialize(ctx context.Context) (*client.ServerInfo, error) {
	return c.mcp.Initialize(ctx)
}

// Close closes the underlying transport.
func (c *Client) Close() error {
	return c.mcp.Close()
}

// call invokes a tool with retry.
func (c *Client) call(ctx context.Context, tool string, args map[string]any) (*client.ToolResult, error) {
	r := retry.New[*client.ToolResult](c.retryCfg)
	result, err := r.Execute(ctx, func(ctx context.Context) (*client.ToolResult, error) {
		return c.mcp.CallTool(ctx, tool, args)
	})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", tool, err)
	}
	if result.IsError {
		msg := ""
		if len(result.Content) > 0 {
			msg = result.Content[0].Text
		}
		return nil, &ToolError{Tool: tool, Message: msg}
	}
	return result, nil
}

// unmarshalText extracts Content[0].Text from a tool result and unmarshals it as JSON.
func unmarshalText[T any](result *client.ToolResult) (*T, error) {
	text, err := textResult(result)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return &v, nil
}

// textResult extracts Content[0].Text from a tool result.
func textResult(result *client.ToolResult) (string, error) {
	if len(result.Content) == 0 {
		return "", ErrNoContent
	}
	return result.Content[0].Text, nil
}

// --- Schema ---

// GetSchema reads the roady://schema resource from the server.
func (c *Client) GetSchema(ctx context.Context) (*SchemaInfo, error) {
	rc, err := c.mcp.ReadResource(ctx, "roady://schema")
	if err != nil {
		return nil, fmt.Errorf("read schema resource: %w", err)
	}
	var info SchemaInfo
	if err := json.Unmarshal([]byte(rc.Text), &info); err != nil {
		return nil, fmt.Errorf("unmarshal schema: %w", err)
	}
	return &info, nil
}

// Compatible checks if the server schema is compatible with this SDK version.
// Returns nil if compatible, error with details if not.
func (c *Client) Compatible(ctx context.Context) error {
	info, err := c.GetSchema(ctx)
	if err != nil {
		return fmt.Errorf("check compatibility: %w", err)
	}
	serverMajor := majorVersion(info.SchemaVersion)
	if serverMajor != SupportedSchemaMajor {
		return fmt.Errorf("incompatible schema: server=%s (major %s), sdk supports major %s",
			info.SchemaVersion, serverMajor, SupportedSchemaMajor)
	}
	return nil
}

// majorVersion extracts the major version from a semver string.
func majorVersion(v string) string {
	for i, ch := range v {
		if ch == '.' {
			return v[:i]
		}
	}
	return v
}

// --- Project ---

// Init initializes a new roady project.
func (c *Client) Init(ctx context.Context, name string) (string, error) {
	res, err := c.call(ctx, "roady_init", map[string]any{"name": name})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// GetSpec retrieves the current product specification.
func (c *Client) GetSpec(ctx context.Context) (*Spec, error) {
	res, err := c.call(ctx, "roady_get_spec", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[Spec](res)
}

// GetPlan retrieves the current execution plan.
func (c *Client) GetPlan(ctx context.Context) (*Plan, error) {
	res, err := c.call(ctx, "roady_get_plan", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[Plan](res)
}

// GetState retrieves the current execution state.
func (c *Client) GetState(ctx context.Context) (*ExecutionState, error) {
	res, err := c.call(ctx, "roady_get_state", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[ExecutionState](res)
}

// --- Planning ---

// GeneratePlan generates a plan from the spec using 1:1 heuristic.
func (c *Client) GeneratePlan(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_generate_plan", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// UpdatePlan updates the plan with the given tasks.
func (c *Client) UpdatePlan(ctx context.Context, tasks []Task) (string, error) {
	res, err := c.call(ctx, "roady_update_plan", map[string]any{"tasks": tasks})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// ApprovePlan approves the current plan for execution.
func (c *Client) ApprovePlan(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_approve_plan", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Drift ---

// DetectDrift detects discrepancies between spec and plan.
func (c *Client) DetectDrift(ctx context.Context) (*DriftReport, error) {
	res, err := c.call(ctx, "roady_detect_drift", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[DriftReport](res)
}

// AcceptDrift accepts drift and locks the spec snapshot.
func (c *Client) AcceptDrift(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_accept_drift", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// ExplainDrift provides an AI-generated explanation of current drift.
func (c *Client) ExplainDrift(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_explain_drift", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Tasks ---

// TransitionTask transitions a task to a new state.
func (c *Client) TransitionTask(ctx context.Context, taskID, event, evidence string) (string, error) {
	args := map[string]any{"task_id": taskID, "event": event}
	if evidence != "" {
		args["evidence"] = evidence
	}
	res, err := c.call(ctx, "roady_transition_task", args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// AssignTask assigns a task to a person or agent without changing its status.
func (c *Client) AssignTask(ctx context.Context, taskID, assignee string) (string, error) {
	res, err := c.call(ctx, "roady_assign_task", map[string]any{"task_id": taskID, "assignee": assignee})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// Status returns project status with optional filters.
func (c *Client) Status(ctx context.Context, args map[string]any) (string, error) {
	res, err := c.call(ctx, "roady_status", args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// CheckPolicy checks plan compliance with execution policies.
func (c *Client) CheckPolicy(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_check_policy", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Features ---

// AddFeature adds a new feature to the product specification.
func (c *Client) AddFeature(ctx context.Context, title, description string) (string, error) {
	res, err := c.call(ctx, "roady_add_feature", map[string]any{"title": title, "description": description})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// QueryProject asks a natural language question about the project.
func (c *Client) QueryProject(ctx context.Context, question string) (string, error) {
	res, err := c.call(ctx, "roady_query", map[string]any{"question": question})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// SuggestPriorities returns AI-powered priority suggestions for plan tasks.
func (c *Client) SuggestPriorities(ctx context.Context) (*PrioritySuggestions, error) {
	res, err := c.call(ctx, "roady_suggest_priorities", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[PrioritySuggestions](res)
}

// ReviewSpec performs an AI-powered quality review of the specification.
func (c *Client) ReviewSpec(ctx context.Context) (*SpecReview, error) {
	res, err := c.call(ctx, "roady_review_spec", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[SpecReview](res)
}

// ExplainSpec provides an AI-generated walkthrough of the specification.
func (c *Client) ExplainSpec(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_explain_spec", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// GetUsage retrieves project usage and telemetry statistics.
func (c *Client) GetUsage(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_get_usage", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Coordinator ---

// GetSnapshot returns a consistent project snapshot.
func (c *Client) GetSnapshot(ctx context.Context) (*Snapshot, error) {
	res, err := c.call(ctx, "roady_get_snapshot", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[Snapshot](res)
}

// GetReadyTasks returns tasks that are ready to start.
func (c *Client) GetReadyTasks(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_tasks", map[string]any{"status": "ready"})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// GetBlockedTasks returns tasks that are currently blocked.
func (c *Client) GetBlockedTasks(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_tasks", map[string]any{"status": "blocked"})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// GetInProgressTasks returns tasks currently in progress.
func (c *Client) GetInProgressTasks(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_tasks", map[string]any{"status": "in_progress"})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Forecast / Org ---

// Forecast predicts project completion based on current velocity.
func (c *Client) Forecast(ctx context.Context) (*Forecast, error) {
	res, err := c.call(ctx, "roady_forecast", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[Forecast](res)
}

// OrgStatus returns status overview of all Roady projects.
func (c *Client) OrgStatus(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_org_status", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// GitSync synchronizes task statuses from git commit markers.
func (c *Client) GitSync(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_git_sync", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// OrgPolicy returns merged policy for a project.
func (c *Client) OrgPolicy(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_org_policy", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// OrgDetectDrift detects drift across all projects.
func (c *Client) OrgDetectDrift(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_org_detect_drift", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Sync ---

// Sync syncs the plan with an external system via a plugin binary.
func (c *Client) Sync(ctx context.Context, pluginPath string) (string, error) {
	res, err := c.call(ctx, "roady_sync", map[string]any{"plugin_path": pluginPath})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Deps ---

// DepsList lists all cross-repository dependencies.
func (c *Client) DepsList(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_deps_list", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// DepsScan scans health status of dependent repositories.
func (c *Client) DepsScan(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_deps_scan", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// DepsGraph returns dependency graph summary with optional cycle detection.
func (c *Client) DepsGraph(ctx context.Context, checkCycles bool) (string, error) {
	args := map[string]any{}
	if checkCycles {
		args["check_cycles"] = true
	}
	res, err := c.call(ctx, "roady_deps_graph", args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Debt ---

// DebtReport generates a comprehensive debt report.
func (c *Client) DebtReport(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_debt_report", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// DebtSummary returns a quick overview of debt status.
func (c *Client) DebtSummary(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_debt_summary", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// StickyDrift returns unresolved drift items older than 7 days.
func (c *Client) StickyDrift(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_drift_recurring", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// AuditTrailQuery selects the subject of an evidence trail. Exactly one of
// TaskID, Agent, or Session identifies it; TaskID combined with Agent narrows
// to what that agent did to that task.
type AuditTrailQuery struct {
	TaskID  string
	Agent   string
	Session string
	// Since accepts a relative window ("7d", "2w") or an absolute date
	// ("2026-07-01"). Empty means the whole history.
	Since string
}

// AuditTrail returns the evidence trail for a task, agent, or session:
// chain-integrity status, findings, the task's evidence and spec citation,
// who acted, and every recorded event.
//
// The trail attests to a tamper-evident record of what was asserted, not to
// who acted — actor and agent are caller-supplied and unauthenticated.
func (c *Client) AuditTrail(ctx context.Context, q AuditTrailQuery) (string, error) {
	args := map[string]any{}
	if q.TaskID != "" {
		args["task_id"] = q.TaskID
	}
	if q.Agent != "" {
		args["agent"] = q.Agent
	}
	if q.Session != "" {
		args["session_id"] = q.Session
	}
	if q.Since != "" {
		args["since"] = q.Since
	}

	res, err := c.call(ctx, "roady_audit_trail", args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// DebtTrend analyzes drift trend over the given number of days.
func (c *Client) DebtTrend(ctx context.Context, days int) (string, error) {
	args := map[string]any{}
	if days > 0 {
		args["days"] = days
	}
	res, err := c.call(ctx, "roady_debt_trend", args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Plugins ---

// PluginList lists all registered plugins.
func (c *Client) PluginList(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_plugin_list", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// PluginValidate validates a registered plugin.
func (c *Client) PluginValidate(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_plugin_validate", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// PluginStatus checks health status of plugins.
func (c *Client) PluginStatus(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_plugin_status", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Messaging ---

// MessagingList lists configured messaging adapters.
func (c *Client) MessagingList(ctx context.Context) (string, error) {
	res, err := c.call(ctx, "roady_messaging_list", nil)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Workspace Sync ---

// WorkspacePush commits and pushes .roady/ changes to git remote.
func (c *Client) WorkspacePush(ctx context.Context) (*SyncResult, error) {
	res, err := c.call(ctx, "roady_workspace_push", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[SyncResult](res)
}

// WorkspacePull pulls remote .roady/ changes with conflict detection.
func (c *Client) WorkspacePull(ctx context.Context) (*SyncResult, error) {
	res, err := c.call(ctx, "roady_workspace_pull", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[SyncResult](res)
}

// --- Smart Decompose ---

// SmartDecompose performs AI-powered context-aware task decomposition.
func (c *Client) SmartDecompose(ctx context.Context) (*SmartPlan, error) {
	res, err := c.call(ctx, "roady_plan_decompose", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[SmartPlan](res)
}

// --- Team ---

// TeamList returns the current team configuration.
func (c *Client) TeamList(ctx context.Context) (*TeamConfig, error) {
	res, err := c.call(ctx, "roady_team_list", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[TeamConfig](res)
}

// TeamAdd adds or updates a team member with a role.
func (c *Client) TeamAdd(ctx context.Context, name, role string) (string, error) {
	res, err := c.call(ctx, "roady_team_add", map[string]any{"name": name, "role": role})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// TeamRemove removes a team member.
func (c *Client) TeamRemove(ctx context.Context, name string) (string, error) {
	res, err := c.call(ctx, "roady_team_remove", map[string]any{"name": name})
	if err != nil {
		return "", err
	}
	return textResult(res)
}
