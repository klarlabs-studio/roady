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

var (
	planImportFormat      string
	planImportFeature     string
	planImportParallel    bool
	planImportIncludeDone bool
	planImportDryRun      bool
	planImportJSON        bool
)

var planImportCmd = &cobra.Command{
	Use:   "import <plan-file>",
	Short: "Turn a harness's plan file (Claude Code, Kiro, Codex ExecPlan, markdown) into tasks",
	Long: `Read the plan an agent already wrote and record it as roady tasks.

Recognised formats (detected, or forced with --format):
  kiro      .kiro/specs/<name>/tasks.md — numbered checkbox tasks; "2.1"
            sub-tasks become dependencies of "2"
  execplan  a Codex ExecPlan (PLANS.md) — the Progress checklist, or Concrete
            Steps before any progress is recorded
  markdown  any other plan (Claude Code plan mode, Cursor, Gemini CLI):
            list items under Steps/Implementation/Tasks, else list items
            outside narrative sections, else one task per sub-heading

Each step becomes task-<plan>-<step>, citing the plan's file and line, and
depends on the step before it (--parallel leaves them independent). Steps
already checked off are skipped (--include-done imports them as pending work).
Without --feature the plan becomes a new feature, which is new intent and
needs approval like any other.

The import is a capture: all or nothing, and running it again after editing
the plan updates the same tasks instead of adding new ones.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		ws := wiring.NewWorkspace(root)
		current, _ := ws.Repo.LoadSpec()
		imp, err := application.ImportPlanFile(args[0], current, application.PlanImportOptions{
			Format:      planImportFormat,
			FeatureID:   planImportFeature,
			Parallel:    planImportParallel,
			IncludeDone: planImportIncludeDone,
			Root:        root,
		})
		if err != nil {
			return MapError(fmt.Errorf("import plan: %w", err))
		}

		svc := application.NewCaptureService(ws.Repo, ws.Audit)
		actor := resolveCurrentOwner(gitConfigUserName)
		if actor == "" {
			actor = "unknown-human"
		}
		result, err := svc.Capture(imp.Doc, application.CaptureOptions{Actor: actor, DryRun: planImportDryRun, Origin: planning.OriginHuman})
		if err != nil {
			return MapError(fmt.Errorf("import plan: %w", err))
		}

		out := cmd.OutOrStdout()
		if planImportJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(map[string]any{"import": imp, "result": result}); err != nil {
				return err
			}
		} else {
			_, _ = fmt.Fprintf(out, "Plan %q (%s): %d step(s)", imp.Title, imp.Format, len(imp.Doc.Tasks))
			if imp.Skipped > 0 {
				_, _ = fmt.Fprintf(out, ", %d already checked off and skipped", imp.Skipped)
			}
			_, _ = fmt.Fprintln(out)
			printCaptureResult(out, result)
		}
		if len(result.Rejected) > 0 {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	planImportCmd.Flags().StringVar(&planImportFormat, "format", "auto", "Plan format: auto, kiro, execplan or markdown")
	planImportCmd.Flags().StringVar(&planImportFeature, "feature", "", "Attach the tasks to this existing feature instead of creating one for the plan")
	planImportCmd.Flags().BoolVar(&planImportParallel, "parallel", false, "Leave steps independent instead of chaining each to the one before")
	planImportCmd.Flags().BoolVar(&planImportIncludeDone, "include-done", false, "Import steps already checked off in the plan")
	planImportCmd.Flags().BoolVar(&planImportDryRun, "dry-run", false, "Report what would change without writing")
	planImportCmd.Flags().BoolVar(&planImportJSON, "json", false, "Print the import and its result as JSON")
	planCmd.AddCommand(planImportCmd)
}
