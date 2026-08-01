package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/spf13/cobra"
)

// Flag variables for status command
var (
	statusFilter   string
	priorityFilter string
	readyOnly      bool
	blockedOnly    bool
	activeOnly     bool
	statusLimit    int
	statusJSON     bool
	snapshotMode   bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show a high-level summary of the project state",
	Long: `Show a high-level summary of the project state.

Use flags to filter tasks:
  --status, -s    Filter by status (pending,blocked,in_progress,done,verified)
  --priority, -p  Filter by priority (high,medium,low)
  --ready         Show only tasks ready to start (unlocked + pending)
  --blocked       Show only blocked tasks
  --active        Show only in-progress tasks
  --limit, -n     Limit number of tasks shown
  --json          Output in JSON format

Examples:
  roady status --status pending
  roady status -s in_progress,blocked -p high
  roady status --ready --limit 5
  roady status --json`,
	RunE: runStatusCmd,
}

// statusJSONOutput represents the JSON output format for status
type statusJSONOutput struct {
	Project  string           `json:"project"`
	Version  string           `json:"version"`
	Features int              `json:"features"`
	Plan     *planJSONOutput  `json:"plan,omitempty"`
	Drift    *driftJSONOutput `json:"drift,omitempty"`
}

type planJSONOutput struct {
	Status   string           `json:"status"`
	Tasks    int              `json:"total_tasks"`
	Progress float64          `json:"progress"`
	Counts   map[string]int   `json:"counts"`
	Items    []taskJSONOutput `json:"tasks,omitempty"`
}

type taskJSONOutput struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	Origin   string `json:"origin,omitempty"`
	Unlocked bool   `json:"unlocked,omitempty"`
}

type driftJSONOutput struct {
	HasDrift bool `json:"has_drift"`
	Count    int  `json:"count"`
}

func runStatusCmd(cmd *cobra.Command, args []string) error {
	if hint, ok := emptyStateHintForCurrentDir(); ok {
		if statusJSON {
			return writeEmptyStateJSON(cmd, hint)
		}
		writeEmptyStateText(cmd, hint)
		return nil
	}

	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}

	repo := services.Workspace.Repo

	productSpec, err := repo.LoadSpec()
	if err != nil {
		return err
	}

	plan, err := repo.LoadPlan()
	if err != nil {
		return err
	}

	state, err := repo.LoadState()
	if err != nil {
		return err
	}

	// Check drift
	var driftCount int
	report, driftErr := services.Drift.DetectDrift(cmd.Context())
	if driftErr == nil && len(report.Issues) > 0 {
		driftCount = len(report.Issues)
	}

	// Snapshot mode — uses coordinator for consistent view
	if snapshotMode {
		return outputSnapshot(services, statusJSON)
	}

	// JSON output mode
	if statusJSON {
		return outputStatusJSON(productSpec, plan, state, driftCount)
	}

	// Text output mode
	return outputStatusText(cmd, productSpec, plan, state, driftCount)
}

