package mcp

import (
	"fmt"
	"sort"
	"strings"
)

// toolGroup names a family of tools that a given project either uses or never
// touches. Every registered tool belongs to exactly one.
//
// The point is context, not access control. Every tool is advertised
// to every client on every session, and a client pays for each one in its
// prompt whether or not the project has a rate card or a debt ledger. The
// report that prompted this counted five tools actually used across a very
// long session (#87, item 2).
type toolGroup string

const (
	// groupCore is the spec → plan → execute loop, plus the reads and the
	// drift and policy checks that go with it. This is what an agent working
	// a task list uses.
	groupCore toolGroup = "core"

	groupAnalytics toolGroup = "analytics" // semantic drift judgement
	groupAudit     toolGroup = "audit"     // hash-chained event log
)

// allGroups is the enumeration used to validate configuration and to build
// the "all" profile. Order is the order shown in error messages.
var allGroups = []toolGroup{groupCore, groupAnalytics, groupAudit}

// toolGroups assigns every tool to a group.
//
// A tool missing from this map is a build-time failure via
// TestEveryToolHasAGroup, the same guarantee toolBehaviours has. That matters
// more here than it looks: an unclassified tool would silently vanish from
// every profile, and a tool that is never advertised is indistinguishable
// from a tool that does not exist.
var toolGroups = map[string]toolGroup{
	// --- core: the spec → plan → execute loop ----------------------------
	"roady_init":            groupCore,
	"roady_status":          groupCore,
	"roady_query":           groupCore,
	"roady_snapshot_get":    groupCore,
	"roady_spec_get":        groupCore,
	"roady_plan_get":        groupCore,
	"roady_state_get":       groupCore,
	"roady_spec_add":        groupCore,
	"roady_spec_explain":    groupCore,
	"roady_spec_review":     groupCore,
	"roady_spec_validate":   groupCore,
	"roady_spec_lock":       groupCore,
	"roady_spec_analyze":    groupCore,
	"roady_spec_import":     groupCore,
	"roady_plan_generate":   groupCore,
	"roady_plan_update":     groupCore,
	"roady_plan_approve":    groupCore,
	"roady_plan_reject":     groupCore,
	"roady_plan_prune":      groupCore,
	"roady_plan_decompose":  groupCore,
	"roady_tasks":           groupCore,
	"roady_task_transition": groupCore,
	"roady_task_check":      groupCore,
	"roady_capture":         groupCore,
	"roady_plan_import":     groupCore,
	"roady_next":            groupCore,
	"roady_task_dispatch":   groupCore,
	"roady_drift_detect":    groupCore,
	"roady_drift_accept":    groupCore,
	"roady_drift_explain":   groupCore,
	"roady_policy_check":    groupCore,
	"roady_state_rebuild":   groupCore,
	"roady_plan_prioritize": groupCore,

	// --- sync: leaves the repository ------------------------------------------
	"roady_git_sync": groupCore,

	// --- analytics -------------------------------------------------------------
	"roady_semantic_drift":        groupAnalytics,
	"roady_drift_record_semantic": groupAnalytics,

	// --- audit -------------------------------------------------------------------
	"roady_audit_trail":  groupAudit,
	"roady_audit_verify": groupAudit,
}

// essentialTools are what an agent needs to work a plan: what to do next,
// recording intent of any size (or a plan it already wrote), moving a task
// through its lifecycle, proving it done, and reading the project. About
// 2.7k tokens of tools/list, against ~10k for all 38.
//
// It is the default advertised surface. Every other tool stays registered
// and callable — the SDK and any client that knows a tool's name keep
// working, and every CLI operation stays reachable — it is just not listed,
// so an agent does not pay for every tool in its prompt to use seven.
var essentialTools = map[string]bool{
	"roady_next":            true,
	"roady_capture":         true,
	"roady_plan_import":     true,
	"roady_task_transition": true,
	"roady_task_check":      true,
	"roady_status":          true,
	"roady_query":           true,
}

// profileEssential names the default surface in ROADY_MCP_TOOLS.
const profileEssential = "essential"

// toolSurface is what a server registers and what it lists.
type toolSurface struct {
	groups map[toolGroup]bool // registered groups
	// listed decides which registered tools tools/list shows; nil lists all.
	listed func(name string) bool
}

// resolveSurface reads ROADY_MCP_TOOLS:
//
//   - unset or "essential": everything registered, the essential tools listed
//   - "essential,<group>...": the same, plus those groups listed
//   - "all": everything registered and listed
//   - "<group>,...": only those groups (and core) registered and listed
func resolveSurface(profile string) (toolSurface, error) {
	var extra []string
	essential := strings.TrimSpace(profile) == ""
	for _, raw := range strings.Split(profile, ",") {
		switch name := strings.TrimSpace(raw); name {
		case "":
		case profileEssential:
			essential = true
		default:
			extra = append(extra, name)
		}
	}
	if !essential {
		groups, err := enabledGroups(profile)
		return toolSurface{groups: groups}, err
	}
	all, _ := enabledGroups("all")
	listedGroups := map[toolGroup]bool{}
	if len(extra) > 0 {
		g, err := enabledGroups(strings.Join(extra, ","))
		if err != nil {
			return toolSurface{}, err
		}
		if len(extra) == 1 && extra[0] == "all" {
			return toolSurface{groups: all}, nil
		}
		for name := range g {
			// enabledGroups always adds core; list it only when asked for.
			if name != groupCore || containsTrimmed(extra, string(groupCore)) {
				listedGroups[name] = true
			}
		}
	}
	return toolSurface{groups: all, listed: func(name string) bool {
		return essentialTools[name] || listedGroups[toolGroups[name]]
	}}, nil
}

func containsTrimmed(list []string, want string) bool {
	for _, s := range list {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}

// enabledGroups resolves a profile string into the set of groups to register.
//
// The zero value enables everything. (An unset ROADY_MCP_TOOLS registers
// everything too, but lists only the essential tools: see resolveSurface.)
//
// Accepted: "all" (default), or a comma-separated list of group names. "core"
// is always included, because a profile without the spec/plan/execute loop
// leaves a server that cannot do the thing roady is for.
func enabledGroups(profile string) (map[toolGroup]bool, error) {
	profile = strings.TrimSpace(profile)
	if profile == "" || profile == "all" {
		out := make(map[toolGroup]bool, len(allGroups))
		for _, g := range allGroups {
			out[g] = true
		}
		return out, nil
	}

	valid := make(map[toolGroup]bool, len(allGroups))
	for _, g := range allGroups {
		valid[g] = true
	}

	out := map[toolGroup]bool{groupCore: true}
	var unknown []string
	for _, raw := range strings.Split(profile, ",") {
		name := toolGroup(strings.TrimSpace(raw))
		if name == "" {
			continue
		}
		if !valid[name] {
			unknown = append(unknown, string(name))
			continue
		}
		out[name] = true
	}

	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf(
			"unknown tool group(s) %s; valid groups are %s (or \"all\")",
			strings.Join(unknown, ", "), groupNames())
	}

	return out, nil
}

// groupNames renders the valid group list for error messages.
func groupNames() string {
	names := make([]string, 0, len(allGroups))
	for _, g := range allGroups {
		names = append(names, string(g))
	}
	return strings.Join(names, ", ")
}
