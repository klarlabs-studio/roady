package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/dispatch"
	"github.com/spf13/cobra"
)

var (
	dispatchAgent   string
	dispatchSession string
	dispatchJSON    bool
	dispatchDryRun  bool
)

var taskDispatchCmd = &cobra.Command{
	Use:   "dispatch <task-id>",
	Short: "Hand a ready task to a subagent, with its intent and completion contract",
	Long: `Prepare a ready task for handoff to a subagent.

An agent told to "work on task-42" has to reconstruct why the task exists,
what counts as finished, and how to report back. Roady already holds all
three, so a dispatch hands them over: the originating feature and requirement,
the doc:line citation that motivated the task, and the exact call that records
completion against the agent doing the work.

The task is claimed (moved to in_progress, owned by the agent) unless
--dry-run is given.

Examples:
  roady task dispatch task-42 --agent claude-code
  roady task dispatch task-42 --agent codex --session run-7 --json
  roady task dispatch task-42 --agent claude-code --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runTaskDispatch,
}

func runTaskDispatch(cmd *cobra.Command, args []string) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}

	agent := dispatchAgent
	if agent == "" {
		// Fall back to whoever this process already identifies as, rather
		// than refusing outright — but never to an empty attribution.
		agent = resolveCurrentOwner(gitConfigUserName)
	}

	brief, err := services.Dispatch.Dispatch(cmd.Context(), args[0], application.DispatchOptions{
		Agent:   agent,
		Session: dispatchSession,
		Start:   !dispatchDryRun,
	})
	if err != nil {
		return MapError(err)
	}

	if dispatchJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(brief)
	}

	printBrief(brief, dispatchDryRun)
	return nil
}

func printBrief(b *dispatch.Brief, dryRun bool) {
	fmt.Printf("# %s — %s\n\n", b.TaskID, b.Title)

	if b.Feature != "" {
		fmt.Printf("Feature: %s\n", b.Feature)
	}
	if b.Citation != "" {
		fmt.Printf("Traces to: %s\n", b.Citation)
	}
	if b.Priority != "" {
		fmt.Printf("Priority: %s", b.Priority)
		if b.Estimate != "" {
			fmt.Printf("  Estimate: %s", b.Estimate)
		}
		fmt.Println()
	}
	fmt.Println()

	if b.Description != "" {
		fmt.Printf("%s\n\n", b.Description)
	}
	if b.Requirement != "" && b.Requirement != b.Description {
		fmt.Printf("Requirement: %s\n\n", b.Requirement)
	}
	if b.Acceptance != "" {
		fmt.Printf("Done when: %s\n\n", b.Acceptance)
	}
	if len(b.DependsOn) > 0 {
		fmt.Printf("Depends on (all satisfied): %s\n\n", strings.Join(b.DependsOn, ", "))
	}

	fmt.Println("## Reporting completion")
	fmt.Println()
	fmt.Printf("%s\n\n", b.Completion.Instructions)
	fmt.Printf("  MCP: %s\n", b.Completion.Tool)
	fmt.Printf("  CLI: %s\n", b.Completion.CLI)

	// Gaps go to stderr so they reach the operator without contaminating a
	// brief that may be piped straight to an agent.
	for _, w := range b.Warnings() {
		fmt.Fprintf(os.Stderr, "\nwarning: %s\n", w)
	}

	if dryRun {
		fmt.Fprintln(os.Stderr, "\nDry run: the task was not claimed. Re-run without --dry-run to start it.")
	}
}

func init() {
	taskDispatchCmd.Flags().StringVar(&dispatchAgent, "agent", "", "Agent taking the task (defaults to this process's identity)")
	taskDispatchCmd.Flags().StringVar(&dispatchSession, "session", "", "Session ID to record the subagent's work under")
	taskDispatchCmd.Flags().BoolVar(&dispatchJSON, "json", false, "Emit the brief as JSON for an agent to consume")
	taskDispatchCmd.Flags().BoolVar(&dispatchDryRun, "dry-run", false, "Build the brief without claiming the task")

	taskCmd.AddCommand(taskDispatchCmd)
}
