package cli

import (
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/storage"
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

		// The chain cannot see a truncated tail, since nothing references the
		// newest entries; a committed copy of the log can.
		eventsPath, err := workspace.Repo.ResolvePath(storage.EventsFile)
		if err != nil {
			return fmt.Errorf("resolve audit log: %w", err)
		}
		baseline, err := service.VerifyAgainstCommitted(cwd, eventsPath, auditBaseline)
		if err != nil {
			return fmt.Errorf("baseline verification failed: %w", err)
		}
		violations = append(violations, baseline.Violations...)

		if len(violations) == 0 {
			fmt.Println("Audit trail is intact and verified.")
			fmt.Println(baselineNote(baseline))
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
			{domain.KindMissingParent, "removed (an entry that later entries reference is missing)"},
			{domain.KindUnknownAlgo, "written with an algorithm this build cannot verify"},
			{domain.KindLegacyUnverifiable, "predating hash_algo, unverifiable either way"},
			{domain.KindRemovedSinceBaseline, "removed since " + baseline.Ref + " (committed, now missing)"},
		} {
			if n := counts[row.kind]; n > 0 {
				fmt.Printf("  %4d  %s\n", n, row.label)
			}
		}
		fmt.Println("\n" + auditVerdict(counts))
		fmt.Println(baselineNote(baseline))
		os.Exit(1)
		return nil
	},
}

// auditVerdict is the closing line of a failed verification.
//
// It used to reassure whenever no hash failed to reproduce, which covered a
// log with an entry deleted from the middle: the chain reported the removal and
// the verdict underneath said nothing here was evidence of alteration. The
// reassurance is now reserved for logs whose only findings are history this
// build cannot check.
func auditVerdict(counts map[domain.ViolationKind]int) string {
	altered, unexplained := 0, 0
	for kind, n := range counts {
		switch {
		case kind.EvidencesAlteration():
			altered += n
		case kind.Unexplained():
			unexplained += n
		}
	}
	switch {
	case altered > 0:
		return fmt.Sprintf("%d finding(s) are evidence that the log was altered or had entries removed after writing.", altered)
	case unexplained > 0:
		return fmt.Sprintf("No entry was shown to be altered or removed, but %d finding(s) are unexplained and should be investigated.", unexplained)
	default:
		return "No entry failed under an algorithm this build can verify: nothing here is evidence of alteration."
	}
}

// baselineNote says what the comparison with committed history covered, so a
// clean result is never read as ruling out a truncation it did not check.
func baselineNote(b application.BaselineCheck) string {
	if b.Checked {
		return fmt.Sprintf("Compared with the log committed at %s: every committed entry is accounted for unless reported above.", b.Ref)
	}
	return fmt.Sprintf("Not compared with committed history (%s), so removal of the newest entries cannot be ruled out. Pass --baseline <ref> in a git repository.", b.Reason)
}

var auditBaseline string

func init() {
	auditVerifyCmd.Flags().StringVar(&auditBaseline, "baseline", application.DefaultAuditBaseline,
		"Git revision whose committed events.jsonl must still be fully present (e.g. origin/main in CI; empty to skip)")
	auditCmd.AddCommand(auditVerifyCmd)
	RootCmd.AddCommand(auditCmd)
}
