package cli

import (
	"fmt"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var specCmd = &cobra.Command{
	Use:   "spec",
	Short: "Manage product specifications",
}

var specExplainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Emit a prompt that explains the current spec",
	Long: `Assemble a prompt that explains the current specification.

Roady does not call a model. It gathers the spec context and hands you the
prompt; run it with whatever model you already have.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		req, err := services.Prompt.ExplainSpec(cmd.Context())
		if err != nil {
			return MapError(err)
		}

		return printPromptRequest(req, promptJSON)
	},
}

var specImportCmd = &cobra.Command{
	Use:   "import [file]",
	Short: "Import a spec from a markdown file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		repo := wiring.NewWorkspace(cwd).Repo
		service := application.NewSpecService(repo)
		filePath := args[0]

		spec, err := service.ImportFromMarkdown(filePath)
		if err != nil {
			return MapError(fmt.Errorf("failed to import spec: %w", err))
		}

		fmt.Printf("Successfully imported spec '%s' with %d features.\n", spec.Title, len(spec.Features))
		return nil
	},
}

var specValidateCmd = &cobra.Command{
	Use:     "validate",
	Aliases: []string{"lint"},
	Short:   "Validate the current specification (alias: lint)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		repo := wiring.NewWorkspace(cwd).Repo
		spec, err := repo.LoadSpec()
		if err != nil {
			return MapError(fmt.Errorf("failed to load/parse spec: %w", err))
		}

		errs := spec.Validate()
		if len(errs) > 0 {
			fmt.Println("Spec validation failed:")
			for _, e := range errs {
				fmt.Printf("- %v\n", e)
			}
			return fmt.Errorf("spec validation failed")
		}

		fmt.Println("Spec is valid and correctly formatted.")
		return nil
	},
}

var reconcileSpec bool

var specAnalyzeCmd = &cobra.Command{
	Use:   "analyze [dir]",
	Short: "Analyze a directory for markdown files and infer a product specification",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}

		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		workspace := wiring.NewWorkspace(cwd)
		repo := workspace.Repo
		service := application.NewSpecService(repo)

		spec, err := service.AnalyzeDirectory(dir)
		if err != nil {
			return MapError(fmt.Errorf("failed to analyze directory: %w", err))
		}

		if reconcileSpec {
			return fmt.Errorf("--reconcile called a language model to tidy the parsed spec; " +
				"Roady no longer runs inference. Run 'roady spec explain' to get the prompt, " +
				"reconcile with your own model, and write the result back with 'roady spec add'")
		}

		fmt.Printf("Successfully analyzed directory and generated spec '%s' with %d features.\n", spec.Title, len(spec.Features))
		return nil
	},
}

var specAddCmd = &cobra.Command{
	Use:   "add [title] [description]",
	Short: "Quickly add a new feature to the specification",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		repo := wiring.NewWorkspace(cwd).Repo
		service := application.NewSpecService(repo)

		title, desc := args[0], args[1]
		spec, err := service.AddFeature(title, desc)
		if err != nil {
			return MapError(fmt.Errorf("failed to add feature: %w", err))
		}

		fmt.Printf("Successfully added feature '%s'. (Total features: %d)\n", title, len(spec.Features))
		fmt.Println("Intent synced to docs/backlog.md")
		return nil
	},
}

var specReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Perform an AI-powered quality review of the current spec",
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		req, err := services.Prompt.ReviewSpec(cmd.Context())
		if err != nil {
			return MapError(err)
		}

		return printPromptRequest(req, promptJSON)
	},
}

func init() {
	specCmd.AddCommand(specAddCmd)
	specAnalyzeCmd.Flags().BoolVar(&reconcileSpec, "reconcile", false, "Use AI to semanticly deduplicate and reconcile the spec")
	specCmd.AddCommand(specImportCmd)
	specCmd.AddCommand(specValidateCmd)
	specCmd.AddCommand(specExplainCmd)
	specCmd.AddCommand(specReviewCmd)
	specCmd.AddCommand(specAnalyzeCmd)
	RootCmd.AddCommand(specCmd)
}
