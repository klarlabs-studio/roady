package cli

import (
	"os"
	"strings"
	"testing"
)

// mcp-go keeps its roadmap by phase. Import finds no horizon, and says which
// sections it did find instead of only that it found nothing.
func TestGoalImportNamesTheSectionsItFound(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	if _, err := runRoady(t, "", "init", "phases"); err != nil {
		t.Fatal(err)
	}
	roadmap := "# Roadmap\n\n## Phase 0 — Foundation\n\n- Negotiation layer\n\n## Phase 1 — Certify 2025-03-26\n\n- Streamable HTTP\n"
	if err := os.WriteFile("roadmap.md", []byte(roadmap), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runRoady(t, "", "goal", "import", "roadmap.md", "--dry-run")
	if err == nil {
		t.Fatal("a phase roadmap imported goals")
	}
	for _, want := range []string{"Phase 0 — Foundation", "Phase 1 — Certify 2025-03-26", "are not horizons", "## Now, Next, Later", "roady goal add"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

// The same phase roadmap imports once the phases are mapped.
func TestGoalImportSectionMapThroughTheCLI(t *testing.T) {
	_, cleanup := withPlainTempDir(t)
	defer cleanup()
	if _, err := runRoady(t, "", "init", "phases"); err != nil {
		t.Fatal(err)
	}
	roadmap := "# Roadmap\n\n## Phase 0 — Foundation (v1.22.0)\n\nNegotiation first.\n\n- [x] Negotiation layer\n\n## Phase 1 — Certify 2025-03-26 (v1.23.0)\n\n- [x] Streamable HTTP\n"
	if err := os.WriteFile("roadmap.md", []byte(roadmap), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runRoady(t, "", "goal", "import", "roadmap.md", "--section-goal", "Phase=shipped")
	if err != nil || !strings.Contains(out, "Imported 2 goal(s)") {
		t.Fatalf("import: %v\n%s", err, out)
	}
	out, _ = runRoady(t, "", "goal", "list")
	for _, want := range []string{"Phase 0 — Foundation  (v1.22.0)", "Phase 1 — Certify 2025-03-26  (v1.23.0)"} {
		if !strings.Contains(out, want) {
			t.Errorf("goal list lacks %q:\n%s", want, out)
		}
	}
}
