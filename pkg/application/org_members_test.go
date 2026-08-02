package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/org"
)

// initRoadyProject creates a directory carrying a .roady/ directory.
func initRoadyProject(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(path, ".roady"), 0o750); err != nil {
		t.Fatal(err)
	}
}

func writeOrgConfig(t *testing.T, root string, cfg *org.OrgConfig) {
	t.Helper()
	if err := application.NewOrgService(root).SaveOrgConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

// org.yaml has carried a repos: list since the type was introduced, and
// nothing ever read it — a workspace could declare its members and Roady
// would quietly walk the tree instead, missing any repo outside the root and
// silently including any repo inside it.
func TestResolveMembersHonoursDeclaredRepos(t *testing.T) {
	root := t.TempDir()
	initRoadyProject(t, filepath.Join(root, "api"))
	initRoadyProject(t, filepath.Join(root, "web"))
	// Present on disk but not declared: must not appear.
	initRoadyProject(t, filepath.Join(root, "scratch"))

	writeOrgConfig(t, root, &org.OrgConfig{Name: "acme", Repos: []string{"./api", "./web"}})

	set, err := application.NewOrgService(root).ResolveMembers()
	if err != nil {
		t.Fatal(err)
	}

	if !set.IsDeclared() {
		t.Error("membership from org.yaml is not marked as declared")
	}
	if got := set.Names(); len(got) != 2 || got[0] != "api" || got[1] != "web" {
		t.Errorf("members = %v, want [api web]", got)
	}
	for _, m := range set.Usable() {
		if m.RoadyDir != filepath.Join(m.Path, ".roady") {
			t.Errorf("member %s resolved RoadyDir to %q", m.Name(), m.RoadyDir)
		}
	}
}

// A declared member outside the root is the reason to declare members at all.
func TestResolveMembersReachesOutsideTheRoot(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	sibling := filepath.Join(base, "shared-lib")
	initRoadyProject(t, root)
	initRoadyProject(t, sibling)

	writeOrgConfig(t, root, &org.OrgConfig{Name: "acme", Repos: []string{"../shared-lib"}})

	set, err := application.NewOrgService(root).ResolveMembers()
	if err != nil {
		t.Fatal(err)
	}

	usable := set.Usable()
	if len(usable) != 1 {
		t.Fatalf("got %d usable members, want 1 (problems: %v)", len(usable), set.Problems())
	}
	if usable[0].Name() != "shared-lib" {
		t.Errorf("member = %q, want shared-lib", usable[0].Name())
	}
}

func TestResolveMembersAcceptsAbsolutePaths(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	other := filepath.Join(base, "other")
	initRoadyProject(t, root)
	initRoadyProject(t, other)

	writeOrgConfig(t, root, &org.OrgConfig{Name: "acme", Repos: []string{other}})

	set, err := application.NewOrgService(root).ResolveMembers()
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Usable()) != 1 {
		t.Errorf("absolute member path was not resolved: %v", set.Problems())
	}
}

// A member that is missing, or is a directory with no Roady project in it,
// must be named rather than dropped.
func TestResolveMembersReportsUnusableMembers(t *testing.T) {
	root := t.TempDir()
	initRoadyProject(t, filepath.Join(root, "api"))
	if err := os.MkdirAll(filepath.Join(root, "not-a-project"), 0o750); err != nil {
		t.Fatal(err)
	}

	writeOrgConfig(t, root, &org.OrgConfig{
		Name:  "acme",
		Repos: []string{"./api", "./gone", "./not-a-project"},
	})

	set, err := application.NewOrgService(root).ResolveMembers()
	if err != nil {
		t.Fatal(err)
	}

	if len(set.Usable()) != 1 {
		t.Errorf("got %d usable members, want 1", len(set.Usable()))
	}

	problems := strings.Join(set.Problems(), "\n")
	if !strings.Contains(problems, "./gone") {
		t.Errorf("a missing member was not reported: %q", problems)
	}
	if !strings.Contains(problems, "./not-a-project") {
		t.Errorf("a directory with no Roady project was not reported: %q", problems)
	}
	if !strings.Contains(problems, "roady init") {
		t.Errorf("the report does not say how to fix an uninitialised member: %q", problems)
	}
}

