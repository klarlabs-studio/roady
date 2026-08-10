package cli

import (
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Audit and verify project history",
}

var auditVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify the integrity of the project audit trail",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		workspace := wiring.NewWorkspace(cwd)
		service := application.NewAuditService(workspace.Repo)

		fmt.Println("Verifying audit trail integrity...")
		violations, err := service.VerifyIntegrityDetailed()
		if err != nil {
			return fmt.Errorf("verification failed: %w", err)
		}

		if len(violations) == 0 {
			fmt.Println("Audit trail is intact and verified.")
			return nil
		}

		fmt.Printf("Found %d integrity violations:\n", len(violations))
		for _, v := range violations {
			fmt.Printf("  - %s\n", v.Message)
		}

		// A bare count invites the wrong conclusion in both directions. A log
		// carrying old entries nobody can check reads as compromised, and a
		// single altered entry buried in a hundred of them reads as more of the
		// same. Say which is which, and say plainly when nothing failed under an
		// algorithm this build can actually verify.
		counts := map[domain.ViolationKind]int{}
		for _, v := range violations {
			counts[v.Kind]++
		}
		fmt.Println("\nSummary:")
		for _, row := range []struct {
			kind  domain.ViolationKind
			label string
		}{
			{domain.KindHashMismatch, "altered after writing (hash does not reproduce under the algorithm named)"},
			{domain.KindUnhashed, "appended outside roady (no hash)"},
			{domain.KindDuplicate, "duplicated in the log"},
			{domain.KindMissingParent, "referencing a removed parent"},
			{domain.KindUnknownAlgo, "written with an algorithm this build cannot verify"},
			{domain.KindLegacyUnverifiable, "predating hash_algo, unverifiable either way"},
		} {
			if n := counts[row.kind]; n > 0 {
				fmt.Printf("  %4d  %s\n", n, row.label)
			}
		}
		if counts[domain.KindHashMismatch] == 0 {
			fmt.Println("\nNo entry failed under an algorithm this build can verify:" +
				" nothing here is evidence of alteration.")
		}
		os.Exit(1)
		return nil
	},
}

func init() {
	auditCmd.AddCommand(auditVerifyCmd)
	RootCmd.AddCommand(auditCmd)
}
