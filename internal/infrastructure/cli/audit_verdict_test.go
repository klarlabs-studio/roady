package cli

import (
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
)

// The verdict must never reassure over a removal. Deleting one event from the
// middle of events.jsonl produced a missing-parent finding followed by "nothing
// here is evidence of alteration".
func TestAuditVerdict(t *testing.T) {
	const reassurance = "nothing here is evidence of alteration"
	tests := []struct {
		name       string
		counts     map[domain.ViolationKind]int
		reassures  bool
		wantPhrase string
	}{
		{"legacy only", map[domain.ViolationKind]int{domain.KindLegacyUnverifiable: 73, domain.KindUnhashed: 13}, true, reassurance},
		{"removed entry", map[domain.ViolationKind]int{domain.KindMissingParent: 1}, false, "altered or had entries removed"},
		{"removed among legacy", map[domain.ViolationKind]int{domain.KindMissingParent: 1, domain.KindLegacyUnverifiable: 73}, false, "1 finding(s)"},
		{"hash mismatch", map[domain.ViolationKind]int{domain.KindHashMismatch: 2}, false, "2 finding(s)"},
		{"duplicate", map[domain.ViolationKind]int{domain.KindDuplicate: 1}, false, "unexplained"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := auditVerdict(tc.counts)
			if strings.Contains(got, reassurance) != tc.reassures {
				t.Errorf("reassures = %v, want %v: %q", !tc.reassures, tc.reassures, got)
			}
			if !strings.Contains(got, tc.wantPhrase) {
				t.Errorf("verdict %q does not contain %q", got, tc.wantPhrase)
			}
		})
	}
}
