package mcp

// This file collects the output types that roady tools advertise as their MCP
// outputSchema. Declaring them at package scope lets a tool registration pass a
// zero value to OutputSchema(...); the handler returns the same type and mcp-go
// emits it as structuredContent, so MCP clients get typed, validated data
// instead of parsing the JSON text. The core read tools (roady_plan_get/state/
// spec) advertise their domain types directly; this file only needs to host the
// types that were previously declared inside a handler.

// burndownPt is one point on the forecast burndown curve: the actual and
// projected remaining-task counts on a given date. Hoisted from handleForecast
// so forecastResp can be named as an outputSchema.
type burndownPt struct {
	Date      string `json:"date"`
	Actual    int    `json:"actual"`
	Projected int    `json:"projected"`
}

// windowPt is a velocity measurement over a trailing window of days. Hoisted
// from handleForecast alongside forecastResp.
type windowPt struct {
	Days     int     `json:"days"`
	Velocity float64 `json:"velocity"`
	Count    int     `json:"count"`
}

// forecastResp is the structured result of roady_forecast: completion
// projection with velocity, a confidence interval, trend analysis, and the
// burndown/velocity series that drive the UI. Previously a handler-local type;
// hoisted here so it can be advertised as the tool's outputSchema.
type forecastResp struct {
	Remaining      int          `json:"remaining"`
	Completed      int          `json:"completed"`
	Total          int          `json:"total"`
	Velocity       float64      `json:"velocity"`
	EstimatedDays  float64      `json:"estimated_days"`
	CompletionRate float64      `json:"completion_rate"`
	Trend          string       `json:"trend"`
	TrendSlope     float64      `json:"trend_slope"`
	Confidence     float64      `json:"confidence"`
	CILow          float64      `json:"ci_low"`
	CIExpected     float64      `json:"ci_expected"`
	CIHigh         float64      `json:"ci_high"`
	Burndown       []burndownPt `json:"burndown"`
	Windows        []windowPt   `json:"windows"`
	DataPoints     int          `json:"data_points"`
}

// snapshotResp is the structured result of roady_snapshot_get: a consistent
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
