// Package provenance records who or what performed an action, so an audit
// trail can answer "which agent, in which session, did this?" rather than
// only "something called ai-agent did this".
//
// # Attestation limits
//
// Identity here is *asserted*, not *authenticated*. A caller supplies its own
// agent name and session ID, and nothing verifies the claim. The hash-chained
// event log proves the record was not altered after it was written; it does
// not prove the actor was who it said it was.
//
// Read that boundary carefully before taking a Roady trail to an auditor:
// Roady offers a complete, tamper-evident record of what was asserted at the
// time, which is a real and useful thing. It is not proof of identity, and
// getting there would require signed attestations.
package provenance

import (
	"strings"
)

// Metadata keys under which provenance is stamped onto every event.
const (
	KeySessionID = "session_id"
	KeyAgent     = "agent"
	KeySurface   = "surface"
)

// Surface names how an action reached Roady. It distinguishes a human typing
// a command from an agent calling a tool, which is the first question a
// reviewer asks of any change.
type Surface string

const (
	// SurfaceCLI is a command run in a terminal.
	SurfaceCLI Surface = "cli"
	// SurfaceMCP is a tool call from an agent over MCP.
	SurfaceMCP Surface = "mcp"
	// SurfacePlugin is an external syncer acting on Roady's behalf.
	SurfacePlugin Surface = "plugin"
	// SurfaceUnknown is used when nothing identified itself.
	SurfaceUnknown Surface = "unknown"
)

// Context identifies the actor behind a run.
//
// SessionID groups every action from one CLI invocation or one MCP server
// process, which is what makes "show me everything session X did" answerable.
// Agent names the software (claude-code, codex, cursor, or a human's shell).
type Context struct {
	SessionID string
	Agent     string
	Surface   Surface
}

// Unknown is the zero-information context, used when identity cannot be
// resolved. It is deliberately valid rather than an error: refusing to record
// an event because its actor is unidentified would lose more audit history
// than it protects.
func Unknown() Context {
	return Context{Surface: SurfaceUnknown}
}

// IsZero reports whether the context carries no identifying information.
func (c Context) IsZero() bool {
	return c.SessionID == "" && c.Agent == "" &&
		(c.Surface == "" || c.Surface == SurfaceUnknown)
}

// Apply stamps this context onto an event's metadata, creating the map when
// absent. Existing keys are never overwritten: a caller that supplied its own
// session — an agent forwarding the identity of the run that spawned it —
// knows better than the ambient process does.
func (c Context) Apply(metadata map[string]any) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}

	setIfAbsent(metadata, KeySessionID, c.SessionID)
	setIfAbsent(metadata, KeyAgent, c.Agent)
	if c.Surface != "" && c.Surface != SurfaceUnknown {
		setIfAbsent(metadata, KeySurface, string(c.Surface))
	}

	return metadata
}

func setIfAbsent(metadata map[string]any, key, value string) {
	if value == "" {
		return
	}
	if existing, ok := metadata[key].(string); ok && existing != "" {
		return
	}
	metadata[key] = value
}

// FromMetadata reads provenance back off a recorded event.
func FromMetadata(metadata map[string]any) Context {
	if metadata == nil {
		return Unknown()
	}

	ctx := Context{
		SessionID: stringValue(metadata, KeySessionID),
		Agent:     stringValue(metadata, KeyAgent),
		Surface:   Surface(stringValue(metadata, KeySurface)),
	}
	if ctx.Surface == "" {
		ctx.Surface = SurfaceUnknown
	}
	return ctx
}

func stringValue(metadata map[string]any, key string) string {
	if v, ok := metadata[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

// Matches reports whether this context satisfies a query for the given agent
// or session. An empty filter matches anything; comparison is
// case-insensitive because agent names arrive from env vars and tool args
// with inconsistent casing.
func (c Context) Matches(agent, sessionID string) bool {
	if agent != "" && !strings.EqualFold(strings.TrimSpace(agent), c.Agent) {
		return false
	}
	if sessionID != "" && !strings.EqualFold(strings.TrimSpace(sessionID), c.SessionID) {
		return false
	}
	return true
}
