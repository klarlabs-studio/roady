package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	taskAcceptAllDone bool
	taskAcceptReason  string
)

var taskAcceptCmd = &cobra.Command{
	Use:   "accept [task-id...]",
	Short: "Accept finished tasks as done without verification (work finished before roady tracked it)",
	Long: `Accept finished tasks as done without verification.

Adopting roady in a project with history leaves every earlier completion
awaiting a verification nothing can give it: the work was finished before it
had an acceptance check. Accepting says a person takes it as done. The task
stays done, status stops counting it as awaiting verification, and it can
still be verified later; reopening it ends the acceptance.

It is not verification. Verified means a check passed or a person vouched for
that task; accepting records who accepted, why and when, in the state and the
audit trail, and nothing more.

  roady task accept --all-done --reason "finished before adopting roady"
  roady task accept task-a task-b --reason "shipped in v1.2, no checks then"

All or nothing: if any named task is not done, none is accepted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		actor := resolveCurrentOwner(gitConfigUserName)
		if actor == "" {
			actor = "unknown-human"
		}
		accepted, err := services.Task.AcceptFinished(cmd.Context(), args, taskAcceptAllDone, actor, taskAcceptReason)
		if err != nil {
			return MapError(fmt.Errorf("accept: %w", err))
		}
		noun := "tasks"
		if len(accepted) == 1 {
			noun = "task"
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Accepted %d %s as done without verification.\n", len(accepted), noun)
		return nil
	},
}

func init() {
	taskAcceptCmd.Flags().BoolVar(&taskAcceptAllDone, "all-done", false, "Accept every done task that is not verified or accepted")
	taskAcceptCmd.Flags().StringVar(&taskAcceptReason, "reason", "", "Why these are taken as done without verification (required; recorded in the audit trail)")
	taskCmd.AddCommand(taskAcceptCmd)
}
