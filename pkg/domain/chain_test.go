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
		// A hash that does not reproduce means two different things depending on
		// whether the entry says what wrote it. Only the first can be called
		// tampering; the second is history nobody can check.
		{"mismatch under a named algorithm is tampering", ChainEntry{ID: "x", Hash: "h", HashAlgo: "sha256-canonical-v1", Verifiable: true}, "does not reproduce"},
		{"mismatch with no named algorithm is unverifiable, not tampering", ChainEntry{ID: "x", Hash: "h", Verifiable: true}, "predates the hash_algo field"},
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

// A deleted entry is an alteration. The chain detected it all along; what was
// wrong was the classification the verdict read, which counted only hash
// mismatches as evidence and so reassured over a removal.
func TestRemovedEntryEvidencesAlteration(t *testing.T) {
	log := []ChainEntry{
		entry("a", "h1", ""),
		// "b" (h2) was deleted; "c" still names it as its parent.
		entry("c", "h3", "h2"),
	}
	violations := VerifyChainDetailed(log)
	if len(violations) != 1 || violations[0].Kind != KindMissingParent {
		t.Fatalf("expected one missing-parent finding, got %+v", violations)
	}
	if !violations[0].Kind.EvidencesAlteration() {
		t.Fatal("a removed entry must count as evidence of alteration")
	}
}

func TestViolationKindClassification(t *testing.T) {
	tests := []struct {
		kind        ViolationKind
		altered     bool
		unexplained bool
	}{
		{KindHashMismatch, true, false},
		{KindMissingParent, true, false},
		{KindDuplicate, false, true},
		{KindUnhashed, false, false},
		{KindUnknownAlgo, false, false},
		{KindLegacyUnverifiable, false, false},
		{KindRemovedSinceBaseline, true, false},
	}
	for _, tc := range tests {
		if got := tc.kind.EvidencesAlteration(); got != tc.altered {
			t.Errorf("kind %d: EvidencesAlteration = %v, want %v", tc.kind, got, tc.altered)
		}
		if got := tc.kind.Unexplained(); got != tc.unexplained {
			t.Errorf("kind %d: Unexplained = %v, want %v", tc.kind, got, tc.unexplained)
		}
	}
}

// Truncating the tail left the chain internally consistent: nothing references
// the newest entries, so VerifyChainDetailed has nothing to report. Only a
// committed baseline can show they are gone.
func TestMissingFromBaselineCatchesTruncatedTail(t *testing.T) {
	committed := []ChainEntry{entry("a", "h1", ""), entry("b", "h2", "h1"), entry("c", "h3", "h2")}
	truncated := committed[:1]

	if v := VerifyChainDetailed(truncated); len(v) != 0 {
		t.Fatalf("precondition: the truncated chain should verify on its own, got %+v", v)
	}
	missing := MissingFromBaseline(committed, truncated, "HEAD")
	if len(missing) != 2 {
		t.Fatalf("expected the two removed entries, got %+v", missing)
	}
	for _, v := range missing {
		if v.Kind != KindRemovedSinceBaseline || !v.Kind.EvidencesAlteration() {
			t.Errorf("unexpected finding %+v", v)
		}
	}
}

// A working copy that has grown since the baseline, by appends or a merged
// branch, is not a finding.
func TestMissingFromBaselineAllowsGrowth(t *testing.T) {
	committed := []ChainEntry{entry("a", "h1", ""), entry("b", "h2", "h1")}
	grown := append(append([]ChainEntry{}, committed...), entry("c", "h3", "h2"), entry("d", "h4", "h1"))
	if v := MissingFromBaseline(committed, grown, "HEAD"); len(v) != 0 {
		t.Fatalf("growth reported as removal: %+v", v)
	}
}

func TestMissingFromBaselineMatchesUnhashedByID(t *testing.T) {
	committed := []ChainEntry{{ID: "legacy-1"}, entry("a", "h1", "")}
	if v := MissingFromBaseline(committed, []ChainEntry{entry("a", "h1", "")}, "origin/main"); len(v) != 1 || v[0].ID != "legacy-1" {
		t.Fatalf("expected the unhashed entry to be reported by ID, got %+v", v)
	}
}
