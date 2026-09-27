package cli

import (
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/spf13/cobra"
)

var (
	taskListStatus string
	taskListLimit  int
)

// taskListCmd is the command agents reach for first. Before it existed,
// `roady task list` printed the task help and the agent guessed again.
var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List tasks: all, or by --status (ready, pending, in_progress, blocked, done, verified, unassigned)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		status := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(taskListStatus)), "-", "_")
		var tasks []project.TaskSummary
		switch status {
		case "", "all":
			status = "all"
			tasks, err = services.Plan.GetTaskSummaries(ctx)
		case "ready":
			tasks, err = services.Plan.GetReadyTasks(ctx)
		case "unassigned":
			tasks, err = services.Plan.GetTasksByOwner(ctx, "")
		case string(planning.StatusPending), string(planning.StatusInProgress), string(planning.StatusBlocked),
			string(planning.StatusDone), string(planning.StatusVerified):
			var all []project.TaskSummary
			all, err = services.Plan.GetTaskSummaries(ctx)
			for _, t := range all {
				if string(t.Status) == status {
					tasks = append(tasks, t)
				}
			}
		default:
			return fmt.Errorf("unknown --status %q: use all, ready, pending, in_progress, blocked, done, verified or unassigned", taskListStatus)
		}
		if err != nil {
			return MapError(fmt.Errorf("list tasks: %w", err))
		}
		total := len(tasks)
		if taskListLimit > 0 && total > taskListLimit {
			tasks = tasks[:taskListLimit]
		}
		title := fmt.Sprintf("Tasks (%s)", status)
		if taskQueryJSON {
			return outputTaskSummaries(title, tasks, true)
		}
		out := cmd.OutOrStdout()
		_, _ = fmt.Fprintf(out, "%s: %d\n", title, total)
		for _, t := range tasks {
			_, _ = fmt.Fprintf(out, "  %-12s %-32s %s\n", "["+string(t.Status)+"]", t.ID, t.Title)
		}
		if total == 0 {
			_, _ = fmt.Fprintln(out, "  (none)")
		}
		if len(tasks) < total {
			_, _ = fmt.Fprintf(out, "  … %d more (--limit 0 for all)\n", total-len(tasks))
		}
		return nil
	},
}

func init() {
	taskListCmd.Flags().StringVar(&taskListStatus, "status", "all", "all, ready, pending, in_progress, blocked, done, verified or unassigned")
	taskListCmd.Flags().IntVar(&taskListLimit, "limit", 50, "Show at most this many (0 for all)")
	taskListCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")
	taskCmd.AddCommand(taskListCmd)
}
