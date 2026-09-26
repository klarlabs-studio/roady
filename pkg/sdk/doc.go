// Package sdk provides a typed Go client for the Roady MCP server.
//
// The client wraps mcp-go/client.CallTool with one method per MCP tool,
// connection management, and automatic retry via fortify.
//
// Usage:
//
//	transport, _ := client.NewStdioTransport("roady", "mcp")
//	c := sdk.NewClient(transport)
//	defer c.Close()
//
//	info, _ := c.Connect(ctx)
//	spec, _ := c.GetSpec(ctx)
//	fmt.Println(spec.Title)
package sdk
