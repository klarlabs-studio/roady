package mcp

import (
	"fmt"
	"sort"
	"strings"
)

// toolGroup names a family of tools that a given project either uses or never
// touches. Every registered tool belongs to exactly one.
//
// The point is context, not access control. All seventy tools are advertised
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

	groupCost      toolGroup = "cost"      // budgets, rate cards, tax
	groupTeam      toolGroup = "team"      // roster and assignment
	groupOrg       toolGroup = "org"       // cross-repo org rollups
	groupDebt      toolGroup = "debt"      // technical-debt ledger
	groupDeps      toolGroup = "deps"      // dependency graph and scanning
	groupPlugin    toolGroup = "plugin"    // external syncer plugins
	groupSync      toolGroup = "sync"      // anything that leaves the repo
	groupAnalytics toolGroup = "analytics" // forecasting, reporting, trends
	groupAudit     toolGroup = "audit"     // hash-chained event log
)

// allGroups is the enumeration used to validate configuration and to build
// the "all" profile. Order is the order shown in error messages.
var allGroups = []toolGroup{
	groupCore, groupCost, groupTeam, groupOrg, groupDebt,
	groupDeps, groupPlugin, groupSync, groupAnalytics, groupAudit,
}

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
	"roady_task_dispatch":   groupCore,
	"roady_task_log_time":   groupCore,
	"roady_drift_detect":    groupCore,
	"roady_drift_accept":    groupCore,
	"roady_drift_explain":   groupCore,
	"roady_policy_check":    groupCore,
	"roady_state_rebuild":   groupCore,
	"roady_plan_prioritize": groupCore,

	// --- cost: finance surface -------------------------------------------
	"roady_cost_budget":      groupCost,
	"roady_cost_report":      groupCost,
	"roady_rate_add":         groupCost,
	"roady_rate_list":        groupCost,
	"roady_rate_remove":      groupCost,
	"roady_rate_set_default": groupCost,
	"roady_rate_tax":         groupCost,
	"roady_usage_get":        groupCost,

	// --- team -------------------------------------------------------------
	"roady_team_add":    groupTeam,
	"roady_team_list":   groupTeam,
	"roady_team_remove": groupTeam,
	"roady_task_assign": groupTeam,

	// --- org ---------------------------------------------------------------
	"roady_org_status":       groupOrg,
	"roady_org_members":      groupOrg,
	"roady_org_policy":       groupOrg,
	"roady_org_detect_drift": groupOrg,

	// --- debt ---------------------------------------------------------------
	"roady_debt_report":  groupDebt,
	"roady_debt_summary": groupDebt,
	"roady_debt_score":   groupDebt,
	"roady_debt_trend":   groupDebt,
	"roady_debt_history": groupDebt,

	// --- deps ----------------------------------------------------------------
	"roady_deps_graph": groupDeps,
	"roady_deps_list":  groupDeps,
	"roady_deps_scan":  groupDeps,

	// --- plugin ---------------------------------------------------------------
	"roady_plugin_list":     groupPlugin,
	"roady_plugin_status":   groupPlugin,
	"roady_plugin_validate": groupPlugin,

	// --- sync: leaves the repository ------------------------------------------
	"roady_sync":           groupSync,
	"roady_git_sync":       groupSync,
	"roady_workspace_push": groupSync,
	"roady_workspace_pull": groupSync,
	"roady_messaging_list": groupSync,

	// --- analytics -------------------------------------------------------------
	"roady_forecast":              groupAnalytics,
	"roady_report":                groupAnalytics,
	"roady_timeline":              groupAnalytics,
	"roady_drift_recurring":       groupAnalytics,
	"roady_semantic_drift":        groupAnalytics,
	"roady_drift_record_semantic": groupAnalytics,

	// --- audit -------------------------------------------------------------------
	"roady_audit_trail":  groupAudit,
	"roady_audit_verify": groupAudit,
}

// enabledGroups resolves a profile string into the set of groups to register.
//
// The zero value — an unset ROADY_MCP_TOOLS — enables everything. Trimming the
// surface is opt-in on purpose: a server that silently stopped advertising
// tools an existing client already calls would turn a context saving into a
// broken integration.
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
