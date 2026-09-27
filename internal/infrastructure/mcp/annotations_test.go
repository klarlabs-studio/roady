package mcp

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// registeredToolNames scrapes the registration source rather than starting a
// server, so the check works without a project on disk and cannot be fooled
// by a tool that registers conditionally.
func registeredToolNames(t *testing.T) []string {
	t.Helper()

	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}

	re := regexp.MustCompile(`s\.tool\("([a-z_]+)"\)`)
	seen := map[string]bool{}
	var names []string
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}

	if len(names) == 0 {
		t.Fatal("found no tool registrations; the scrape pattern is stale")
	}
	sort.Strings(names)
	return names
}

// TestEveryToolIsAnnotated is the regression guard. An unannotated tool
// inherits the spec's pessimistic defaults — not read-only, potentially
// destructive — so a new read tool added without a classification quietly
// tells every client that reading might destroy something.
func TestEveryToolIsAnnotated(t *testing.T) {
	var missing []string
	for _, name := range registeredToolNames(t) {
		if _, ok := toolBehaviours[name]; !ok {
			missing = append(missing, name)
		}
	}

	if len(missing) > 0 {
		t.Errorf("these tools have no behaviour classification; add them to toolBehaviours in annotations.go:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// TestNoStaleAnnotations catches the opposite drift: a classification left
// behind after its tool was renamed or removed, which silently stops
// applying to anything.
func TestNoStaleAnnotations(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range registeredToolNames(t) {
		registered[name] = true
	}

	var stale []string
	for name := range toolBehaviours {
		if !registered[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)

	if len(stale) > 0 {
		t.Errorf("these classifications reference tools that are no longer registered:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// TestReadOnlyToolsAreNotDestructive asserts the combination that would be
// incoherent: a tool cannot both be unable to change state and be capable of
// an irreversible change.
func TestReadOnlyToolsAreNotDestructive(t *testing.T) {
	for name, b := range toolBehaviours {
		if b.readOnly && b.destructive {
			t.Errorf("%s is marked both read-only and destructive", name)
		}
	}
}
