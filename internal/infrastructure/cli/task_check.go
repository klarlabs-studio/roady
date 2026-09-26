package cli

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/spf13/cobra"
)

var (
	taskCheckConfirm bool
	taskCheckTimeout time.Duration
)

var taskCheckCmd = &cobra.Command{
	Use:   "check <task-id>",
	Short: "Run a task's acceptance check and record the result as evidence",
	Long: `Run the acceptance check defined for a task (its requirement's "check" in
spec.yaml) and record the result — pass or fail, exit code, commit, and the tail
of the output — on the task and in the audit log.

A "run" check executes its command in the project root; exit status 0 passes.
A "manual" check is a person's confirmation: pass --confirm to record it.

'roady task verify' runs the check itself and refuses when it fails, so this
command is for checking progress before then.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		workspace := wiring.NewWorkspace(root)
		service := application.NewTaskService(workspace.Repo, workspace.Audit, application.NewPolicyService(workspace.Repo))
		actor := resolveCurrentOwner(gitConfigUserName)
		if actor == "" {
			actor = "unknown-human"
		}

		result, err := service.RunCheck(cmd.Context(), args[0], actor, application.CheckOptions{
			ConfirmManual: taskCheckConfirm,
			Timeout:       taskCheckTimeout,
		})
		var manual *application.ManualCheckError
		if errors.As(err, &manual) {
			return fmt.Errorf("%s", manual.Error())
		}
		if err != nil {
			return MapError(fmt.Errorf("failed to run check: %w", err))
		}
		printCheckResult(args[0], result)
		if !result.Passed {
			os.Exit(1)
		}
		return nil
	},
}

func printCheckResult(taskID string, r planning.CheckResult) {
	verdict := "PASSED"
	if !r.Passed {
		verdict = fmt.Sprintf("FAILED (exit %d)", r.ExitCode)
	}
	fmt.Printf("Check for %s %s: %s\n", taskID, verdict, r.Command)
	if r.Commit != "" {
		at := r.Commit
		if len(at) > 12 {
			at = at[:12]
		}
		if r.Dirty {
			at += " with uncommitted changes"
		}
		fmt.Printf("  at %s", at)
		if r.Duration != "" {
			fmt.Printf(", took %s", r.Duration)
		}
		fmt.Println()
	}
	if !r.Passed && r.Output != "" {
		fmt.Printf("\n%s\n", r.Output)
	}
}

func init() {
	taskCheckCmd.Flags().BoolVar(&taskCheckConfirm, "confirm", false, "Confirm a manual check as the person performing it")
	taskCheckCmd.Flags().DurationVar(&taskCheckTimeout, "timeout", application.DefaultCheckTimeout, "Maximum time a run check may take")
	taskCmd.AddCommand(taskCheckCmd)
}
