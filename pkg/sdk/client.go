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

// Connect opens the MCP session the way MCP 2026-07-28 defines it: it calls
// server/discover, and falls back to the initialize handshake only when the
// server predates discover. roady's HTTP transport is stateless Streamable
// HTTP, which retires initialize, so this is the call to use.
func (c *Client) Connect(ctx context.Context) (*client.ServerInfo, error) {
	return c.mcp.Connect(ctx)
}

// Initialize performs the legacy MCP initialize handshake.
//
// Deprecated: MCP 2026-07-28 retires initialize, and roady's HTTP server
// rejects it. Use Connect, which discovers the server and falls back to
// initialize only for servers that predate discover.
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

// --- Planning ---

// --- Drift ---

// DetectDrift detects discrepancies between spec and plan.
func (c *Client) DetectDrift(ctx context.Context) (*DriftReport, error) {
	res, err := c.call(ctx, "roady_drift", map[string]any{"action": "detect"})
	if err != nil {
		return nil, err
	}
	return unmarshalText[DriftReport](res)
}

// --- Tasks ---

// TransitionTask transitions a task to a new state.
func (c *Client) TransitionTask(ctx context.Context, taskID, event, evidence string) (string, error) {
	args := map[string]any{"action": event, "task_id": taskID}
	if evidence != "" {
		args["evidence"] = evidence
	}
	res, err := c.call(ctx, "roady_task", args)
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

// --- Features ---

// QueryProject asks a natural language question about the project.
func (c *Client) QueryProject(ctx context.Context, question string) (string, error) {
	res, err := c.call(ctx, "roady_query", map[string]any{"question": question})
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- Coordinator ---

// --- Forecast / Org ---

// --- Sync ---

// --- Deps ---

// --- Debt ---

// --- Plugins ---

// --- Messaging ---

// --- Workspace Sync ---

// --- Smart Decompose ---

// --- Team ---

// --- The agent loop ---

// Next returns the brief on the caller's current task, or the ready task to
// start next. owner defaults to ai-agent on the server.
func (c *Client) Next(ctx context.Context, owner string) (string, error) {
	var args map[string]any
	if owner != "" {
		args = map[string]any{"owner": owner}
	}
	return c.text(ctx, "roady_next", args)
}

// Capture records features, requirements and tasks in one write. doc is the
// capture document ({"features": [...], "tasks": [...]}); dryRun previews.
// The result is the capture report as JSON.
func (c *Client) Capture(ctx context.Context, doc map[string]any, dryRun bool) (string, error) {
	args := map[string]any{}
	for k, v := range doc {
		args[k] = v
	}
	if dryRun {
		args["dry_run"] = true
	}
	return c.text(ctx, "roady_capture", args)
}

// PlanImport imports a plan file an agent wrote (markdown, Kiro tasks.md,
// Codex ExecPlan) as tasks. opts may carry format, feature_id, parallel,
// include_done and dry_run.
func (c *Client) PlanImport(ctx context.Context, path string, opts map[string]any) (string, error) {
	args := map[string]any{"path": path}
	for k, v := range opts {
		args[k] = v
	}
	return c.Plan(ctx, "import", args)
}

// TaskCheck runs a task's acceptance check and records the result.
func (c *Client) TaskCheck(ctx context.Context, taskID string) (string, error) {
	return c.Task(ctx, "check", map[string]any{"task_id": taskID})
}

// DispatchTask hands a ready task to a subagent and claims it unless dryRun.
func (c *Client) DispatchTask(ctx context.Context, taskID, agent string, dryRun bool) (string, error) {
	args := map[string]any{"task_id": taskID, "agent": agent}
	if dryRun {
		args["dry_run"] = true
	}
	return c.Task(ctx, "dispatch", args)
}

// SemanticDrift returns the prompt for judging whether implementations still
// mean what their requirements say, with the questions it covers.
func (c *Client) SemanticDrift(ctx context.Context) (string, error) {
	return c.Drift(ctx, "semantic", nil)
}

// RecordSemanticDrift records judgements on SemanticDrift's questions. Each
// judgement is {"requirement_id", "agrees", "explanation"}.
func (c *Client) RecordSemanticDrift(ctx context.Context, judgements []map[string]any) (string, error) {
	return c.Drift(ctx, "record", map[string]any{"judgements": judgements})
}

func (c *Client) text(ctx context.Context, tool string, args map[string]any) (string, error) {
	res, err := c.call(ctx, tool, args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// --- One method per CLI noun ---
//
// The server has one tool per CLI noun with the CLI's verbs as actions
// (`roady plan approve` is Plan(ctx, "approve", nil)). args carries the
// action's other arguments. Decisions (plan approve/reject/prune, drift
// accept, spec lock/import/analyze, state rebuild) run only after the user
// confirms in their MCP client; without that the call returns a ToolError.

// Task runs `roady task <action>`: start, complete, block, unblock, stop,
// reopen, verify, check, dispatch, list.
func (c *Client) Task(ctx context.Context, action string, args map[string]any) (string, error) {
	return c.noun(ctx, "roady_task", action, args)
}

// Plan runs `roady plan <action>`: get, generate, approve, reject, prune,
// prioritize, decompose, import.
func (c *Client) Plan(ctx context.Context, action string, args map[string]any) (string, error) {
	return c.noun(ctx, "roady_plan", action, args)
}

// Spec runs `roady spec <action>`: get, add, analyze, explain, import, lock,
// review, validate.
func (c *Client) Spec(ctx context.Context, action string, args map[string]any) (string, error) {
	return c.noun(ctx, "roady_spec", action, args)
}

// Drift runs `roady drift <action>`: detect, accept, explain, semantic, record.
func (c *Client) Drift(ctx context.Context, action string, args map[string]any) (string, error) {
	return c.noun(ctx, "roady_drift", action, args)
}

// Audit runs `roady audit <action>`: verify, trail.
func (c *Client) Audit(ctx context.Context, action string, args map[string]any) (string, error) {
	return c.noun(ctx, "roady_audit", action, args)
}

// State runs `roady state <action>`: get, rebuild.
func (c *Client) State(ctx context.Context, action string, args map[string]any) (string, error) {
	return c.noun(ctx, "roady_state", action, args)
}

// PolicyCheck runs `roady policy check`.
func (c *Client) PolicyCheck(ctx context.Context) (string, error) {
	return c.noun(ctx, "roady_policy", "check", nil)
}

// GitSync runs `roady git sync`.
func (c *Client) GitSync(ctx context.Context) (string, error) {
	return c.noun(ctx, "roady_git", "sync", nil)
}

// Init runs `roady init <name>`.
func (c *Client) Init(ctx context.Context, name string) (string, error) {
	return c.text(ctx, "roady_init", map[string]any{"name": name})
}

func (c *Client) noun(ctx context.Context, tool, action string, args map[string]any) (string, error) {
	all := map[string]any{"action": action}
	for k, v := range args {
		all[k] = v
	}
	return c.text(ctx, tool, all)
}
