package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage individual tasks",
}

func createTaskCommand(use, short, event string) *cobra.Command {
	var evidence string
	var rateID string
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, cErr := getProjectRoot()
			if cErr != nil {
				return fmt.Errorf("resolve project path: %w", cErr)
			}
			workspace := wiring.NewWorkspace(cwd)
			repo := workspace.Repo
			audit := workspace.Audit
			policy := application.NewPolicyService(repo)
			service := application.NewTaskService(repo, audit, policy)
			taskID := args[0]

			actor := os.Getenv("USER")
			if actor == "" {
				actor = "unknown-human"
			}

			if event == "start" {
				err := service.StartTask(cmd.Context(), taskID, actor, rateID)
				if err != nil {
					return MapError(fmt.Errorf("failed to start task: %w", err))
				}
			} else {
				err := service.TransitionTask(taskID, event, actor, evidence)
				if err != nil {
					return MapError(fmt.Errorf("failed to transition task: %w", err))
				}
			}
			fmt.Printf("Task %s transition '%s' successful.\n", taskID, event)
			return nil
		},
	}
	cmd.Flags().StringVarP(&evidence, "evidence", "e", "", "Evidence for the task completion (e.g. commit hash, URL)")
	if event == "start" {
		cmd.Flags().StringVarP(&rateID, "rate", "r", "", "Rate ID to use for billing")
	}
	return cmd
}

var taskQueryJSON bool

var taskReadyCmd = &cobra.Command{
	Use:   "ready",
	Short: "List tasks ready to start (unlocked and pending)",
	RunE:  runTaskReady,
}

var taskBlockedCmd = &cobra.Command{
	Use:   "blocked",
	Short: "List currently blocked tasks",
	RunE:  runTaskBlocked,
}

var taskInProgressCmd = &cobra.Command{
	Use:   "in-progress",
	Short: "List currently in-progress tasks",
	RunE:  runTaskInProgress,
}

func runTaskReady(cmd *cobra.Command, args []string) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}
	tasks, err := services.Plan.GetReadyTasks(cmd.Context())
	if err != nil {
		return MapError(fmt.Errorf("get ready tasks: %w", err))
	}
	return outputTaskSummaries("Ready Tasks", tasks, taskQueryJSON)
}

func runTaskBlocked(cmd *cobra.Command, args []string) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}
	tasks, err := services.Plan.GetBlockedTasks(cmd.Context())
	if err != nil {
		return MapError(fmt.Errorf("get blocked tasks: %w", err))
	}
	return outputTaskSummaries("Blocked Tasks", tasks, taskQueryJSON)
}

func runTaskInProgress(cmd *cobra.Command, args []string) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}
	tasks, err := services.Plan.GetInProgressTasks(cmd.Context())
	if err != nil {
		return MapError(fmt.Errorf("get in-progress tasks: %w", err))
	}
	return outputTaskSummaries("In-Progress Tasks", tasks, taskQueryJSON)
}

func outputTaskSummaries(title string, tasks []project.TaskSummary, jsonOut bool) error {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tasks)
	}

	fmt.Printf("%s (%d)\n", title, len(tasks))
	fmt.Println(strings.Repeat("-", len(title)+10))
	for _, t := range tasks {
		fmt.Printf("  %-30s [%s] %s\n", t.ID, t.Priority, t.Title)
		if t.Owner != "" {
			fmt.Printf("    → assigned: %s\n", t.Owner)
		}
	}
	if len(tasks) == 0 {
		fmt.Println("  (none)")
	}
	return nil
}

var taskAssignCmd = &cobra.Command{
	Use:   "assign <task-id> <assignee>",
	Short: "Assign a task to a person or agent",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, cErr := getProjectRoot()
		if cErr != nil {
			return fmt.Errorf("resolve project path: %w", cErr)
		}
		workspace := wiring.NewWorkspace(cwd)
		repo := workspace.Repo
		audit := workspace.Audit
		policy := application.NewPolicyService(repo)
		service := application.NewTaskService(repo, audit, policy)

		err := service.AssignTask(cmd.Context(), args[0], args[1])
		if err != nil {
			return MapError(fmt.Errorf("failed to assign task: %w", err))
		}
		fmt.Printf("Task %s assigned to %s\n", args[0], args[1])
		return nil
	},
}

