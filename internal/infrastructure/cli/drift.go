package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between specs, plans, and code",
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

		if outputFormat == "json" {
			data, _ := json.MarshalIndent(report, "", "  ")
			fmt.Println(string(data))
			if len(report.Issues) > 0 {
				return fmt.Errorf("drift detected")
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

		return fmt.Errorf("drift detected")
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
	driftCmd.AddCommand(driftDetectCmd)
	addPromptJSONFlag(driftExplainCmd)
	driftCmd.AddCommand(driftExplainCmd)
	driftCmd.AddCommand(driftAcceptCmd)
	RootCmd.AddCommand(driftCmd)
}
