package cli

import (
	"testing"

	"github.com/felixgeelhaar/roady/internal/infrastructure/mcp"
	"github.com/spf13/cobra"
)

// hostOnly commands configure the machine or the agent host, not the
// project, so they have no MCP counterpart.
var hostOnly = map[string]bool{"setup": true, "hook": true, "mcp": true, "completion": true, "config": true, "doctor": true, "help": true}

// verbAlias maps CLI verbs to the MCP action that covers them.
var verbAlias = map[string]map[string]string{
	"task": {"ready": "list", "blocked": "list", "in-progress": "list", "mine": "list", "assigned": "list", "unassigned": "list"},
	"plan": {"smart-decompose": "decompose"},
}

// leafTool maps verb-less CLI commands to their tool.
var leafTool = map[string]string{
	"init": "roady_init", "next": "roady_next", "status": "roady_status", "query": "roady_query", "capture": "roady_capture",
	"add": "roady_capture", "edit": "roady_capture", "split": "roady_capture", "move": "roady_capture",
}

// Parity: every project operation on the CLI is reachable over MCP, under the
// same noun and verb. A new command fails this test until it has a tool.
func TestEveryCLICommandHasAnMCPTool(t *testing.T) {
	single := map[string]bool{}
	for _, s := range mcp.SingleTools {
		single[s] = true
	}
	for _, cmd := range RootCmd.Commands() {
		name := cmd.Name()
		if hostOnly[name] {
			continue
		}
		verbs := visible(cmd.Commands())
		if len(verbs) == 0 {
			tool, ok := leafTool[name]
			if !ok || !single[tool] {
				t.Errorf("roady %s has no MCP tool", name)
			}
			continue
		}
		actions, ok := mcp.NounActions["roady_"+name]
		if !ok {
			t.Errorf("roady %s has no MCP tool roady_%s", name, name)
			continue
		}
		have := map[string]bool{}
		for _, a := range actions {
			have[a] = true
		}
		for _, v := range verbs {
			action := v
			if alias, ok := verbAlias[name][v]; ok {
				action = alias
			}
			if !have[action] {
				t.Errorf("roady %s %s has no action on roady_%s", name, v, name)
			}
		}
	}
}

func visible(cmds []*cobra.Command) []string {
	var out []string
	for _, c := range cmds {
		if !c.Hidden && c.Name() != "help" {
			out = append(out, c.Name())
		}
	}
	return out
}
