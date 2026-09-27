package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between specs, plans, and code",
}

// driftChecks re-runs the acceptance checks of verified tasks.
var driftChecks bool

// driftFailOn is the severity at or above which drift fails the command.
var driftFailOn string

// driftWantPatch switches drift explain from prose to a patch request.
var driftWantPatch bool

// driftGateError reports how many issues tripped the gate, so a CI log says
// what to fix rather than only that something is wrong.
func driftGateError(gating []drift.Issue, threshold drift.Severity) error {
	return fmt.Errorf("drift detected: %d issue(s) at or above %s", len(gating), threshold)
}

var driftExplainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Provide an AI-generated explanation and resolution steps for current drift",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		report, err := services.Drift.DetectDrift(cmd.Context())
		if err != nil {
			return MapError(fmt.Errorf("failed to detect drift: %w", err))
		}

		// --patch asks for a diff instead of prose. Same context, different
		// artifact: one is read, the other is applied and reviewed.
		build := services.Prompt.ExplainDrift
		if driftWantPatch {
			build = services.Prompt.PatchDrift
		}

		req, err := build(cmd.Context(), report)
		if err != nil {
			return MapError(err)
		}

		return printPromptRequest(req, promptJSON)
	},
}

var driftDetectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Check for discrepancies between the current Spec and Plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFormat, _ := cmd.Flags().GetString("output")

		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		report, err := services.Drift.DetectDrift(cmd.Context())
		if err != nil {
			return MapError(fmt.Errorf("failed to detect drift: %w", err))
		}
		if driftChecks {
			actor := resolveCurrentOwner(gitConfigUserName)
			regressions, runs, err := services.Task.RecheckVerified(cmd.Context(), actor, application.CheckOptions{})
			if err != nil {
				return MapError(err)
			}
			if outputFormat != "json" {
				fmt.Printf("Re-ran the checks of %d verified task(s); %d fail now.\n", len(runs), len(regressions))
			}
			report.Issues = append(report.Issues, regressions...)
		}

		// --fail-on decides what makes the command exit non-zero. Without
		// it any drift at all fails, which is too blunt for CI: a
		// low-severity note would block a merge, and a gate that blocks on
		// noise gets switched off.
		threshold := drift.SeverityLow
		if driftFailOn != "" {
			parsed, pErr := drift.ParseSeverity(driftFailOn)
			if pErr != nil {
				return pErr
			}
			threshold = parsed
		}
		gating := report.AtOrAbove(threshold)

		if outputFormat == "json" {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
			if len(gating) > 0 {
				return driftGateError(gating, threshold)
			}
			return nil
		}

		if len(report.Issues) == 0 {
			fmt.Println("No drift detected. Project is in a healthy state.")
			return nil
		}

		fmt.Printf("Detected %d drift issues:\n", len(report.Issues))
		printDriftIssues(os.Stdout, report.Issues)

		if len(gating) == 0 {
			// Reported, but below the gate. Say so explicitly rather than
			// exiting 0 silently and leaving the operator unsure whether
			// the threshold applied.
			fmt.Printf("\nNone at or above %s — not failing.\n", threshold)
			return nil
		}

		return driftGateError(gating, threshold)
	},
}

var driftAcceptChangeChecks bool

var driftAcceptCmd = &cobra.Command{
	Use:   "accept",
	Short: "Accept current drift by locking the spec snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		actor := resolveCurrentOwner(gitConfigUserName)
		if actor == "" {
			actor = "unknown-human"
		}
		if err := services.Drift.AcceptDriftWith(driftAcceptChangeChecks, actor); err != nil {
			return MapError(fmt.Errorf("failed to accept drift: %w", err))
		}

		fmt.Println("Drift accepted. Spec snapshot locked.")
		return nil
	},
}

func init() {
	driftDetectCmd.Flags().StringP("output", "o", "text", "Output format (text, json)")
	driftDetectCmd.Flags().BoolVar(&driftChecks, "checks", false, "Also re-run the acceptance checks of verified tasks; a failing one is reported as a regression")
	driftDetectCmd.Flags().StringVar(&driftFailOn, "fail-on", "", "Exit non-zero only for drift at or above this severity (low, medium, high, critical)")
	driftCmd.AddCommand(driftDetectCmd)
	driftExplainCmd.Flags().BoolVar(&driftWantPatch, "patch", false, "Ask for a unified diff that closes the drift, instead of an explanation")
	addPromptJSONFlag(driftExplainCmd)
	driftCmd.AddCommand(driftExplainCmd)
	driftAcceptCmd.Flags().BoolVar(&driftAcceptChangeChecks, "change-checks", false, "Allow removing or changing the acceptance check of work already started (reopens done tasks; recorded as an override)")
	driftCmd.AddCommand(driftAcceptCmd)
	RootCmd.AddCommand(driftCmd)
}

// infoGroupAt is how many informational issues of one kind are listed one
// by one before they are summarised on a single line. A project adopting
// roady late can carry dozens of them; listed in full they bury the issues
// that need action.
const infoGroupAt = 4

// printDriftIssues writes issues as text: each problem with its hint, and
// informational issues of one kind folded into one line once there are
// more than a few.
func printDriftIssues(w io.Writer, issues []drift.Issue) {
	type group struct {
		ids  []string
		hint string
	}
	counts := map[string]int{}
	for _, is := range issues {
		if is.Severity == drift.SeverityInfo {
			counts[string(is.Type)+"/"+string(is.Category)]++
		}
	}
	groups := map[string]*group{}
	var order []string
	for _, is := range issues {
		key := string(is.Type) + "/" + string(is.Category)
		if is.Severity == drift.SeverityInfo && counts[key] >= infoGroupAt {
			g := groups[key]
			if g == nil {
				g = &group{hint: is.Hint}
				groups[key] = g
				order = append(order, key)
			}
			g.ids = append(g.ids, is.ComponentID)
			continue
		}
		_, _ = fmt.Fprintf(w, "- [%s] (%s/%s) %s\n", is.Severity, is.Type, is.Category, is.Message)
		if is.Hint != "" {
			_, _ = fmt.Fprintf(w, "  Hint: %s\n", is.Hint)
		}
	}
	for _, key := range order {
		g := groups[key]
		shown := g.ids
		more := ""
		if len(shown) > 5 {
			more = fmt.Sprintf(", … %d more", len(shown)-5)
			shown = shown[:5]
		}
		_, _ = fmt.Fprintf(w, "- [info] (%s) %d %s: %s%s\n", key, len(g.ids), infoGroupLabel(key), strings.Join(shown, ", "), more)
		_, _ = fmt.Fprintf(w, "  Hint: %s (`--output json` lists each.)\n", g.hint)
	}
}

func infoGroupLabel(key string) string {
	switch key {
	case string(drift.DriftTypePlan) + "/" + string(drift.CategoryOrphan):
		return "finished tasks under features the spec no longer has, kept as history"
	case string(drift.DriftTypePlan) + "/" + string(drift.CategoryUnplanned):
		return "unplanned tasks"
	}
	return "items"
}