// resolveCurrentOwner determines who "me" is for owner-scoped task queries.
// Precedence: ROADY_USER, then git user.name, then USER. Returns "" when no
// identity is configured, which callers must treat as an error rather than as
// a query for unassigned tasks.
func resolveCurrentOwner(gitUserName func() string) string {
	if v := strings.TrimSpace(os.Getenv("ROADY_USER")); v != "" {
		return v
	}
	if v := strings.TrimSpace(gitUserName()); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("USER"))
}

func gitConfigUserName() string {
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

var taskAssignedCmd = &cobra.Command{
	Use:   "assigned <assignee>",
	Short: "List tasks assigned to a person or agent",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return listTasksForOwner(cmd, args[0], "Tasks assigned to "+args[0])
	},
}

var taskMineCmd = &cobra.Command{
	Use:   "mine",
	Short: "List tasks assigned to you (ROADY_USER, git user.name, or USER)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		owner := resolveCurrentOwner(gitConfigUserName)
		if owner == "" {
			return fmt.Errorf("cannot determine who you are: set ROADY_USER or git config user.name")
		}
		return listTasksForOwner(cmd, owner, "Your tasks ("+owner+")")
	},
}

var taskUnassignedCmd = &cobra.Command{
	Use:   "unassigned",
	Short: "List tasks with no assignee",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listTasksForOwner(cmd, "", "Unassigned Tasks")
	},
}

func listTasksForOwner(cmd *cobra.Command, owner, title string) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}
	tasks, err := services.Plan.GetTasksByOwner(cmd.Context(), owner)
	if err != nil {
		return MapError(fmt.Errorf("get tasks by owner: %w", err))
	}
	return outputTaskSummaries(title, tasks, taskQueryJSON)
}

var taskStartRate string

var taskLogCmd = &cobra.Command{
	Use:   "log <task-id> <minutes>",
	Short: "Log time manually to a task",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		billingSvc := services.Billing

		taskID := args[0]
		var minutes int
		_, err = fmt.Sscanf(args[1], "%d", &minutes)
		if err != nil {
			return fmt.Errorf("invalid minutes: %w", err)
		}

		err = billingSvc.LogTime(taskID, taskStartRate, minutes, "")
		if err != nil {
			return MapError(fmt.Errorf("failed to log time: %w", err))
		}
		fmt.Printf("Logged %d minutes to task %s\n", minutes, taskID)
		return nil
	},
}

func init() {
	taskCmd.AddCommand(taskAssignCmd)
	taskCmd.AddCommand(createTaskCommand("start", "Start a task", "start"))
	taskCmd.AddCommand(createTaskCommand("block", "Block a task", "block"))
	taskCmd.AddCommand(createTaskCommand("unblock", "Unblock a task", "unblock"))
	taskCmd.AddCommand(createTaskCommand("complete", "Complete a task", "complete"))
	taskCmd.AddCommand(createTaskCommand("stop", "Stop working on a task", "stop"))
	taskCmd.AddCommand(createTaskCommand("reopen", "Reopen a completed task", "reopen"))
	taskCmd.AddCommand(createTaskCommand("verify", "Mark a task as verified with evidence", "verify"))

	taskLogCmd.Flags().StringVar(&taskStartRate, "rate", "", "Rate ID to use for billing")

	taskReadyCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")
	taskBlockedCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")
	taskInProgressCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")
	taskAssignedCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")
	taskMineCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")
	taskUnassignedCmd.Flags().BoolVar(&taskQueryJSON, "json", false, "Output in JSON format")

	taskCmd.AddCommand(taskAssignedCmd)
	taskCmd.AddCommand(taskMineCmd)
	taskCmd.AddCommand(taskUnassignedCmd)

	taskCmd.AddCommand(taskReadyCmd)
	taskCmd.AddCommand(taskBlockedCmd)
	taskCmd.AddCommand(taskInProgressCmd)
	taskCmd.AddCommand(taskLogCmd)

	RootCmd.AddCommand(taskCmd)
}
