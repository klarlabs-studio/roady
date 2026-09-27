package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/spf13/cobra"
)

// Single-task commands. Each builds a capture document and applies it, so it
// is validated like any capture and recorded as one plan.capture event.

var (
	editDryRun bool
	editJSON   bool

	addID, addReq, addFeature, addDesc, addPriority, addEstimate string
	addAfter, addBefore                                          []string
	addGoal                                                      string
	addCheckRun, addCheckManual                                  string

	editTitle, editDesc, editPriority, editEstimate string
	editDependsOn, editAddDep, editDropDep          []string
	editCheckRun, editCheckManual                   string
	editChecks                                      bool

	splitSequential bool

	moveReq, moveFeature string
)

// applyEdit runs a capture built by one of the single-task commands.
func applyEdit(cmd *cobra.Command, ws *wiring.Workspace, doc application.CaptureDoc, allowChecks bool, summary string) error {
	return applyEditVia(cmd, ws, doc, allowChecks, summary, "")
}

// applyEditVia is applyEdit recording which path the capture came by.
func applyEditVia(cmd *cobra.Command, ws *wiring.Workspace, doc application.CaptureDoc, allowChecks bool, summary, via string) error {
	actor := resolveCurrentOwner(gitConfigUserName)
	if actor == "" {
		actor = "unknown-human"
	}
	result, err := application.NewCaptureService(ws.Repo, ws.Audit).Capture(doc, application.CaptureOptions{
		Actor: actor, DryRun: editDryRun, Origin: planning.OriginHuman, AllowCheckChange: allowChecks, Note: summary, Via: via,
	})
	if err != nil {
		return MapError(err)
	}
	out := cmd.OutOrStdout()
	if editJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			return err
		}
	} else {
		if summary != "" && result.Changed() && len(result.Rejected) == 0 {
			_, _ = fmt.Fprintln(out, summary)
		}
		printCaptureResult(out, result)
	}
	if len(result.Rejected) > 0 {
		os.Exit(1)
	}
	return nil
}

func loadEditPlan() (*wiring.Workspace, *planning.Plan, error) {
	root, err := getProjectRoot()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve project path: %w", err)
	}
	ws := wiring.NewWorkspace(root)
	plan, _ := ws.Repo.LoadPlan()
	return ws, plan, nil
}

func checkFromFlags(run, manual string) (*planning.Check, error) {
	switch {
	case run != "" && manual != "":
		return nil, fmt.Errorf("a check is either --check-run or --check-manual, not both")
	case run != "":
		return &planning.Check{Run: run}, nil
	case manual != "":
		return &planning.Check{Manual: manual}, nil
	}
	return nil, nil
}

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add one task",
	Long: `Add one task to the plan without writing a capture document.

  roady add "Handle an empty input file" --req pdf-gen --after task-pdf-gen
  roady add "Load test the endpoint" --after task-rate-limits --before task-release

The task belongs to --req (which implies its feature) or --feature; with
neither, it joins the feature of the first --after task. With none of them it
is unplanned work — a chore or quick fix outside the spec — kept in the
inbox, or under a roadmap goal with --goal; drift notes it but never prunes it. --after tasks become
its dependencies; --before tasks come to depend on it. The id is
task-<title>, or --id. Adding the same title again changes nothing.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, plan, err := loadEditPlan()
		if err != nil {
			return err
		}
		check, err := checkFromFlags(addCheckRun, addCheckManual)
		if err != nil {
			return err
		}
		doc, id, err := application.AddTaskDoc(plan, application.AddTask{
			Title: args[0], ID: addID, Description: addDesc, Requirement: addReq, Feature: addFeature,
			After: addAfter, Before: addBefore, Priority: addPriority, Estimate: addEstimate, Check: check, Goal: addGoal,
		})
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, false, "Task "+id)
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <task-id>",
	Short: "Change fields of one task",
	Long: `Change one task's fields; anything not given stays as it is.

  roady edit task-pdf-gen --priority high --estimate 2d
  roady edit task-pdf-gen --add-dep task-fonts --drop-dep task-old-renderer
  roady edit task-pdf-gen --check-run "go test ./pdf"

