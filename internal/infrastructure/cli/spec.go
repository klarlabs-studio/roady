package cli

import (
	"fmt"
	"os"

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

		// Shape is not agreement. The lock is the drift baseline, so a spec
		// that validates while the lock describes a different project is a
		// false green at the moment it matters most — the first real drift
		// check would compare against a spec this project never had.
		status, sErr := application.NewSpecService(repo).LockStatus()
		if sErr == nil {
			for _, p := range status.Problems() {
				fmt.Fprintf(os.Stderr, "warning: %s\n", p)
			}
		}
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
		result, err := service.AddFeature(title, desc)
		if err != nil {
			return MapError(fmt.Errorf("failed to add feature: %w", err))
		}

		fmt.Printf("Successfully added feature '%s'. (Total features: %d)\n", title, len(result.Spec.Features))
		if result.Synced() {
			fmt.Printf("Intent synced to %s\n", result.BacklogPath)
		}
		for _, w := range result.Warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
		return nil
	},
}

var specReviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Emit a prompt asking your model to review the spec",
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

var specLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Re-capture the drift baseline from the current spec",
	Long: `Re-capture spec.lock.json from spec.yaml, and reconcile state.json's
project id with it.

The lock is what every drift check compares against. ` + "`roady init`" + ` writes it
alongside the spec, so they agree — but the ordinary way to adopt Roady in an
existing project is to replace the generated spec, and nothing re-derived the
lock from it. Until now the only way to fix that was to hand-write the JSON,
which works until it silently does not.

Re-running is a no-op when everything already agrees.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		repo := wiring.NewWorkspace(cwd).Repo

		result, err := application.NewSpecService(repo).WriteLock()
		if err != nil {
			return MapError(err)
		}

		if !result.Changed() {
			fmt.Printf("Already in sync with spec %q; nothing to do.\n", result.SpecID)
			return nil
		}
		if result.LockUpdated {
			fmt.Printf("Re-captured spec.lock.json for %q.\n", result.SpecID)
		}
		if result.StateUpdated {
			fmt.Printf("Reconciled state.json project_id to %q.\n", result.SpecID)
		}
		return nil
	},
}

func init() {
	specCmd.AddCommand(specLockCmd)
	specCmd.AddCommand(specAddCmd)
	specAnalyzeCmd.Flags().BoolVar(&reconcileSpec, "reconcile", false, "Removed: Roady no longer runs inference. Use 'roady spec explain' and write the result back with 'roady spec add'")
	specCmd.AddCommand(specImportCmd)
	specCmd.AddCommand(specValidateCmd)
	specCmd.AddCommand(specExplainCmd)
	specCmd.AddCommand(specReviewCmd)
	specCmd.AddCommand(specAnalyzeCmd)
	RootCmd.AddCommand(specCmd)
}
