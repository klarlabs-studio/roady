package mcp

// This file collects the output types that roady tools advertise as their MCP
// outputSchema. Declaring them at package scope lets a tool registration pass a
// zero value to OutputSchema(...); the handler returns the same type and mcp-go
// emits it as structuredContent, so MCP clients get typed, validated data
// instead of parsing the JSON text. The core read tools (roady_get_plan/state/
// spec) advertise their domain types directly; this file only needs to host the
// types that were previously declared inside a handler.

// snapshotResp is the structured result of roady_get_snapshot: a consistent
// project overview — overall progress plus the task ids in each lifecycle
// bucket. Previously a handler-local type; hoisted here so it can be advertised
// as the tool's outputSchema.
type snapshotResp struct {
	Progress      float64  `json:"progress"`
	UnlockedTasks []string `json:"unlocked_tasks"`
	BlockedTasks  []string `json:"blocked_tasks"`
	InProgress    []string `json:"in_progress"`
	Completed     []string `json:"completed"`
	Verified      []string `json:"verified"`
	TotalTasks    int      `json:"total_tasks"`
	SnapshotTime  string   `json:"snapshot_time"`
}
