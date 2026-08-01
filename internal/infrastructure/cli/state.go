package cli

import (
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var stateRebuildDryRun bool

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Inspect and repair execution state",
}

var stateRebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild state.json by replaying the event log",
	Long: `Reconstruct .roady/state.json from .roady/events.jsonl.

state.json is a whole-file document, so two collaborators who both moved a
task conflict on it in git even when their work does not disagree. The event
log has no such problem: it is append-only and merges by union. After a
conflict, keep either side and run this — the replay produces the same result
either way.

  git checkout --ours .roady/state.json
  roady state rebuild

Use --dry-run first to see what would change.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		svc := application.NewStateRebuildService(services.Workspace.Repo)

		if stateRebuildDryRun {
			_, result, rErr := svc.Rebuild()
			if rErr != nil {
				return MapError(rErr)
			}
			reportRebuild(result, true)
			return nil
		}

		result, err := svc.Save()
		if err != nil {
			return MapError(err)
		}
		reportRebuild(result, false)
		return nil
	},
}

func reportRebuild(result *application.RebuildResult, dryRun bool) {
	verb := "Rebuilt"
	if dryRun {
		verb = "Would rebuild"
	}

	fmt.Printf("%s state from %d task events across %d tasks.\n",
		verb, result.EventsReplayed, result.TasksAffected)

	if len(result.Changed) == 0 {
		fmt.Println("State on disk already matches the event log.")
		return
	}

	fmt.Printf("\n%d task(s) differ from the current state.json:\n", len(result.Changed))
	for _, c := range result.Changed {
		from := string(c.From)
		if from == "" {
			from = "(absent)"
		}
		to := string(c.To)
		if to == "" {
			to = "(absent)"
		}
		fmt.Printf("  %-30s %s -> %s\n", c.TaskID, from, to)
	}
}

func init() {
	stateRebuildCmd.Flags().BoolVar(&stateRebuildDryRun, "dry-run", false, "Show what would change without writing")
	stateCmd.AddCommand(stateRebuildCmd)
	RootCmd.AddCommand(stateCmd)
}
