package mcp

import (
	mcpserver "go.klarlabs.de/mcp/server"
)

// Tool behaviour hints, per the MCP tool-annotations spec.
//
// These are not cosmetic. Clients use them to decide whether to prompt before
// a call, and the spec's defaults are pessimistic: an unannotated tool is
// assumed *not* read-only and *potentially destructive*. Roady registers ~60
// tools, most of which only read local files, so shipping them unannotated
// told every client that reading a plan might destroy something — which
// trains users to click through confirmations that matter.
//
// Accuracy over convenience: readOnlyHint is only set where the tool cannot
// modify state at all. Roady's AI tools look like reads but record token
// usage to the audit log, so they are deliberately not marked read-only.
type toolBehaviour struct {
	// readOnly means the call cannot change any state, including telemetry.
	readOnly bool
	// destructive means an effect that is not easily undone: overwriting a
	// plan, replacing the drift baseline, deleting a record.
	destructive bool
	// idempotent means repeating the call with the same input lands in the
	// same place.
	idempotent bool
	// openWorld means the tool reaches beyond this repository — an external
	// tracker, a git remote, or an LLM provider.
	openWorld bool
}

// toolBehaviours classifies every registered tool. A tool missing from this
// map fails TestEveryToolIsAnnotated rather than silently inheriting the
// pessimistic defaults.
var toolBehaviours = map[string]toolBehaviour{
	// --- Pure local reads -------------------------------------------------
	"roady_get_spec":         {readOnly: true, idempotent: true},
	"roady_get_plan":         {readOnly: true, idempotent: true},
	"roady_get_state":        {readOnly: true, idempotent: true},
	"roady_get_snapshot":     {readOnly: true, idempotent: true},
	"roady_get_usage":        {readOnly: true, idempotent: true},
	"roady_status":           {readOnly: true, idempotent: true},
	"roady_tasks":            {readOnly: true, idempotent: true},
	"roady_org_members":      {readOnly: true, idempotent: true},
	"roady_report":           {readOnly: true, idempotent: true},
	"roady_spec_analyze":     {},
	"roady_check_policy":     {readOnly: true, idempotent: true},
	"roady_detect_drift":     {readOnly: true, idempotent: true},
	"roady_debt_report":      {readOnly: true, idempotent: true},
	"roady_debt_summary":     {readOnly: true, idempotent: true},
	"roady_debt_trend":       {readOnly: true, idempotent: true},
	"roady_drift_recurring":  {readOnly: true, idempotent: true},
	"roady_audit_trail":      {readOnly: true, idempotent: true},
	"roady_deps_list":        {readOnly: true, idempotent: true},
	"roady_deps_graph":       {readOnly: true, idempotent: true},
	"roady_forecast":         {readOnly: true, idempotent: true},
	"roady_cost_report":      {readOnly: true, idempotent: true},
	"roady_cost_budget":      {readOnly: true, idempotent: true},
	"roady_rate_list":        {readOnly: true, idempotent: true},
	"roady_team_list":        {readOnly: true, idempotent: true},
	"roady_messaging_list":   {readOnly: true, idempotent: true},
	"roady_plugin_list":      {readOnly: true, idempotent: true},
	"roady_org_status":       {readOnly: true, idempotent: true},
	"roady_org_policy":       {readOnly: true, idempotent: true},
	"roady_org_detect_drift": {readOnly: true, idempotent: true},

	// Reads that shell out to a plugin binary, so they leave the repo.
	"roady_plugin_status":   {readOnly: true, idempotent: true, openWorld: true},
	"roady_plugin_validate": {readOnly: true, idempotent: true, openWorld: true},

	// --- AI: look like reads, but record token usage to the audit log -----
	// Marking these read-only would be the exact mistake the spec warns
	// about, and would also hide their cost from clients that surface it.
	"roady_explain_spec":       {openWorld: true},
	"roady_review_spec":        {openWorld: true},
	"roady_explain_drift":      {openWorld: true},
	"roady_query":              {openWorld: true},
	"roady_suggest_priorities": {openWorld: true},

	// --- Additive writes: create or record, nothing lost ------------------
	"roady_init":             {idempotent: true},
	"roady_add_feature":      {},
	"roady_assign_task":      {idempotent: true},
	"roady_task_log_time":    {},
	"roady_rate_add":         {idempotent: true},
	"roady_team_add":         {idempotent: true},
	"roady_rate_set_default": {idempotent: true},
	"roady_rate_tax":         {idempotent: true},
	"roady_deps_scan":        {idempotent: true},

	// State moves that are reversible through the FSM.
	"roady_transition_task": {},
	// Claims the task by default, so not read-only; reversible via stop.
	"roady_dispatch_task": {},
	"roady_approve_plan":  {idempotent: true},

	// --- Destructive: overwrite or delete ---------------------------------
	// Plan generation replaces the existing task DAG, so unapproved local
	// edits are lost; drift acceptance replaces the intent baseline that
	// made the drift visible in the first place.
	"roady_generate_plan":  {destructive: true},
	"roady_update_plan":    {destructive: true},
	"roady_plan_decompose": {destructive: true, openWorld: true},
	"roady_accept_drift":   {destructive: true, idempotent: true},
	"roady_rate_remove":    {destructive: true, idempotent: true},
	"roady_team_remove":    {destructive: true, idempotent: true},

	// --- Reach outside the repository -------------------------------------
	"roady_sync":           {openWorld: true},
	"roady_git_sync":       {openWorld: true, idempotent: true},
	"roady_workspace_push": {openWorld: true},
	"roady_workspace_pull": {openWorld: true, destructive: true},
}

// tool starts a tool registration with its behaviour hints already applied,
// so no call site can register a tool without them.
func (s *Server) tool(name string) *mcpserver.ToolBuilder {
	b := s.mcpServer.Tool(name)

	behaviour, ok := toolBehaviours[name]
	if !ok {
		// Unclassified tools keep the spec's pessimistic defaults rather
		// than being quietly assumed safe. TestEveryToolIsAnnotated turns
		// this into a build-time failure instead of a runtime surprise.
		return b
	}

	if behaviour.readOnly {
		b = b.ReadOnly()
	}
	if behaviour.destructive {
		b = b.Destructive()
	}
	if behaviour.idempotent {
		b = b.Idempotent()
	}
	if behaviour.openWorld {
		b = b.OpenWorld()
	} else {
		b = b.ClosedWorld()
	}

	return b
}
