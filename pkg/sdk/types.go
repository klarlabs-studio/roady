package sdk

import "time"

// Spec represents a product specification.
type Spec struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Features    []Feature    `json:"features"`
	Constraints []Constraint `json:"constraints"`
	Version     string       `json:"version"`
}

// Feature represents a product feature.
type Feature struct {
	ID           string        `json:"id"`
	Title        string        `json:"title"`
	Description  string        `json:"description"`
	Requirements []Requirement `json:"requirements"`
}

// Requirement represents a feature requirement.
type Requirement struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Estimate    string   `json:"estimate"`
	DependsOn   []string `json:"depends_on"`
}

// Constraint represents a project constraint.
type Constraint struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Plan represents an execution plan.
type Plan struct {
	ID             string    `json:"id"`
	SpecID         string    `json:"spec_id"`
	Tasks          []Task    `json:"tasks"`
	ApprovalStatus string    `json:"approval_status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Task represents a plan task.
type Task struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	Estimate    string   `json:"estimate"`
	DependsOn   []string `json:"depends_on"`
	FeatureID   string   `json:"feature_id"`
}

// DriftReport represents a drift detection report.
type DriftReport struct {
	ID        string       `json:"id"`
	Issues    []DriftIssue `json:"issues"`
	CreatedAt time.Time    `json:"created_at"`
}

// DriftIssue represents a single drift issue.
type DriftIssue struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	ComponentID string `json:"component_id"`
	Message     string `json:"message"`
	Path        string `json:"path"`
	Hint        string `json:"hint"`
}

// Snapshot represents a project snapshot.
type Snapshot struct {
	Progress      float64  `json:"progress"`
	UnlockedTasks []string `json:"unlocked_tasks"`
	BlockedTasks  []string `json:"blocked_tasks"`
	InProgress    []string `json:"in_progress"`
	Completed     []string `json:"completed"`
	Verified      []string `json:"verified"`
	TotalTasks    int      `json:"total_tasks"`
	SnapshotTime  string   `json:"snapshot_time"`
}

// StatusResult represents project status output.
type StatusResult struct {
	TotalTasks    int            `json:"total_tasks"`
	FilteredCount int            `json:"filtered_count"`
	Counts        map[string]int `json:"counts"`
	Tasks         []StatusTask   `json:"tasks"`
}

// StatusTask is a task entry in status output.
type StatusTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Unlocked bool   `json:"unlocked,omitempty"`
}

// SchemaInfo describes the MCP schema version and deprecation info.
type SchemaInfo struct {
	SchemaVersion string            `json:"schema_version"`
	ServerVersion string            `json:"server_version"`
	Deprecated    []DeprecatedField `json:"deprecated"`
	Changelog     string            `json:"changelog"`
}

// DeprecatedField records a field or tool that has been deprecated.
type DeprecatedField struct {
	Tool      string `json:"tool"`
	Field     string `json:"field"`
	Since     string `json:"since"`
	RemovedIn string `json:"removed_in"`
	Migration string `json:"migration"`
}
