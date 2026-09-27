package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/drift"
)

func TestDriftTextFoldsInfoIssues(t *testing.T) {
	var issues []drift.Issue
	for i := 0; i < 7; i++ {
		issues = append(issues, drift.Issue{Type: drift.DriftTypePlan, Category: drift.CategoryOrphan, Severity: drift.SeverityInfo, ComponentID: fmt.Sprintf("t%d", i), Message: "m", Hint: "h"})
	}
	issues = append(issues,
		drift.Issue{Type: drift.DriftTypePlan, Category: drift.CategoryUnplanned, Severity: drift.SeverityInfo, ComponentID: "u1", Message: "unplanned one"},
		drift.Issue{Type: drift.DriftTypePlan, Category: drift.CategoryOrphan, Severity: drift.SeverityMedium, ComponentID: "open", Message: "open orphan", Hint: "prune"})
	var b bytes.Buffer
	printDriftIssues(&b, issues)
	out := b.String()
	for _, want := range []string{"- [medium] (plan/ORPHAN) open orphan", "unplanned one", "7 finished tasks under features the spec no longer has, kept as history: t0, t1, t2, t3, t4, … 2 more"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "\n- ") != 2 {
		t.Errorf("expected three issue lines:\n%s", out)
	}
}
