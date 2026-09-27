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
	var override string
	var blockReason string
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

			// Same identity resolution as `roady task mine`, so ownership,
			// per-owner WIP limits, and team-role checks all agree on who
			// you are.
			actor := resolveCurrentOwner(gitConfigUserName)
			if actor == "" {
				actor = "unknown-human"
			}

			if event == "start" {
				err := service.StartTask(cmd.Context(), taskID, actor)
				if err != nil {
					return MapError(fmt.Errorf("failed to start task: %w", err))
				}
			} else if event == "block" {
				if err := service.BlockWithReason(taskID, blockReason, evidence, actor); err != nil {
					return MapError(fmt.Errorf("failed to block task: %w", err))
				}
			} else if event == "verify" && override != "" {
				// A person verifying without the evidence the policy asks
				// for; recorded as an override with its reason.
				if err := service.VerifyWithOverride(cmd.Context(), taskID, actor, override); err != nil {
					return MapError(fmt.Errorf("failed to verify task: %w", err))
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
	if event == "block" {
		cmd.Flags().StringVar(&blockReason, "reason", "", "spec-conflict or cannot-complete when the task cannot be done as specified: a person decides, and drift reports it until then. Say what is wrong with -e")
		cmd.Long = `Block a task. An ordinary block means waiting on something.

When a task cannot be done as specified, block it with a reason instead of
forcing it done — bending a check or a test to pass is worse than saying so:

  roady task block task-x --reason spec-conflict -e "R2 requires sync writes, R5 forbids them"
  roady task block task-x --reason cannot-complete -e "needs production credentials"

A person resolves it: changes the requirement, re-scopes the task, or
unblocks it. Until then it shows in status and drift.`
	}
	if event == "verify" {
		cmd.Flags().StringVar(&override, "override", "", "Verify without the evidence verify_requires_evidence asks for, recording this reason. Does not override a failing check.")
	}
	return cmd
}

var taskQueryJSON bool

var taskHistoryJSON bool

var taskHistoryCmd = &cobra.Command{
	Use:   "history <task-id>",
	Short: "What happened to a task: created, edited, split, moved, started, checked, blocked",
	Long: `Read a task's history back from the event log. Every edit records the
fields it changed, every transition and check its own event, so the history
is complete even though plan.json only holds the latest shape.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		ws := wiring.NewWorkspace(root)
		svc := application.NewTaskService(ws.Repo, ws.Audit, application.NewPolicyService(ws.Repo))
		entries, err := svc.TaskHistory(args[0])
		if err != nil {
			return MapError(err)
		}
		if taskHistoryJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(entries)
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), application.RenderHistory(args[0], entries))
		return err
	},
}

var taskRenewCmd = &cobra.Command{
	Use:   "renew <task-id>",
	Short: "Keep your claim on an in-progress task",
	Long: `Starting a task claims it with a lease (policy claim_lease, default 2h).
The lease is renewed whenever you run ` + "`roady next`" + ` (the session-start hook does,
also after compaction) and each time the agent writes a file (the write-guard
hook); this renews it explicitly. A claim nobody renews runs out and the task goes back to pending,
so an agent that crashed does not hold it forever.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		ws := wiring.NewWorkspace(root)
		svc := application.NewTaskService(ws.Repo, ws.Audit, application.NewPolicyService(ws.Repo))
		actor := resolveCurrentOwner(gitConfigUserName)
		lease, err := svc.RenewClaim(args[0], actor)
		if err != nil {
			return MapError(err)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Claim on %s renewed until %s.\n", args[0], lease.ExpiresAt.Local().Format("2006-01-02 15:04"))
		return nil
	},
}

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

func init() {
	taskCmd.AddCommand(createTaskCommand("start", "Start a task (claims it; see `roady task renew`)", "start"))
	taskCmd.AddCommand(taskRenewCmd)
	taskHistoryCmd.Flags().BoolVar(&taskHistoryJSON, "json", false, "Print the history as JSON")
	taskCmd.AddCommand(taskHistoryCmd)
	taskCmd.AddCommand(createTaskCommand("block", "Block a task", "block"))
	taskCmd.AddCommand(createTaskCommand("unblock", "Unblock a task", "unblock"))
	taskCmd.AddCommand(createTaskCommand("complete", "Complete a task", "complete"))
	taskCmd.AddCommand(createTaskCommand("stop", "Stop working on a task", "stop"))
	taskCmd.AddCommand(createTaskCommand("reopen", "Reopen a completed task", "reopen"))
	taskCmd.AddCommand(createTaskCommand("verify", "Mark a completed task as verified; runs its acceptance check first and refuses if it fails", "verify"))

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

	RootCmd.AddCommand(taskCmd)
}
