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