// With no org.yaml, or one that declares no repos, discovery still applies —
// declaring members is an option, not a requirement.
func TestResolveMembersFallsBackToDiscovery(t *testing.T) {
	root := t.TempDir()
	initRoadyProject(t, root)
	initRoadyProject(t, filepath.Join(root, "api"))
	initRoadyProject(t, filepath.Join(root, "web"))

	set, err := application.NewOrgService(root).ResolveMembers()
	if err != nil {
		t.Fatal(err)
	}

	if set.IsDeclared() {
		t.Error("discovered membership is marked as declared")
	}
	if len(set.Usable()) < 2 {
		t.Errorf("discovery found %d members, want at least 2", len(set.Usable()))
	}
	if set.HasProblems() {
		t.Errorf("discovery reported problems: %v", set.Problems())
	}
}

func TestResolveMembersEmptyReposListFallsBackToDiscovery(t *testing.T) {
	root := t.TempDir()
	initRoadyProject(t, filepath.Join(root, "api"))
	writeOrgConfig(t, root, &org.OrgConfig{Name: "acme"})

	set, err := application.NewOrgService(root).ResolveMembers()
	if err != nil {
		t.Fatal(err)
	}
	if set.IsDeclared() {
		t.Error("an org.yaml with no repos should not produce a declared set")
	}
}

// Aggregation must cover exactly the declared membership. Walking the tree
// instead means a sibling repository is invisible to org status and org
// drift, and a scratch checkout inside the workspace silently counts towards
// the numbers leadership reads.
func TestAggregateMetricsCoversDeclaredMembersOnly(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	sibling := filepath.Join(base, "shared-lib")
	for _, p := range []string{root, filepath.Join(root, "api"), filepath.Join(root, "scratch"), sibling} {
		initRoadyProject(t, p)
	}

	writeOrgConfig(t, root, &org.OrgConfig{
		Name:  "acme",
		Repos: []string{"./api", "../shared-lib"},
	})

	metrics, err := application.NewOrgService(root).AggregateMetrics()
	if err != nil {
		t.Fatal(err)
	}

	names := map[string]bool{}
	for _, p := range metrics.Projects {
		names[filepath.Base(p.Path)] = true
	}

	if !names["api"] {
		t.Error("declared member api is missing from the aggregate")
	}
	if !names["shared-lib"] {
		t.Error("declared member outside the root is missing from the aggregate")
	}
	if names["scratch"] {
		t.Error("an undeclared repository inside the workspace was counted")
	}
	if metrics.TotalProjects != 2 {
		t.Errorf("TotalProjects = %d, want 2", metrics.TotalProjects)
	}
}

func TestDetectCrossDriftCoversDeclaredMembersOnly(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	sibling := filepath.Join(base, "shared-lib")
	for _, p := range []string{root, filepath.Join(root, "api"), filepath.Join(root, "scratch"), sibling} {
		initRoadyProject(t, p)
	}

	writeOrgConfig(t, root, &org.OrgConfig{
		Name:  "acme",
		Repos: []string{"./api", "../shared-lib"},
	})

	report, err := application.NewOrgService(root).DetectCrossDrift()
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Projects) != 2 {
		t.Errorf("covered %d projects, want the 2 declared members", len(report.Projects))
	}
	for _, p := range report.Projects {
		if filepath.Base(p.Path) == "scratch" {
			t.Error("an undeclared repository was included in cross-repo drift")
		}
	}
}

// Aggregation with no org.yaml keeps walking the tree.
func TestAggregateMetricsFallsBackToDiscovery(t *testing.T) {
	root := t.TempDir()
	initRoadyProject(t, filepath.Join(root, "api"))
	initRoadyProject(t, filepath.Join(root, "web"))

	metrics, err := application.NewOrgService(root).AggregateMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if metrics.TotalProjects != 2 {
		t.Errorf("TotalProjects = %d, want 2 from discovery", metrics.TotalProjects)
	}
}
