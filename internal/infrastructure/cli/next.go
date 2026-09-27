package cli

import (
	"encoding/json"
	"fmt"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var nextJSON bool
var nextOwner string

var nextCmd = &cobra.Command{
	Use:   "next",
	Short: "Brief on the task you are on, or should start next",
	Long: `Print a compact brief of your current task — or, when none is in progress,
the ready task to start next: why it exists (with its doc:line), what done means
and the last check result, what it depends on and unblocks, and the working
rules.

It is small enough to inject into an agent's context unconditionally: at
session start, after compaction, or every few steps. Agents rarely ask for
their plan on their own; showing it to them is what keeps them on it.

"You" is ROADY_USER, git user.name, or USER — the same identity as
'roady task mine'. Use --owner to brief someone else.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		ws := wiring.NewWorkspace(root)
		svc := application.NewTaskService(ws.Repo, ws.Audit, application.NewPolicyService(ws.Repo))
		owner := nextOwner
		if owner == "" {
			owner = resolveCurrentOwner(gitConfigUserName)
		}
		brief, err := svc.Brief(cmd.Context(), owner)
		if err != nil {
			return MapError(err)
		}
		if nextJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(brief)
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), brief.Render())
		return err
	},
}

func init() {
	nextCmd.Flags().BoolVar(&nextJSON, "json", false, "Print the brief as JSON")
	nextCmd.Flags().StringVar(&nextOwner, "owner", "", "Brief for this owner instead of you")
	RootCmd.AddCommand(nextCmd)
}
