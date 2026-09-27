package mcp

import (
	mcpserver "go.klarlabs.de/mcp/server"
)

// Tool behaviour hints, per the MCP tool-annotations spec.
//
// These are not cosmetic. Clients use them to decide whether to prompt before
// a call, and the spec's defaults are pessimistic: an unannotated tool is
// assumed *not* read-only and *potentially destructive*. readOnlyHint is set
// only where a tool cannot modify state at all.
type toolBehaviour struct {
	// readOnly means the call cannot change any state.
	readOnly bool
	// destructive means an effect that is not easily undone.
	destructive bool
	// idempotent means repeating the call with the same input lands in the
	// same place.
	idempotent bool
	// openWorld means the tool reaches beyond this repository.
	openWorld bool
}

// toolBehaviours classifies every registered tool. A tool missing from this
// map fails TestEveryToolIsAnnotated rather than silently inheriting the
// pessimistic defaults.
//
// The operations that replace or discard intent — approving or rejecting a
// plan, accepting drift, re-locking the spec, pruning — are gated on the
// user's confirmation (approval.go).
var toolBehaviours = map[string]toolBehaviour{
	// Reads. roady_query returns a prompt for the caller's model; roady runs
	// no inference.
	"roady_next":   {readOnly: true, idempotent: true},
	"roady_status": {readOnly: true, idempotent: true},
	"roady_query":  {readOnly: true, idempotent: true},
	// verify reads; trail reads.
	"roady_audit":  {readOnly: true, idempotent: true},
	"roady_policy": {readOnly: true, idempotent: true},

	// Upserts; re-sending the same document is a no-op.
	"roady_capture": {idempotent: true},
	// Transitions are reversible through the FSM; check and dispatch record.
	"roady_task": {},
	// Reads git history; moves tasks it finds markers for.
	"roady_git":  {idempotent: true},
	"roady_goal": {idempotent: true},
	"roady_init": {},

	// These nouns include operations that replace intent or state (approve,
	// reject, prune, accept, lock, analyze, import, rebuild). Those run only
	// after the user confirms in their client, but the tool as a whole must
	// still say it can be destructive.
	"roady_plan":  {destructive: true},
	"roady_spec":  {destructive: true},
	"roady_drift": {destructive: true},
	"roady_state": {destructive: true},
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
