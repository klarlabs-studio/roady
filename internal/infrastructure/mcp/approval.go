package mcp

import (
	"context"
	"fmt"

	mcpserver "go.klarlabs.de/mcp/server"
)

// Decisions — approving or rejecting a plan, accepting drift, re-baselining
// the spec — and destructive operations are available over MCP, but an agent
// cannot take them on its own say-so. The server asks the user directly
// (MCP elicitation), in their client, and proceeds only on an explicit yes.
// A client that cannot ask gets a refusal naming the CLI command, so the
// decision still reaches a person. The confirmation is recorded in the audit
// log next to the operation it allowed.

// confirmFunc asks the user to confirm message. It returns nil only when the
// user explicitly approved.
type confirmFunc func(ctx context.Context, message string) error

// elicitConfirm asks through the client's elicitation support.
func elicitConfirm(ctx context.Context, message string) error {
	e := mcpserver.ElicitFromContext(ctx)
	res, err := e.Elicit(ctx, &mcpserver.ElicitRequest{
		Message: message,
		RequestedSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"approve": map[string]any{
					"type":        "boolean",
					"title":       "Approve",
					"description": "Your agent asked roady to do this. It happens only if you approve.",
				},
			},
			"required": []string{"approve"},
		},
	})
	if err != nil {
		return fmt.Errorf("this client cannot ask you to confirm (%v)", err)
	}
	if res.Action != "accept" {
		return fmt.Errorf("you %sd the request", res.Action)
	}
	if ok, _ := res.Content["approve"].(bool); !ok {
		return fmt.Errorf("you did not approve it")
	}
	return nil
}

// gated runs op after the user confirms; otherwise it returns a refusal the
// agent can relay. operation is the CLI verb ("plan approve").
func (s *Server) gated(ctx context.Context, sc scope, operation, question string, op func() (any, error)) (any, error) {
	confirm := s.confirm
	if confirm == nil {
		confirm = elicitConfirm
	}
	if err := confirm(ctx, "roady "+operation+": "+question); err != nil {
		return mcpErr(fmt.Sprintf("roady %s needs your approval and did not get it: %v. Nothing was changed. "+
			"Run `roady %s` yourself, or approve it when your client asks.", operation, err, operation)), nil
	}
	if svc, err := s.servicesForPath(sc.ProjectPath, sc.Project); err == nil && svc.Audit != nil {
		_ = svc.Audit.Log("approval.confirmed", "user", map[string]any{
			"operation": operation,
			"via":       "mcp-elicitation",
		})
	}
	return op()
}
