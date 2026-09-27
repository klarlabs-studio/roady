package cli

import (
	"encoding/json"
	"fmt"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var statsJSON bool

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Is roady doing its job here? Three numbers from the local event log",
	Long: `Three adoption numbers, computed from .roady/events.jsonl; nothing leaves
the machine.

  Plans captured automatically     plans agents wrote that the plan-approved
                                   hook imported, of all plans imported
  Verified with a passing check    verified tasks whose verification ran a
                                   passing acceptance check (latest verify)
  Sessions resumed the right task  sessions that began with work in progress
                                   and touched that work first`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		ws := wiring.NewWorkspace(root)
		st, err := application.NewTaskService(ws.Repo, ws.Audit, application.NewPolicyService(ws.Repo)).Stats()
		if err != nil {
			return MapError(err)
		}
		if statsJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(st)
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), st.Render())
		return err
	},
}

func init() {
	statsCmd.Flags().BoolVar(&statsJSON, "json", false, "Print the numbers as JSON")
	RootCmd.AddCommand(statsCmd)
}
