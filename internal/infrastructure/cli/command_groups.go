package cli

import "github.com/spf13/cobra"

const (
	groupGetStarted  = "getting-started"
	groupTrackReport = "track-report"
	groupIntegrate   = "integrate"
	groupAdmin       = "admin"
)

// commandGroupAssignments maps a top-level command name (cmd.Name()) to the
// group it belongs to. Commands not listed are left ungrouped and Cobra
// renders them under "Additional Commands".
var commandGroupAssignments = map[string]string{
	"init":    groupGetStarted,
	"setup":   groupGetStarted,
	"next":    groupGetStarted,
	"status":  groupGetStarted,
	"task":    groupGetStarted,
	"plan":    groupGetStarted,
	"spec":    groupGetStarted,
	"capture": groupGetStarted,
	"goal":    groupGetStarted,
	"add":     groupGetStarted,
	"edit":    groupGetStarted,
	"split":   groupGetStarted,
	"move":    groupGetStarted,

	"drift": groupTrackReport,
	"query": groupTrackReport,

	"mcp": groupIntegrate,
	"git": groupIntegrate,

	"audit":      groupAdmin,
	"doctor":     groupAdmin,
	"completion": groupAdmin,
	"config":     groupAdmin,
	"policy":     groupAdmin,
	"state":      groupAdmin,
}

// assignCommandGroups registers Cobra command groups on RootCmd and assigns
// each top-level subcommand to its group. Called from Execute at runtime so
// all sub-package init() registrations have already completed. Idempotent.
func assignCommandGroups() {
	existing := make(map[string]bool, len(RootCmd.Groups()))
	for _, g := range RootCmd.Groups() {
		existing[g.ID] = true
	}
	groups := []*cobra.Group{
		{ID: groupGetStarted, Title: "Get Started:"},
		{ID: groupTrackReport, Title: "Track & Report:"},
		{ID: groupIntegrate, Title: "Integrate:"},
		{ID: groupAdmin, Title: "Admin:"},
	}
	for _, g := range groups {
		if !existing[g.ID] {
			RootCmd.AddGroup(g)
		}
	}

	for _, cmd := range RootCmd.Commands() {
		if id, ok := commandGroupAssignments[cmd.Name()]; ok {
			cmd.GroupID = id
		}
	}
}
