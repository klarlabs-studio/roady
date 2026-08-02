package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage execution plans",
}

var useAI bool

var planGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate a plan from the current spec",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		// --ai no longer runs a model. Roady assembles the decomposition
		// prompt and hands it back; the caller runs inference with whatever
		// model it already has and returns the tasks through
		// `roady_update_plan`.
		if useAI {
			req, pErr := services.Prompt.DecomposeSpec(cmd.Context())
			if pErr != nil {
				return MapError(pErr)
			}
			return printPromptRequest(req, promptJSON)
		}

		plan, err := services.Plan.GeneratePlan(cmd.Context())

		if err != nil {
			return MapError(err)
		}

		if plan == nil {
			return fmt.Errorf("plan generation returned no plan and no error")
		}

		fmt.Printf("Successfully generated plan: %s\n", plan.ID)
		fmt.Printf("Status: %s\n", plan.ApprovalStatus)
		fmt.Printf("Tasks generated: %d\n", len(plan.Tasks))

		state, _ := services.Workspace.Repo.LoadState()
		for _, t := range plan.Tasks {
			status := "pending"
			if res, ok := state.TaskStates[t.ID]; ok {
				status = string(res.Status)
			}
			fmt.Printf("- [%s] %s (Priority: %s, Estimate: %s)\n", status, t.Title, t.Priority, t.Estimate)
		}
		return nil
	},
}

var planApproveCmd = &cobra.Command{

	Use: "approve",

	Short: "Approve the current plan for execution",

	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		if err := services.Plan.ApprovePlan(); err != nil {
			return MapError(fmt.Errorf("failed to approve plan: %w", err))
		}

		fmt.Println("Plan approved. You can now start tasks.")
		return nil
	},
}

var planRejectCmd = &cobra.Command{

	Use: "reject",

	Short: "Reject the current plan",

	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		if err := services.Plan.RejectPlan(); err != nil {
			return MapError(fmt.Errorf("failed to reject plan: %w", err))
		}

		fmt.Println("Plan rejected.")
		return nil
	},
}

var planPruneCmd = &cobra.Command{

	Use: "prune",

	Short: "Remove tasks from the plan that are no longer in the spec",

	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		if err := services.Plan.PrunePlan(); err != nil {
			return MapError(fmt.Errorf("failed to prune plan: %w", err))
		}

		fmt.Println("Plan pruned. Orphan tasks removed.")
		return nil
	},
}

var planPrioritizeCmd = &cobra.Command{
	Use:   "prioritize",
	Short: "Emit a prompt asking your model to suggest task priorities",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		req, pErr := services.Prompt.SuggestPriorities(cmd.Context())
		if pErr != nil {
			return MapError(pErr)
		}

		return printPromptRequest(req, promptJSON)
	},
}

var planSmartDecomposeCmd = &cobra.Command{
	Use:   "smart-decompose",
	Short: "Emit a codebase-aware decomposition prompt for your model",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		req, dErr := services.Prompt.DecomposeSpec(cmd.Context())
		if dErr != nil {
			return MapError(dErr)
		}

		return printPromptRequest(req, promptJSON)
	},
}

func init() {

	planGenerateCmd.Flags().BoolVar(&useAI, "ai", false, "Emit a decomposition prompt for your own model instead of the heuristic planner (Roady runs no inference)")

	planCmd.AddCommand(planGenerateCmd)

	planCmd.AddCommand(planApproveCmd)

	planCmd.AddCommand(planRejectCmd)

	planCmd.AddCommand(planPruneCmd)

	addPromptJSONFlag(planPrioritizeCmd, planSmartDecomposeCmd, planGenerateCmd)
	planCmd.AddCommand(planPrioritizeCmd)

	planCmd.AddCommand(planSmartDecomposeCmd)

	RootCmd.AddCommand(planCmd)

}