Changing the check of work already started needs --change-checks, and is
recorded as an override.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, plan, err := loadEditPlan()
		if err != nil {
			return err
		}
		check, err := checkFromFlags(editCheckRun, editCheckManual)
		if err != nil {
			return err
		}
		e := application.EditTask{AddDeps: editAddDep, DropDeps: editDropDep, Check: check}
		flags := cmd.Flags()
		if flags.Changed("title") {
			e.Title = &editTitle
		}
		if flags.Changed("description") {
			e.Description = &editDesc
		}
		if flags.Changed("priority") {
			e.Priority = &editPriority
		}
		if flags.Changed("estimate") {
			e.Estimate = &editEstimate
		}
		if flags.Changed("depends-on") {
			deps := append([]string{}, editDependsOn...)
			e.DependsOn = &deps
		}
		doc, err := application.EditTaskDoc(plan, args[0], e)
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, editChecks, "")
	},
}

var splitCmd = &cobra.Command{
	Use:   "split <task-id> <part> <part> [part...]",
	Short: "Break a task into parts",
	Long: `Break a task into two or more parts.

  roady split task-pdf-gen "Lay out the page" "Embed fonts" "Write the file"

Each part becomes task-<id>-<part> in the same feature. The parts take over
what the task waited for, and the task now waits for its parts: it stays where
it was for everything that depends on it, and keeps its acceptance check as
proof that the parts add up. --sequential chains the parts in order.`,
	Args: cobra.MinimumNArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, plan, err := loadEditPlan()
		if err != nil {
			return err
		}
		doc, ids, err := application.SplitTaskDoc(plan, args[0], args[1:], splitSequential)
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, false, fmt.Sprintf("Split %s into %d parts", args[0], len(ids)))
	},
}

var moveCmd = &cobra.Command{
	Use:   "move <task-id>",
	Short: "Move a task to another requirement or feature",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, plan, err := loadEditPlan()
		if err != nil {
			return err
		}
		doc, err := application.MoveTaskDoc(plan, args[0], moveReq, moveFeature)
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, false, "")
	},
}

func init() {
	for _, c := range []*cobra.Command{addCmd, editCmd, splitCmd, moveCmd} {
		c.Flags().BoolVar(&editDryRun, "dry-run", false, "Report what would change without writing")
		c.Flags().BoolVar(&editJSON, "json", false, "Print the result as JSON")
		RootCmd.AddCommand(c)
	}

	f := addCmd.Flags()
	f.StringVar(&addID, "id", "", "Task id (default task-<title>)")
	f.StringVar(&addReq, "req", "", "Requirement the task serves (implies its feature)")
	f.StringVar(&addFeature, "feature", "", "Feature the task belongs to")
	f.StringVarP(&addDesc, "description", "d", "", "Task description")
	f.StringVar(&addGoal, "goal", "", "Roadmap goal for unplanned work (a task with no --req or --feature)")
	f.StringSliceVar(&addAfter, "after", nil, "Tasks this one depends on (comma-separated or repeated)")
	f.StringSliceVar(&addBefore, "before", nil, "Tasks that should depend on this one")
	f.StringVarP(&addPriority, "priority", "p", "", "low, medium or high")
	f.StringVar(&addEstimate, "estimate", "", "Estimate, e.g. 4h or 2d")
	f.StringVar(&addCheckRun, "check-run", "", "Acceptance check: a command that exits 0 when done")
	f.StringVar(&addCheckManual, "check-manual", "", "Acceptance check a person confirms")

	f = editCmd.Flags()
	f.StringVar(&editTitle, "title", "", "New title")
	f.StringVarP(&editDesc, "description", "d", "", "New description")
	f.StringVarP(&editPriority, "priority", "p", "", "low, medium or high")
	f.StringVar(&editEstimate, "estimate", "", "New estimate")
	f.StringSliceVar(&editDependsOn, "depends-on", nil, "Replace the dependencies (empty to clear)")
	f.StringSliceVar(&editAddDep, "add-dep", nil, "Add dependencies")
	f.StringSliceVar(&editDropDep, "drop-dep", nil, "Remove dependencies")
	f.StringVar(&editCheckRun, "check-run", "", "Set a run check")
	f.StringVar(&editCheckManual, "check-manual", "", "Set a manual check")
	f.BoolVar(&editChecks, "change-checks", false, "Allow changing the check of work already started (recorded as an override)")

	splitCmd.Flags().BoolVar(&splitSequential, "sequential", false, "Chain the parts in the order given")

	moveCmd.Flags().StringVar(&moveReq, "req", "", "Requirement to move the task under")
	moveCmd.Flags().StringVar(&moveFeature, "feature", "", "Feature to move the task to")
}