func outputStatusJSON(productSpec *spec.ProductSpec, plan *planning.Plan, state *planning.ExecutionState, driftCount int) error {
	output := statusJSONOutput{
		Project:  productSpec.Title,
		Version:  productSpec.Version,
		Features: len(productSpec.Features),
	}

	if plan != nil {
		counts := countTasksByStatus(plan, state)
		totalDone := counts[planning.StatusDone] + counts[planning.StatusVerified]
		progress := 0.0
		if len(plan.Tasks) > 0 {
			progress = float64(totalDone) / float64(len(plan.Tasks)) * 100
		}

		planOutput := &planJSONOutput{
			Status:   string(plan.ApprovalStatus),
			Tasks:    len(plan.Tasks),
			Progress: progress,
			Counts: map[string]int{
				"pending":     counts[planning.StatusPending],
				"blocked":     counts[planning.StatusBlocked],
				"in_progress": counts[planning.StatusInProgress],
				"done":        counts[planning.StatusDone],
				"verified":    counts[planning.StatusVerified],
			},
		}

		// Filter and include tasks
		filteredTasks := filterTasks(plan.Tasks, state)
		for _, t := range filteredTasks {
			status := getTaskStatus(t.ID, state)
			planOutput.Items = append(planOutput.Items, taskJSONOutput{
				ID:       t.ID,
				Title:    t.Title,
				Status:   string(status),
				Priority: string(t.Priority),
				Origin:   string(t.NormalisedOrigin()),
				Unlocked: isTaskUnlocked(t, state, plan),
			})
		}

		output.Plan = planOutput
	}

	if driftCount > 0 {
		output.Drift = &driftJSONOutput{
			HasDrift: true,
			Count:    driftCount,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

func outputStatusText(cmd *cobra.Command, productSpec *spec.ProductSpec, plan *planning.Plan, state *planning.ExecutionState, driftCount int) error {
	if productSpec.Version != "" {
		fmt.Printf("Project: %s (v%s)\n", productSpec.Title, productSpec.Version)
	} else {
		fmt.Printf("Project: %s\n", productSpec.Title)
	}
	fmt.Printf("Spec features: %d\n", len(productSpec.Features))

	if plan == nil {
		fmt.Println("Plan status: No plan generated yet. Run 'roady plan generate'.")
		return nil
	}
	fmt.Printf("Plan status: %s\n", plan.ApprovalStatus)

	counts := countTasksByStatus(plan, state)

	fmt.Printf("Plan tasks: %d\n", len(plan.Tasks))
	fmt.Printf("- Pending:     %d\n", counts[planning.StatusPending])
	fmt.Printf("- Blocked:     %d\n", counts[planning.StatusBlocked])
	fmt.Printf("- In Progress: %d\n", counts[planning.StatusInProgress])
	fmt.Printf("- Done:        %d (awaiting verification)\n", counts[planning.StatusDone])
	fmt.Printf("- Verified:    %d\n", counts[planning.StatusVerified])

	if len(plan.Tasks) > 0 {
		totalDone := counts[planning.StatusDone] + counts[planning.StatusVerified]
		progress := float64(totalDone) / float64(len(plan.Tasks)) * 100
		fmt.Printf("\nOverall Progress: %.1f%% (%d/%d tasks finished)\n", progress, totalDone, len(plan.Tasks))
	}

	// Filter tasks
	filteredTasks := filterTasks(plan.Tasks, state)

	// Show filter info if any filter is active
	if hasActiveFilters() {
		fmt.Printf("\nFiltered Tasks (%d matching)\n", len(filteredTasks))
	} else {
		fmt.Println("\nTask Overview")
	}
	fmt.Println("----------------")

	// Sort filtered tasks
	sortedTasks := sortTasks(filteredTasks, state)

	for _, t := range sortedTasks {
		status := getTaskStatus(t.ID, state)
		prefix := getStatusPrefix(status)
		origin := t.NormalisedOrigin()
		marker := ""
		if origin == planning.OriginAI {
			// Flag AI-proposed tasks so reviewers can give them extra
			// scrutiny in the task overview.
			marker = " [AI]"
		}
		// Append a "from doc:line" annotation when the task carries
		// provenance from a source document (e.g. spec analyze output).
		if !t.Source.IsZero() {
			if t.Source.Line > 0 {
				marker += fmt.Sprintf(" from %s:%d", t.Source.Doc, t.Source.Line)
			} else {
				marker += fmt.Sprintf(" from %s", t.Source.Doc)
			}
		}
		fmt.Printf("%s [%-11s] %-40s (Priority: %s)%s\n", prefix, status, t.Title, t.Priority, marker)
	}

	if len(filteredTasks) == 0 && hasActiveFilters() {
		fmt.Println("  No tasks match the current filters.")
	}

	// Drift warning
	if driftCount > 0 {
		fmt.Printf("\nDRIFT DETECTED: %d issues found. Run 'roady drift detect' for details.\n", driftCount)
	}

	fmt.Printf("\nAudit Trail: .roady/events.jsonl\n")
	return nil
}

// filterTasks applies all active filters to the task list
func filterTasks(tasks []planning.Task, state *planning.ExecutionState) []planning.Task {
	var filtered []planning.Task

	// Parse status filter
	var statusFilters []planning.TaskStatus
	if statusFilter != "" {
		for _, s := range strings.Split(statusFilter, ",") {
			if parsed, err := planning.ParseTaskStatus(strings.TrimSpace(s)); err == nil {
				statusFilters = append(statusFilters, parsed)
			}
		}
	}

	// Parse priority filter
	var priorityFilters []planning.TaskPriority
	if priorityFilter != "" {
		for _, p := range strings.Split(priorityFilter, ",") {
			if parsed, err := planning.ParseTaskPriority(strings.TrimSpace(p)); err == nil {
				priorityFilters = append(priorityFilters, parsed)
			}
		}
	}

	for _, t := range tasks {
		status := getTaskStatus(t.ID, state)

		// Apply shortcut flags
		if readyOnly {
			// Ready = unlocked AND pending
			if status != planning.StatusPending || !isTaskUnlockedByDeps(t, state) {
				continue
			}
		}
		if blockedOnly && status != planning.StatusBlocked {
			continue
		}
		if activeOnly && status != planning.StatusInProgress {
			continue
		}

		// Apply status filter
		if len(statusFilters) > 0 && !containsStatus(statusFilters, status) {
			continue
		}

		// Apply priority filter
		if len(priorityFilters) > 0 && !containsPriority(priorityFilters, t.Priority) {
			continue
		}

		filtered = append(filtered, t)
	}

	// Apply limit
	if statusLimit > 0 && len(filtered) > statusLimit {
		filtered = filtered[:statusLimit]
	}

	return filtered
}

// sortTasks sorts tasks by status rank then priority
func sortTasks(tasks []planning.Task, state *planning.ExecutionState) []planning.Task {
	statusRank := map[planning.TaskStatus]int{
		planning.StatusPending:    0,
		planning.StatusBlocked:    1,
		planning.StatusInProgress: 2,
		planning.StatusDone:       3,
		planning.StatusVerified:   4,
	}

	sortedTasks := make([]planning.Task, len(tasks))
	copy(sortedTasks, tasks)

	sort.Slice(sortedTasks, func(i, j int) bool {
		sI := getTaskStatus(sortedTasks[i].ID, state)
		sJ := getTaskStatus(sortedTasks[j].ID, state)
		if sI != sJ {
			return statusRank[sI] < statusRank[sJ]
		}
		return sortedTasks[i].Priority > sortedTasks[j].Priority
	})

	return sortedTasks
}

// countTasksByStatus counts tasks by their status
func countTasksByStatus(plan *planning.Plan, state *planning.ExecutionState) map[planning.TaskStatus]int {
	counts := make(map[planning.TaskStatus]int)
	for _, t := range plan.Tasks {
		status := getTaskStatus(t.ID, state)
		counts[status]++
	}
	return counts
}

// getTaskStatus returns the current status of a task
func getTaskStatus(taskID string, state *planning.ExecutionState) planning.TaskStatus {
	if state != nil {
		if res, ok := state.TaskStates[taskID]; ok {
			return res.Status
		}
	}
	return planning.StatusPending
}

// getStatusPrefix returns the display prefix for a status
func getStatusPrefix(status planning.TaskStatus) string {
	switch status {
	case planning.StatusVerified:
		return "[V]"
	case planning.StatusDone:
		return "[D]"
	case planning.StatusInProgress:
		return "[W]"
	case planning.StatusBlocked:
		return "[B]"
	default:
		return "[ ]"
	}
}

// isTaskUnlocked checks if a task is unlocked (for JSON output)
func isTaskUnlocked(task planning.Task, state *planning.ExecutionState, plan *planning.Plan) bool {
	status := getTaskStatus(task.ID, state)
	return status == planning.StatusPending && isTaskUnlockedByDeps(task, state)
}

// isTaskUnlockedByDeps checks if all dependencies are complete
func isTaskUnlockedByDeps(task planning.Task, state *planning.ExecutionState) bool {
	for _, depID := range task.DependsOn {
		depStatus := getTaskStatus(depID, state)
		if !depStatus.IsComplete() {
			return false
		}
	}
	return true
}

// containsStatus checks if a status is in the list
func containsStatus(statuses []planning.TaskStatus, s planning.TaskStatus) bool {
	for _, status := range statuses {
		if status == s {
			return true
		}
	}
	return false
}

// containsPriority checks if a priority is in the list
func containsPriority(priorities []planning.TaskPriority, p planning.TaskPriority) bool {
	for _, priority := range priorities {
		if priority == p {
			return true
		}
	}
	return false
}

// hasActiveFilters returns true if any filter is active
func hasActiveFilters() bool {
	return statusFilter != "" || priorityFilter != "" || readyOnly || blockedOnly || activeOnly
}

// Status subcommands - consolidated from top-level commands
var statusForecastCmd = &cobra.Command{
	Use:   "forecast",
	Short: "Predict project completion based on current velocity",
	Long: `Forecast provides project completion predictions with optional detailed analysis.

Flags:
  --detailed   Show confidence intervals and all velocity windows
  --burndown   Show burndown chart data
  --trend      Show velocity trend analysis
  --json       Output in JSON format`,
	RunE: RunForecast,
}

var statusUsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show project usage and AI token statistics",
	RunE:  RunUsage,
}

var statusTimelineCmd = &cobra.Command{
	Use:   "timeline",
	Short: "Show a chronological view of project activity",
	RunE:  RunTimeline,
}

func outputSnapshot(services *wiring.AppServices, jsonOut bool) error {
	ctx := context.Background()
	snapshot, err := services.Plan.GetProjectSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}

	if jsonOut {
		type snapshotJSON struct {
			Progress      float64  `json:"progress"`
			TotalTasks    int      `json:"total_tasks"`
			UnlockedTasks []string `json:"unlocked_tasks"`
			BlockedTasks  []string `json:"blocked_tasks"`
			InProgress    []string `json:"in_progress"`
			Completed     []string `json:"completed"`
			Verified      []string `json:"verified"`
		}

		totalTasks := 0
		if snapshot.Plan != nil {
			totalTasks = len(snapshot.Plan.Tasks)
		}

		out := snapshotJSON{
			Progress:      snapshot.Progress,
			TotalTasks:    totalTasks,
			UnlockedTasks: orEmptySlice(snapshot.UnlockedTasks),
			BlockedTasks:  orEmptySlice(snapshot.BlockedTasks),
			InProgress:    orEmptySlice(snapshot.InProgress),
			Completed:     orEmptySlice(snapshot.Completed),
			Verified:      orEmptySlice(snapshot.Verified),
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	totalTasks := 0
	if snapshot.Plan != nil {
		totalTasks = len(snapshot.Plan.Tasks)
	}

	fmt.Printf("Project Snapshot\n")
	fmt.Printf("================\n")
	fmt.Printf("Progress:    %.1f%%\n", snapshot.Progress)
	fmt.Printf("Total Tasks: %d\n", totalTasks)
	fmt.Printf("Ready:       %d\n", len(snapshot.UnlockedTasks))
	fmt.Printf("Blocked:     %d\n", len(snapshot.BlockedTasks))
	fmt.Printf("In Progress: %d\n", len(snapshot.InProgress))
	fmt.Printf("Completed:   %d\n", len(snapshot.Completed))
	fmt.Printf("Verified:    %d\n", len(snapshot.Verified))

	return nil
}

func orEmptySlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func init() {
	statusCmd.Flags().StringVarP(&statusFilter, "status", "s", "",
		"Filter by status (pending,blocked,in_progress,done,verified)")
	statusCmd.Flags().StringVarP(&priorityFilter, "priority", "p", "",
		"Filter by priority (high,medium,low)")
	statusCmd.Flags().BoolVar(&readyOnly, "ready", false,
		"Show only tasks ready to start (unlocked + pending)")
	statusCmd.Flags().BoolVar(&blockedOnly, "blocked", false,
		"Show only blocked tasks")
	statusCmd.Flags().BoolVar(&activeOnly, "active", false,
		"Show only in-progress tasks")
	statusCmd.Flags().IntVarP(&statusLimit, "limit", "n", 0,
		"Limit number of tasks shown")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false,
		"Output in JSON format")
	statusCmd.Flags().BoolVar(&snapshotMode, "snapshot", false,
		"Show coordinator-based project snapshot with progress and categorized task counts")

	// Add subcommands for consolidated views
	statusForecastCmd.Flags().BoolVar(&forecastDetailed, "detailed", false, "Show detailed forecast with confidence intervals")
	statusForecastCmd.Flags().BoolVar(&forecastBurndown, "burndown", false, "Show burndown chart data")
	statusForecastCmd.Flags().BoolVar(&forecastTrend, "trend", false, "Show velocity trend analysis")
	statusForecastCmd.Flags().BoolVar(&forecastJSON, "json", false, "Output in JSON format")

	statusCmd.AddCommand(statusForecastCmd)
	statusCmd.AddCommand(statusUsageCmd)
	statusCmd.AddCommand(statusTimelineCmd)

	RootCmd.AddCommand(statusCmd)
}
