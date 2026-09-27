package mcp

// snapshotResp is the result of roady_status with snapshot: overall progress
// plus the task ids in each lifecycle bucket.
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
