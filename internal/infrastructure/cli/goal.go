package cli

import (
	"encoding/json"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

// Goals are the roadmap: outcomes with a horizon that features link to.
// add and edit build a capture document, like the single-task commands.

var (
	goalID, goalDesc, goalHorizon, goalStatus, goalMilestone, goalTitle string
	goalFeatures                                                        []string
	goalListJSON                                                        bool
)

var goalCmd = &cobra.Command{
	Use:   "goal",
	Short: "The roadmap: goals with a horizon, and the features serving them",
	Long: `Goals are what a ROADMAP.md holds: outcomes on now, next or later, with an
optional milestone. A goal can be just an idea — it needs no features yet —
or shipped, or deliberately out of scope. Features and requirements link to
the goal they serve.

  roady goal add "Offline mode" --horizon next --milestone v2.0
  roady goal edit goal-offline-mode --horizon now --feature sync
  roady goal list

Goals order work; moving one between horizons does not reopen the plan's
approval. Capture documents take goals too (see docs/capture.md).`,
}

var goalListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the roadmap",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, plan, err := loadEditPlan()
		if err != nil {
			return err
		}
		sp, err := ws.Repo.LoadSpec()
		if err != nil {
			return MapError(err)
		}
		state, _ := ws.Repo.LoadState()
		rm := application.BuildRoadmap(sp, plan, state)
		if goalListJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(rm)
		}
		rm.Render(cmd.OutOrStdout())
		return nil
	},
}

// goalEditFromFlags turns the flags the user set into a GoalEdit.
func goalEditFromFlags(cmd *cobra.Command, id string) application.GoalEdit {
	e := application.GoalEdit{ID: id, Features: goalFeatures}
	set := func(name string, v *string) *string {
		if cmd.Flags().Changed(name) {
			s := *v
			return &s
		}
		return nil
	}
	e.Title = set("title", &goalTitle)
	e.Description = set("description", &goalDesc)
	e.Horizon = set("horizon", &goalHorizon)
	e.Status = set("status", &goalStatus)
	e.Milestone = set("milestone", &goalMilestone)
	return e
}

var goalAddCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a goal to the roadmap",
	Long: `Add a goal. Its id is goal-<title> unless --id is given; adding the same
title again changes nothing.

  roady goal add "Offline mode" --horizon next
  roady goal add "Plugin marketplace" --status idea --horizon later
  roady goal add "Matching Jira feature for feature" --status out_of_scope`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, _, err := loadEditPlan()
		if err != nil {
			return err
		}
		sp, _ := ws.Repo.LoadSpec()
		e := goalEditFromFlags(cmd, goalID)
		title := args[0]
		e.Title = &title
		doc, id, err := application.AddGoalDoc(sp, e)
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, false, "Goal "+id)
	},
}

var goalEditCmd = &cobra.Command{
	Use:   "edit <goal-id>",
	Short: "Change a goal: move it between horizons, ship it, link features",
	Long: `Change one goal's fields; anything not given stays as it is.

  roady goal edit goal-offline-mode --horizon now
  roady goal edit goal-offline-mode --status shipped --milestone v2.0
  roady goal edit goal-offline-mode --feature sync --feature conflicts`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, _, err := loadEditPlan()
		if err != nil {
			return err
		}
		sp, _ := ws.Repo.LoadSpec()
		doc, id, err := application.EditGoalDoc(sp, goalEditFromFlags(cmd, args[0]))
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, false, "Goal "+id)
	},
}

func addGoalFlags(c *cobra.Command) {
	c.Flags().StringVar(&goalDesc, "description", "", "What the goal is for")
	c.Flags().StringVar(&goalHorizon, "horizon", "", "now, next or later")
	c.Flags().StringVar(&goalStatus, "status", "", "idea, planned, shipped or out_of_scope")
	c.Flags().StringVar(&goalMilestone, "milestone", "", "Release or checkpoint, e.g. v2.0")
	c.Flags().StringSliceVar(&goalFeatures, "feature", nil, "Link a feature to the goal (repeatable)")
	c.Flags().BoolVar(&editDryRun, "dry-run", false, "Show what would change without writing")
	c.Flags().BoolVar(&editJSON, "json", false, "Print the result as JSON")
}

func init() {
	addGoalFlags(goalAddCmd)
	goalAddCmd.Flags().StringVar(&goalID, "id", "", "Goal id (default goal-<title>)")
	addGoalFlags(goalEditCmd)
	goalEditCmd.Flags().StringVar(&goalTitle, "title", "", "New title")
	goalListCmd.Flags().BoolVar(&goalListJSON, "json", false, "Print the roadmap as JSON")
	goalCmd.AddCommand(goalListCmd, goalAddCmd, goalEditCmd)
	RootCmd.AddCommand(goalCmd)
}
