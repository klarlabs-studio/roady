package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var (
	decideID, decideChoice, decideContext, decideConsequences, decideSupersedes string
	decideGoals, decideFeatures, decideReqs                                     []string
	decideList                                                                  bool
)

var decideCmd = &cobra.Command{
	Use:   "decide <title>",
	Short: "Record a decision, linked to what it constrains",
	Long: `Record a decision — what was decided, why, and what follows — linked to
the goals, features and requirements it applies to (none: project-wide).
` + "`roady next`" + ` shows the decisions behind the task you are on, so an agent does not
re-open or silently reverse them.

  roady decide "Session storage" --choice "Signed cookies, no server sessions" \
    --context "Stateless deploys; sessions under 4KB" --req jwt-sessions
  roady decide "Session storage" --choice "Redis" --id decision-session-redis \
    --supersedes decision-session-storage
  roady decide --list

Over MCP it is ` + "`roady_capture`" + ` with decisions. Recording a decision never
reopens the plan's approval.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if decideList {
			return cobra.NoArgs(cmd, args)
		}
		return cobra.ExactArgs(1)(cmd, args)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, _, err := loadEditPlan()
		if err != nil {
			return err
		}
		sp, err := ws.Repo.LoadSpec()
		if err != nil {
			return MapError(err)
		}
		if decideList {
			if editJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(sp.Decisions)
			}
			if len(sp.Decisions) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No decisions recorded.")
			}
			for _, d := range sp.Decisions {
				status := ""
				if !d.Stands() {
					status = " [superseded by " + d.SupersededBy + "]"
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s: %s%s\n", d.Date, d.ID, d.Title, d.Choice, status)
				if links := decisionLinks(d.Goals, d.Features, d.Requirements); links != "" {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    applies to %s\n", links)
				}
			}
			return nil
		}
		doc, id, err := application.DecideDoc(sp, application.Decide{
			ID: decideID, Title: args[0], Choice: decideChoice, Context: decideContext, Consequences: decideConsequences,
			Goals: decideGoals, Features: decideFeatures, Requirements: decideReqs, Supersedes: decideSupersedes,
		})
		if err != nil {
			return err
		}
		return applyEdit(cmd, ws, doc, false, "Decision "+id)
	},
}

func decisionLinks(goals, features, reqs []string) string {
	var parts []string
	for _, g := range goals {
		parts = append(parts, "goal "+g)
	}
	for _, f := range features {
		parts = append(parts, "feature "+f)
	}
	for _, r := range reqs {
		parts = append(parts, "requirement "+r)
	}
	return strings.Join(parts, ", ")
}

func init() {
	f := decideCmd.Flags()
	f.StringVar(&decideChoice, "choice", "", "What was chosen (required)")
	f.StringVar(&decideContext, "context", "", "Why the question came up; the options weighed")
	f.StringVar(&decideConsequences, "consequences", "", "What follows from it")
	f.StringVar(&decideID, "id", "", "Decision id (default decision-<title>)")
	f.StringVar(&decideSupersedes, "supersedes", "", "Id of an earlier decision this replaces")
	f.StringSliceVar(&decideGoals, "goal", nil, "Goal it applies to (repeatable)")
	f.StringSliceVar(&decideFeatures, "feature", nil, "Feature it applies to (repeatable)")
	f.StringSliceVar(&decideReqs, "req", nil, "Requirement it applies to (repeatable)")
	f.BoolVar(&decideList, "list", false, "List the decisions instead")
	f.BoolVar(&editDryRun, "dry-run", false, "Show what would change without writing")
	f.BoolVar(&editJSON, "json", false, "Print the result as JSON")
	RootCmd.AddCommand(decideCmd)
}
