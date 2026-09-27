package cli

import (
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the health of the Roady environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("Running Roady Doctor...")

		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		workspace := wiring.NewWorkspace(cwd)
		repo := workspace.Repo

		hasIssues := false
		check := func(name string, fn func() error) {
			fmt.Printf("Checking %s... ", name)
			if err := fn(); err != nil {
				fmt.Printf("FAIL\n  Error: %v\n", err)
				hasIssues = true
			} else {
				fmt.Printf("PASS\n")
			}
		}

		check("Initialization", func() error {
			if !repo.IsInitialized() {
				return fmt.Errorf(".roady directory not found (run 'roady init')")
			}
			return nil
		})

		check("Spec File", func() error {
			_, err := repo.LoadSpec()
			return err
		})

		check("Policy File", func() error {
			_, err := repo.LoadPolicy()
			return err
		})

		check("Plan File", func() error {
			_, err := repo.LoadPlan()
			return err
		})

		check("Execution State", func() error {
			state, err := repo.LoadState()
			if err != nil {
				return err
			}
			if state.ProjectID == "unknown" {
				return fmt.Errorf("project ID is 'unknown' in state.json")
			}
			return nil
		})

		if path, shared := repo.StateLocation(); shared {
			fmt.Printf("ℹ️  Execution state is shared by all worktrees: %s (.roady/state.json is a mirror)\n", path)
		}

		check("Audit Trail", func() error {
			path, err := repo.ResolvePath("events.jsonl")
			if err != nil {
				return err
			}
			_, err = os.Stat(path)
			return err
		})

		check("Audit Integrity", func() error {
			// The same judgement as `roady audit verify`: entries this build
			// cannot check (unhashed, or hashed before hash_algo existed) are
			// history, not evidence. Counting them as violations made doctor
			// fail a log that verify calls intact.
			findings, err := application.NewAuditService(repo).VerifyIntegrityDetailed()
			if err != nil {
				return err
			}
			alteration, unchecked := 0, 0
			for _, f := range findings {
				if f.Kind.EvidencesAlteration() || f.Kind.Unexplained() {
					alteration++
				} else {
					unchecked++
				}
			}
			if alteration > 0 {
				return fmt.Errorf("%d finding(s) that may mean the log was altered (run 'roady audit verify')", alteration)
			}
			if unchecked > 0 {
				fmt.Printf("ℹ️  %d older audit entries cannot be checked by this build; none is evidence of alteration\n", unchecked)
			}
			return nil
		})

		if hasIssues {
			fmt.Println("\nissues found! Please fix them before continuing.")
			return fmt.Errorf("doctor found issues")
		}
		fmt.Println("\nEverything looks good!")
		return nil
	},
}

func init() {
	RootCmd.AddCommand(doctorCmd)
}
