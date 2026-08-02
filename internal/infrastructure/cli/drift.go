package cli

import (
	"encoding/json"
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between specs, plans, and code",
}

// driftFailOn is the severity at or above which drift fails the command.
var driftFailOn string

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

		req, err := services.Prompt.ExplainDrift(cmd.Context(), report)
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
		for _, issue := range report.Issues {
			fmt.Printf("- [%s] (%s/%s) %s\n", issue.Severity, issue.Type, issue.Category, issue.Message)
			if issue.Hint != "" {
				fmt.Printf("  Hint: %s\n", issue.Hint)
			}
		}

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

var driftAcceptCmd = &cobra.Command{
	Use:   "accept",
	Short: "Accept current drift by locking the spec snapshot",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		if err := services.Drift.AcceptDrift(); err != nil {
			return MapError(fmt.Errorf("failed to accept drift: %w", err))
		}

		fmt.Println("Drift accepted. Spec snapshot locked.")
		return nil
	},
}

func init() {
	driftDetectCmd.Flags().StringP("output", "o", "text", "Output format (text, json)")
	driftDetectCmd.Flags().StringVar(&driftFailOn, "fail-on", "", "Exit non-zero only for drift at or above this severity (low, medium, high, critical)")
	driftCmd.AddCommand(driftDetectCmd)
	addPromptJSONFlag(driftExplainCmd)
	driftCmd.AddCommand(driftExplainCmd)
	driftCmd.AddCommand(driftAcceptCmd)
	RootCmd.AddCommand(driftCmd)
}
