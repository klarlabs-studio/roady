package mcp

import (
	"sort"
	"strings"
	"testing"
)

// Every tool must have a group, for the same reason every tool must have a
// behaviour annotation: an unclassified tool would be silently absent from
// every profile, and a tool that is never advertised is indistinguishable
// from one that does not exist.
func TestEveryToolHasAGroup(t *testing.T) {
	var missing []string
	for _, name := range registeredToolNames(t) {
		if _, ok := toolGroups[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)

	if len(missing) > 0 {
		t.Errorf("these tools have no group; add them to toolGroups in profiles.go:\n  %s",
			strings.Join(missing, "\n  "))
	}
}

// The opposite drift: a group assignment left behind after its tool was
// renamed or removed, which quietly stops applying to anything.
func TestNoStaleGroups(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range registeredToolNames(t) {
		registered[name] = true
	}

	var stale []string
	for name := range toolGroups {
		if !registered[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)

	if len(stale) > 0 {
		t.Errorf("these group assignments match no registered tool; remove them from profiles.go:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

func TestEnabledGroups(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    []toolGroup
		absent  []toolGroup
		wantErr string
	}{
		{
			name:    "unset enables everything",
			profile: "",
			want:    allGroups,
		},
		{
			name:    "all enables everything",
			profile: "all",
			want:    allGroups,
		},
		{
			name:    "a single group still includes core",
			profile: "debt",
			want:    []toolGroup{groupCore, groupDebt},
			absent:  []toolGroup{groupCost, groupOrg, groupSync},
		},
		{
			name:    "core alone is the minimum surface",
			profile: "core",
			want:    []toolGroup{groupCore},
			absent:  []toolGroup{groupCost, groupTeam, groupOrg, groupDebt, groupDeps, groupPlugin, groupSync, groupAnalytics, groupAudit},
		},
		{
			name:    "comma separated, whitespace tolerated",
			profile: " cost , team ",
			want:    []toolGroup{groupCore, groupCost, groupTeam},
			absent:  []toolGroup{groupDebt},
		},
		{
			// A typo must fail loudly. Silently ignoring it would start a
			// server with a smaller surface than asked for, and the operator
			// would discover it as a missing tool at call time.
			name:    "an unknown group is an error, not a shrug",
			profile: "core,dept",
			wantErr: "unknown tool group(s) dept",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := enabledGroups(tc.profile)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("enabledGroups(%q) = nil error, want %q", tc.profile, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
				}
				// The message must name the valid options, or the operator
				// has to read the source to recover from a typo.
				if !strings.Contains(err.Error(), "core") {
					t.Errorf("error %q does not list the valid groups", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("enabledGroups(%q): unexpected error %v", tc.profile, err)
			}
			for _, g := range tc.want {
				if !got[g] {
					t.Errorf("group %q should be enabled by profile %q", g, tc.profile)
				}
			}
			for _, g := range tc.absent {
				if got[g] {
					t.Errorf("group %q should NOT be enabled by profile %q", g, tc.profile)
				}
			}
		})
	}
}

// The saving has to be real, or the feature is decoration. core must be a
// substantial reduction and must still contain the tools the loop needs.
func TestCoreProfileIsASubstantialReduction(t *testing.T) {
	total := len(registeredToolNames(t))

	core := 0
	for _, g := range toolGroups {
		if g == groupCore {
			core++
		}
	}

	if core >= total {
		t.Fatalf("core (%d) is not smaller than the full surface (%d)", core, total)
	}
	if pct := 100 * core / total; pct > 60 {
		t.Errorf("core is %d%% of the full surface (%d of %d); that is not a meaningful reduction",
			pct, core, total)
	}

	// The loop roady exists for must survive the trim. These are the tools
	// the report in #87 named as the ones actually reached for.
	essential := []string{
		"roady_status", "roady_tasks", "roady_spec_get",
		"roady_spec_add", "roady_spec_validate", "roady_task_transition",
	}
	for _, name := range essential {
		if toolGroups[name] != groupCore {
			t.Errorf("%s must be in core; it is in %q", name, toolGroups[name])
		}
	}
}

// The unit tests above check the mapping; this one checks the wiring. It
// builds real servers and asks each what it actually advertises, because a
// correct group table that never reaches registration would pass every other
// test in this file and save a client nothing.
func TestProfileChangesTheAdvertisedSurface(t *testing.T) {
	root := t.TempDir()

	build := func(profile string) []string {
		t.Helper()
		t.Setenv("ROADY_MCP_TOOLS", profile)
		s, err := NewServer(root)
		if err != nil {
			t.Fatalf("NewServer(profile=%q): %v", profile, err)
		}
		var names []string
		for _, info := range s.mcpServer.Tools() {
			if strings.HasPrefix(info.Name, "roady_") {
				names = append(names, info.Name)
			}
		}
		sort.Strings(names)
		return names
	}

	all := build("all")
	core := build("core")

	if len(all) == 0 {
		t.Fatal("the full profile advertised no tools; the harness is wrong, not the code")
	}
	if len(core) >= len(all) {
		t.Fatalf("core advertised %d tools, all advertised %d — the profile did not take effect",
			len(core), len(all))
	}

	// Every core tool must be a real subset of the full surface.
	inAll := map[string]bool{}
	for _, n := range all {
		inAll[n] = true
	}
	for _, n := range core {
		if !inAll[n] {
			t.Errorf("core advertised %q, which the full profile does not", n)
		}
	}

	// And the finance/admin families the report singled out must be gone.
	for _, n := range core {
		for _, banned := range []string{"roady_rate_", "roady_cost_", "roady_team_", "roady_debt_"} {
			if strings.HasPrefix(n, banned) {
				t.Errorf("core still advertises %q", n)
			}
		}
	}

	t.Logf("all=%d core=%d (%d fewer tools in every prompt)", len(all), len(core), len(all)-len(core))
}

// A typo must stop the server, not start a quietly diminished one.
func TestBadProfileFailsStartup(t *testing.T) {
	t.Setenv("ROADY_MCP_TOOLS", "core,dept")
	if _, err := NewServer(t.TempDir()); err == nil {
		t.Fatal("NewServer accepted an unknown tool group; it must fail loudly")
	} else if !strings.Contains(err.Error(), "dept") {
		t.Errorf("error %q does not name the offending group", err)
	}
}
