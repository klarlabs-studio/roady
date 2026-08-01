package provenance

import (
	"strings"
	"sync"
)

// Env var names callers use to identify themselves.
const (
	EnvSessionID = "ROADY_SESSION_ID"
	EnvAgent     = "ROADY_AGENT"
)

// Resolver builds a Context from the environment, minting a session ID when
// none was supplied.
type Resolver struct {
	getenv  func(string) string
	newID   func() string
	surface Surface

	once      sync.Once
	sessionID string
}

// NewResolver creates a resolver for the given surface. getenv and newID are
// injected so the resolution rules stay testable without touching real
// process state.
func NewResolver(surface Surface, getenv func(string) string, newID func() string) *Resolver {
	return &Resolver{getenv: getenv, newID: newID, surface: surface}
}

// Resolve returns the provenance context for the current run.
//
// The session ID is minted once per resolver and reused, which gives the unit
// of grouping its meaning: one CLI invocation is one session, and one
// long-lived MCP server process is one session spanning the agent's whole
// conversation. That is the granularity an auditor asks about.
func (r *Resolver) Resolve() Context {
	r.once.Do(func() {
		if v := strings.TrimSpace(r.getenv(EnvSessionID)); v != "" {
			r.sessionID = v
			return
		}
		r.sessionID = r.newID()
	})

	return Context{
		SessionID: r.sessionID,
		Agent:     r.resolveAgent(),
		Surface:   r.surface,
	}
}

// resolveAgent prefers an explicit ROADY_AGENT, then falls back to whichever
// agent runtime advertised itself in the environment. Falling back matters:
// without it every Claude Code session records as "unknown" unless the user
// remembered to export a variable, and audit history nobody configured is
// exactly the history that turns out to be missing later.
func (r *Resolver) resolveAgent() string {
	if v := strings.TrimSpace(r.getenv(EnvAgent)); v != "" {
		return v
	}

	for _, probe := range agentProbes {
		if strings.TrimSpace(r.getenv(probe.env)) != "" {
			return probe.name
		}
	}

	if r.surface == SurfaceCLI {
		return "cli"
	}
	return ""
}

// agentProbes maps a well-known environment marker to the agent it implies.
// Order matters only when a runtime sets more than one marker.
var agentProbes = []struct {
	env  string
	name string
}{
	{env: "CLAUDECODE", name: "claude-code"},
	{env: "CLAUDE_CODE_SESSION", name: "claude-code"},
	{env: "CURSOR_TRACE_ID", name: "cursor"},
	{env: "CODEX_SANDBOX", name: "codex"},
	{env: "GEMINI_CLI", name: "gemini-cli"},
}

// WithSession returns a copy of the context carrying an explicit session and
// agent, used when a caller identifies itself per request rather than per
// process.
func (c Context) WithSession(sessionID, agent string) Context {
	if s := strings.TrimSpace(sessionID); s != "" {
		c.SessionID = s
	}
	if a := strings.TrimSpace(agent); a != "" {
		c.Agent = a
	}
	return c
}
