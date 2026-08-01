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

// TestAIToolsAreNotReadOnly pins the judgement call that is easiest to get
// wrong on a later edit. Roady's AI tools return text and look like reads,
// but each records token usage to the audit log — the spec is explicit that
// a read which also logs is not read-only.
func TestAIToolsAreNotReadOnly(t *testing.T) {
	aiTools := []string{
		"roady_explain_spec",
		"roady_review_spec",
		"roady_explain_drift",
		"roady_query",
		"roady_suggest_priorities",
		"roady_plan_decompose",
	}

	for _, name := range aiTools {
		b, ok := toolBehaviours[name]
		if !ok {
			t.Errorf("%s is unclassified", name)
			continue
		}
		if b.readOnly {
			t.Errorf("%s must not be read-only: it records token usage to the audit log", name)
		}
		if !b.openWorld {
			t.Errorf("%s must be open-world: it calls an external model provider", name)
		}
	}
}

// TestStateChangingToolsAreClassified guards the tools whose misclassification
// would be most costly to a user: anything that overwrites a plan or replaces
// the drift baseline must be marked destructive so clients prompt first.
func TestStateChangingToolsAreClassified(t *testing.T) {
	mustBeDestructive := []string{
		"roady_generate_plan",
		"roady_update_plan",
		"roady_plan_decompose",
		"roady_accept_drift",
		"roady_rate_remove",
		"roady_team_remove",
		"roady_workspace_pull",
	}

	for _, name := range mustBeDestructive {
		b, ok := toolBehaviours[name]
		if !ok {
			t.Errorf("%s is unclassified", name)
			continue
		}
		if !b.destructive {
			t.Errorf("%s overwrites or deletes state and must be marked destructive", name)
		}
	}
}

// TestExternalToolsAreOpenWorld checks that anything leaving the repository
// says so, since openWorldHint is what tells a client the blast radius is not
// bounded by the working directory.
func TestExternalToolsAreOpenWorld(t *testing.T) {
	external := []string{
		"roady_sync",
		"roady_git_sync",
		"roady_workspace_push",
		"roady_workspace_pull",
		"roady_plugin_status",
		"roady_plugin_validate",
	}

	for _, name := range external {
		b, ok := toolBehaviours[name]
		if !ok {
			t.Errorf("%s is unclassified", name)
			continue
		}
		if !b.openWorld {
			t.Errorf("%s reaches outside the repository and must be open-world", name)
		}
	}
}
