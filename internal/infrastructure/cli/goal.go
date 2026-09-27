package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

// Goals are the roadmap: outcomes with a horizon that features link to.
// add and edit build a capture document, like the single-task commands.

var (
	goalID, goalDesc, goalHorizon, goalStatus, goalMilestone, goalTitle string
	goalFeatures                                                        []string
	goalListJSON                                                        bool
	goalRenderOut                                                       string
	goalRenderCheck, goalRenderForce                                    bool
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

var goalRenderCmd = &cobra.Command{
	Use:   "render",
	Short: "Write ROADMAP.md from the goals",
	Long: `Render the goals as ROADMAP.md at the repository root: now, next, later,
ideas, shipped and out of scope, each goal with its milestone, description
and features. Task progress is left out, so the file changes only when the
roadmap does.

The first line marks the file as generated and carries a hash of the rest,
so ` + "`roady drift detect`" + ` reports an edit made to the file instead of to the goals,
and a file the goals have moved past. A hand edit is not overwritten
without --force: move it into the goals first.

  roady goal render                 # write ROADMAP.md
  roady goal render --out -         # print it
  roady goal render --check         # exit 1 unless ROADMAP.md is up to date (CI)`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, _, err := loadEditPlan()
		if err != nil {
			return err
		}
		sp, err := ws.Repo.LoadSpec()
		if err != nil {
			return MapError(err)
		}
		content := application.RenderRoadmapMarkdown(sp)
		out := cmd.OutOrStdout()
		if goalRenderOut == "-" {
			_, err := fmt.Fprint(out, content)
			return err
		}
		path := goalRenderOut
		if path == "" {
			pol, _ := ws.Repo.LoadPolicy()
			p, ok := application.RoadmapPath(ws.Repo.Root(), pol, ws.Repo.IsSubProject())
			if !ok {
				return fmt.Errorf("a sub-project's roadmap has no default file; say where with --out, or set roadmap: in its policy.yaml")
			}
			path = p
		}
		if goalRenderCheck {
			existing, err := os.ReadFile(path)
			if err == nil && application.CompareRoadmap(string(existing), sp) == application.RoadmapInSync {
				_, _ = fmt.Fprintf(out, "%s is up to date.\n", path)
				return nil
			}
			return fmt.Errorf("%s is not up to date with the goals; run `roady goal render`", path)
		}
		res, err := application.WriteRoadmap(path, sp, goalRenderForce)
		if err != nil {
			return err
		}
		if res.Written {
			_, _ = fmt.Fprintf(out, "Wrote %s.\n", path)
		} else {
			_, _ = fmt.Fprintf(out, "%s is already up to date.\n", path)
		}
		return nil
	},
}

var goalImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Read goals from a roadmap file kept by hand (Now / Next / Later / Done)",
	Long: `Move a hand-kept roadmap into goals. The file's ## headings name the
sections — Now, Next, Later, Ideas, Done (or Shipped) and Out of scope — and
each ### heading or top-level bullet under one is a goal: a bold lead or the
text before a dash or colon is its title, the rest its description, and a
trailing "(v1.2)" its milestone. Other sections are skipped and named.

Goal ids are goal-<title>, so importing the same file again changes nothing.
Afterwards the goals are the roadmap: render the file from them with
` + "`roady goal render --out <file> --force`" + `, or delete it.

  roady goal import ROADMAP.md --dry-run
  roady goal import memory/roadmap.md`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, _, err := loadEditPlan()
		if err != nil {
			return err
		}
		f, err := os.Open(args[0])
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		imp, err := application.ImportRoadmapMarkdown(f)
		if err != nil {
			return err
		}
		if len(imp.Doc.Goals) == 0 {
			return fmt.Errorf("no goals found in %s: expected ## Now / Next / Later / Done sections with a ### heading or bullet per goal", args[0])
		}
		if len(imp.Skipped) > 0 && !editJSON {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Skipped sections that are not a horizon: %s\n", strings.Join(imp.Skipped, ", "))
		}
		return applyEditVia(cmd, ws, imp.Doc, false, fmt.Sprintf("Imported %d goal(s) from %s", len(imp.Doc.Goals), args[0]), application.ViaRoadmapImport)
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
	goalRenderCmd.Flags().StringVar(&goalRenderOut, "out", "", "Write here instead of ROADMAP.md at the repository root; - prints it")
	goalRenderCmd.Flags().BoolVar(&goalRenderCheck, "check", false, "Write nothing; fail unless the file is up to date")
	goalRenderCmd.Flags().BoolVar(&goalRenderForce, "force", false, "Replace a file that was edited by hand or not written by roady")
	goalImportCmd.Flags().BoolVar(&editDryRun, "dry-run", false, "Show what would change without writing")
	goalImportCmd.Flags().BoolVar(&editJSON, "json", false, "Print the result as JSON")
	goalCmd.AddCommand(goalListCmd, goalAddCmd, goalEditCmd, goalRenderCmd, goalImportCmd)
	RootCmd.AddCommand(goalCmd)
}
