package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/felixgeelhaar/roady/internal/infrastructure/watch"
	"github.com/spf13/cobra"
)

var (
	autoSync       bool
	reconcile      bool
	debounceWindow time.Duration
	includeGlobs   string
	excludeGlobs   string
)

var watchCmd = &cobra.Command{

	Use: "watch [dir]",

	Short: "Watch a directory for documentation changes and automatically detect drift",

	RunE: func(cmd *cobra.Command, args []string) error {
		dir := "docs"

		if len(args) > 0 {
			dir = args[0]
		}

		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		specSvc := services.Spec
		driftSvc := services.Drift
		planSvc := services.Plan

		fmt.Printf("Watching %s for changes... (Auto-sync: %v, Reconcile: %v)\n", dir, autoSync, reconcile)

		lastHash := ""
		if seed := os.Getenv("ROADY_WATCH_SEED_HASH"); seed != "" {
			lastHash = seed
		}
		once := os.Getenv("ROADY_WATCH_ONCE") == "true"

		handleChange := func(_ watch.ChangeEvent) {
			currentSpec, err := specSvc.AnalyzeDirectory(dir)
			if err != nil {
				return
			}

			currentHash := currentSpec.Hash()
			if currentHash == lastHash {
				return
			}

			if lastHash != "" {
				fmt.Printf("\nDocumentation change detected at %s\n", time.Now().Format("15:04:05"))

				if reconcile {
					fmt.Println("Reconcile: spec analyze → drift detect → accept drift → plan generate")
					// Step 1: spec is already analyzed above
					// Step 2: detect drift
					report, err := driftSvc.DetectDrift(cmd.Context())
					if err == nil && len(report.Issues) > 0 {
						fmt.Printf("Drift detected: %d issues\n", len(report.Issues))
						// Step 3: accept drift
						if err := driftSvc.AcceptDrift(); err != nil {
							fmt.Printf("Accept drift failed: %v\n", err)
						} else {
							fmt.Println("Drift accepted.")
						}
					}
					// Step 4: generate plan
					if _, err := planSvc.GeneratePlan(cmd.Context()); err != nil {
						fmt.Printf("Plan generation failed: %v\n", err)
					} else {
						fmt.Println("Plan regenerated.")
					}
				} else if autoSync {
					// Regenerates from the spec with the deterministic
					// planner. This used to call a model; Roady no longer
					// runs inference, and an unattended file watcher is the
					// last place that should silently spend tokens anyway.
					fmt.Println("Autonomous Reconciliation: Synchronizing plan with new intent...")
					if _, err := planSvc.GeneratePlan(cmd.Context()); err != nil {
						fmt.Printf("Auto-sync failed: %v\n", err)
					} else {
						fmt.Println("Plan successfully synchronized.")
					}

					report, err := driftSvc.DetectDrift(cmd.Context())
					if err == nil && len(report.Issues) > 0 {
						fmt.Printf("Drift detected: %d issues found.\n", len(report.Issues))
						for _, iss := range report.Issues {
							fmt.Printf("  - [%s] %s\n", iss.Severity, iss.Message)
						}
					} else {
						fmt.Println("Intent and Plan are in sync.")
					}
				} else {
					report, err := driftSvc.DetectDrift(cmd.Context())
					if err == nil && len(report.Issues) > 0 {
						fmt.Printf("Drift detected: %d issues found.\n", len(report.Issues))
						for _, iss := range report.Issues {
							fmt.Printf("  - [%s] %s\n", iss.Severity, iss.Message)
						}
					} else {
						fmt.Println("Intent and Plan are in sync.")
					}
				}
			}
			lastHash = currentHash
		}

		// Single-pass mode for testing
		if once {
			handleChange(watch.ChangeEvent{})
			return nil
		}

		// Use fsnotify-based watcher
		watcher, err := watch.NewFSWatcher(debounceWindow, handleChange)
		if err != nil {
			return fmt.Errorf("create watcher: %w", err)
		}

		// Set pattern filter if include/exclude flags provided
		if includeGlobs != "" || excludeGlobs != "" {
			var include, exclude []string
			if includeGlobs != "" {
				include = strings.Split(includeGlobs, ",")
			}
			if excludeGlobs != "" {
				exclude = strings.Split(excludeGlobs, ",")
			}
			watcher.SetFilter(watch.NewPatternFilter(include, exclude))
		}

		if err := watcher.WatchRecursive(dir); err != nil {
			return fmt.Errorf("watch directory: %w", err)
		}

		// Perform initial analysis
		handleChange(watch.ChangeEvent{})

		// Graceful shutdown on signal
		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\nStopping watcher...")
			cancel()
		}()

		return watcher.Run(ctx)
	},
}

func init() {
	watchCmd.Flags().BoolVar(&autoSync, "auto-sync", false, "Automatically regenerate plan on documentation changes")
	watchCmd.Flags().BoolVar(&reconcile, "reconcile", false, "Full reconcile: spec analyze → drift detect → accept → plan generate")
	watchCmd.Flags().DurationVar(&debounceWindow, "debounce", 500*time.Millisecond, "Debounce window for file change events")
	watchCmd.Flags().StringVar(&includeGlobs, "include", "", "Comma-separated glob patterns to include (e.g. '*.md,*.txt')")
	watchCmd.Flags().StringVar(&excludeGlobs, "exclude", "", "Comma-separated glob patterns to exclude (e.g. '*.tmp,*.log')")
	RootCmd.AddCommand(watchCmd)
}
