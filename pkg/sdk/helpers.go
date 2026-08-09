package sdk

import "context"

// StatusRequest provides typed parameters for the Status method.
type StatusRequest struct {
	Status   string `json:"status,omitempty"`
	Priority string `json:"priority,omitempty"`
	Ready    bool   `json:"ready,omitempty"`
	Blocked  bool   `json:"blocked,omitempty"`
	Active   bool   `json:"active,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// StatusTyped returns project status as a typed StatusResult.
func (c *Client) StatusTyped(ctx context.Context, req StatusRequest) (*StatusResult, error) {
	args := map[string]any{"json": true}
	if req.Status != "" {
		args["status"] = req.Status
	}
	if req.Priority != "" {
		args["priority"] = req.Priority
	}
	if req.Ready {
		args["ready"] = true
	}
	if req.Blocked {
		args["blocked"] = true
	}
	if req.Active {
		args["active"] = true
	}
	if req.Limit > 0 {
		args["limit"] = req.Limit
	}
	res, err := c.call(ctx, "roady_status", args)
	if err != nil {
		return nil, err
	}
	return unmarshalText[StatusResult](res)
}

// TransitionRequest provides typed parameters for task transitions.
type TransitionRequest struct {
	TaskID   string
	Event    string
	Evidence string
	Actor    string
}

// TransitionTaskTyped transitions a task using a typed request.
func (c *Client) TransitionTaskTyped(ctx context.Context, req TransitionRequest) (string, error) {
	args := map[string]any{"task_id": req.TaskID, "event": req.Event}
	if req.Evidence != "" {
		args["evidence"] = req.Evidence
	}
	if req.Actor != "" {
		args["actor"] = req.Actor
	}
	res, err := c.call(ctx, "roady_task_transition", args)
	if err != nil {
		return "", err
	}
	return textResult(res)
}

// DebtReportTyped returns a typed debt report.
func (c *Client) DebtReportTyped(ctx context.Context) (*DebtReport, error) {
	res, err := c.call(ctx, "roady_debt_report", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[DebtReport](res)
}

// DebtSummaryTyped returns a typed debt summary.
func (c *Client) DebtSummaryTyped(ctx context.Context) (*DebtSummary, error) {
	res, err := c.call(ctx, "roady_debt_summary", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[DebtSummary](res)
}

// DebtTrendTyped returns a typed debt trend.
func (c *Client) DebtTrendTyped(ctx context.Context, days int) (*DebtTrend, error) {
	args := map[string]any{}
	if days > 0 {
		args["days"] = days
	}
	res, err := c.call(ctx, "roady_debt_trend", args)
	if err != nil {
		return nil, err
	}
	return unmarshalText[DebtTrend](res)
}

// DepsGraphTyped returns a typed dependency graph result.
func (c *Client) DepsGraphTyped(ctx context.Context, checkCycles bool) (*DepsGraphResult, error) {
	args := map[string]any{}
	if checkCycles {
		args["check_cycles"] = true
	}
	res, err := c.call(ctx, "roady_deps_graph", args)
	if err != nil {
		return nil, err
	}
	return unmarshalText[DepsGraphResult](res)
}

// OrgStatusTyped returns typed org metrics.
func (c *Client) OrgStatusTyped(ctx context.Context) (*OrgMetrics, error) {
	res, err := c.call(ctx, "roady_org_status", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[OrgMetrics](res)
}

// OrgPolicyTyped returns a typed org policy.
func (c *Client) OrgPolicyTyped(ctx context.Context) (*OrgPolicy, error) {
	res, err := c.call(ctx, "roady_org_policy", nil)
	if err != nil {
		return nil, err
	}
	return unmarshalText[OrgPolicy](res)
}

// CheckPolicyTyped returns typed policy violations.
func (c *Client) CheckPolicyTyped(ctx context.Context) ([]PolicyViolation, error) {
	res, err := c.call(ctx, "roady_policy_check", nil)
	if err != nil {
		return nil, err
	}
	text, err := textResult(res)
	if err != nil {
		return nil, err
	}
	// The tool returns "No policy violations found." when clean
	if text == "No policy violations found." {
		return nil, nil
	}
	v, err := unmarshalText[[]PolicyViolation](res)
	if err != nil {
		return nil, err
	}
	return *v, nil
}
