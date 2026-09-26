package mcp

import (
	"context"
	"encoding/json"

	mcp "go.klarlabs.de/mcp"
	"go.klarlabs.de/mcp/protocol"
)

// listFilter trims tools/list to the tools listed allows, leaving every
// other request — including tools/call for an unlisted tool — untouched.
// With a nil listed it passes everything through.
func listFilter(listed func(name string) bool) mcp.Middleware {
	return func(next mcp.MiddlewareHandlerFunc) mcp.MiddlewareHandlerFunc {
		return func(ctx context.Context, req *protocol.Request) (*protocol.Response, error) {
			resp, err := next(ctx, req)
			if listed == nil || err != nil || resp == nil || resp.Error != nil || req.Method != protocol.MethodToolsList {
				return resp, err
			}
			raw, merr := json.Marshal(resp.Result)
			if merr != nil {
				return resp, err
			}
			var result map[string]any
			if json.Unmarshal(raw, &result) != nil {
				return resp, err
			}
			tools, _ := result["tools"].([]any)
			kept := make([]any, 0, len(tools))
			for _, t := range tools {
				if m, ok := t.(map[string]any); ok {
					if name, _ := m["name"].(string); listed(name) {
						kept = append(kept, t)
					}
				}
			}
			result["tools"] = kept
			resp.Result = result
			return resp, err
		}
	}
}
