package storage

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain"
)

// TestHashAlgoConstantsAgree pins the two writers together. They live in
// different packages and hash into the same file; if the constants drift, half
// the log carries a stamp the verifier does not recognise and every one of
// those entries reports as unverifiable.
func TestHashAlgoConstantsAgree(t *testing.T) {
	if HashAlgoCurrent != domain.HashAlgoCurrent {
		t.Errorf("storage %q != domain %q — the two writers would stamp different algorithms",
			HashAlgoCurrent, domain.HashAlgoCurrent)
	}
}
