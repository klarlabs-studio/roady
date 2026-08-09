package mcp

import (
	"sync"

	"go.klarlabs.de/mcp"
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
	"roady_get_spec":              {readOnly: true, idempotent: true},
	"roady_get_plan":              {readOnly: true, idempotent: true},
	"roady_get_state":             {readOnly: true, idempotent: true},
	"roady_get_snapshot":          {readOnly: true, idempotent: true},
	"roady_get_usage":             {readOnly: true, idempotent: true},
	"roady_status":                {readOnly: true, idempotent: true},
	"roady_tasks":                 {readOnly: true, idempotent: true},
	"roady_org_members":           {readOnly: true, idempotent: true},
	"roady_report":                {readOnly: true, idempotent: true},
	"roady_spec_analyze":          {},
	"roady_semantic_drift":        {readOnly: true, idempotent: true},
	"roady_record_semantic_drift": {},
	"roady_plan_prune":            {destructive: true},
	"roady_plan_reject":           {idempotent: true},
	"roady_audit_verify":          {readOnly: true, idempotent: true},
	"roady_spec_validate":         {readOnly: true, idempotent: true},
	"roady_spec_import":           {destructive: true},
	"roady_spec_lock":             {idempotent: true},
	"roady_state_rebuild":         {destructive: true},
	"roady_timeline":              {readOnly: true, idempotent: true},
	"roady_debt_history":          {readOnly: true, idempotent: true},
	"roady_debt_score":            {readOnly: true, idempotent: true},
	"roady_check_policy":          {readOnly: true, idempotent: true},
	"roady_detect_drift":          {readOnly: true, idempotent: true},
	"roady_debt_report":           {readOnly: true, idempotent: true},
	"roady_debt_summary":          {readOnly: true, idempotent: true},
	"roady_debt_trend":            {readOnly: true, idempotent: true},
	"roady_drift_recurring":       {readOnly: true, idempotent: true},
	"roady_audit_trail":           {readOnly: true, idempotent: true},
	"roady_deps_list":             {readOnly: true, idempotent: true},
	"roady_deps_graph":            {readOnly: true, idempotent: true},
	"roady_forecast":              {readOnly: true, idempotent: true},
	"roady_cost_report":           {readOnly: true, idempotent: true},
	"roady_cost_budget":           {readOnly: true, idempotent: true},
	"roady_rate_list":             {readOnly: true, idempotent: true},
	"roady_team_list":             {readOnly: true, idempotent: true},
	"roady_messaging_list":        {readOnly: true, idempotent: true},
	"roady_plugin_list":           {readOnly: true, idempotent: true},
	"roady_org_status":            {readOnly: true, idempotent: true},
	"roady_org_policy":            {readOnly: true, idempotent: true},
	"roady_org_detect_drift":      {readOnly: true, idempotent: true},

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
	// A tool whose group is switched off is never registered, so it costs a
	// client nothing in its prompt. s.groups is nil in tests that construct a
	// Server directly; treat that as "everything", so no existing test has to
	// know about profiles to keep passing.
	if s.groups != nil {
		if g, ok := toolGroups[name]; ok && !s.groups[g] {
			return discardedTool()
		}
	}

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

// discardedTool returns a builder whose registration goes nowhere, so a
// disabled tool's `s.tool(...).Description(...).Handler(...)` chain in
// registerTools stays valid without every call site growing a conditional.
//
// The alternative — an `if enabled { ... }` around each of seventy
// registrations — is the kind of thing that gets forgotten on tool
// seventy-one, and a forgotten check here means a tool that ignores the
// profile. Keeping the gate at the single chokepoint is why
// TestEveryToolHasAGroup can guarantee anything.
func discardedTool() *mcpserver.ToolBuilder {
	discardOnce.Do(func() {
		discardServer = mcp.NewServer(mcp.ServerInfo{Name: "discard", Version: "0"})
	})
	return discardServer.Tool("discarded")
}

var (
	discardOnce   sync.Once
	discardServer *mcp.Server
)
