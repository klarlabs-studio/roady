package wiring

import (
	"os"

	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
	"github.com/google/uuid"
)

// ambientProvenance is the identity stamped onto events recorded by this
// process. It is resolved once so every event from one CLI invocation, or one
// long-lived MCP server process, shares a session ID — which is the unit an
// auditor asks about ("what did session X do?").
var ambientProvenance = provenance.NewResolver(
	provenance.SurfaceCLI,
	os.Getenv,
	func() string { return uuid.New().String() },
)

// SetProvenanceSurface declares how this process is being driven. The MCP
// server calls it during start-up; anything else is a CLI run.
func SetProvenanceSurface(surface provenance.Surface) {
	ambientProvenance = provenance.NewResolver(
		surface,
		os.Getenv,
		func() string { return uuid.New().String() },
	)
}

// AmbientProvenance returns the identity for the current process.
func AmbientProvenance() provenance.Context {
	return ambientProvenance.Resolve()
}
