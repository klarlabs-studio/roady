package domain

import "testing"

// entry is a link that verifies cleanly unless the test says otherwise.
func entry(id, hash, prev string) ChainEntry {
	return ChainEntry{ID: id, Hash: hash, PrevHash: prev, Verifiable: true, Matches: true}
}

// The property the linear check destroyed: two collaborators appending
// concurrently produce branches from a shared parent, and git union-merges
// them in whatever order the timestamps fall. Requiring each entry to follow
// the previous *line* rejected every such merge, which is what made concurrent
// work impossible — and what one of the two verifiers still did.
func TestVerifyChainAcceptsConcurrentBranches(t *testing.T) {
	// a → b and a → c, merged into one file in either order.
	entries := []ChainEntry{
		entry("a", "h-a", ""),
		entry("b", "h-b", "h-a"),
		entry("c", "h-c", "h-a"), // same parent as b: a branch, not tampering
		entry("d", "h-d", "h-b"),
	}

	if violations := VerifyChain(entries); len(violations) != 0 {
		t.Errorf("a branching log was reported as broken: %v", violations)
	}
}

// Interleaved order must not matter either: git decides the order, not Roady.
func TestVerifyChainIsOrderIndependent(t *testing.T) {
	entries := []ChainEntry{
		entry("d", "h-d", "h-b"), // child before its parent appears
		entry("a", "h-a", ""),
		entry("c", "h-c", "h-a"),
		entry("b", "h-b", "h-a"),
	}

	if violations := VerifyChain(entries); len(violations) != 0 {
		t.Errorf("an out-of-order log was reported as broken: %v", violations)
	}
}

// What the links still prove: nothing referenced has been removed.
func TestVerifyChainCatchesARemovedParent(t *testing.T) {
	entries := []ChainEntry{
		entry("a", "h-a", ""),
		entry("c", "h-c", "h-b"), // h-b is gone
	}

	violations := VerifyChain(entries)
	if len(violations) != 1 {
		t.Fatalf("got %d violations, want 1: %v", len(violations), violations)
	}
	if !contains(violations[0], "missing parent") {
		t.Errorf("violation does not name the cause: %q", violations[0])
	}
}

func TestVerifyChainDistinguishesItsFindings(t *testing.T) {
	tests := []struct {
		name  string
		entry ChainEntry
		want  string
	}{
		{"no hash is not tampering", ChainEntry{ID: "x", Verifiable: true}, "outside the chain"},
		{"unknown algorithm is not tampering", ChainEntry{ID: "x", Hash: "h", HashAlgo: "sha512-v9"}, "cannot verify"},
		{"content that does not reproduce", ChainEntry{ID: "x", Hash: "h", Verifiable: true}, "does not reproduce"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := VerifyChain([]ChainEntry{tt.entry})
			if len(violations) != 1 {
				t.Fatalf("got %d violations, want 1: %v", len(violations), violations)
			}
			if !contains(violations[0], tt.want) {
				t.Errorf("violation %q does not say %q", violations[0], tt.want)
			}
		})
	}
}

func TestVerifyChainReportsDuplicates(t *testing.T) {
	entries := []ChainEntry{entry("a", "h-a", ""), entry("a", "h-a2", "h-a")}

	violations := VerifyChain(entries)
	if len(violations) == 0 || !contains(violations[0], "same event twice") {
		t.Errorf("a duplicated event was not reported: %v", violations)
	}
}

func TestVerifyChainEmpty(t *testing.T) {
	if v := VerifyChain(nil); len(v) != 0 {
		t.Errorf("an empty log produced %v", v)
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
