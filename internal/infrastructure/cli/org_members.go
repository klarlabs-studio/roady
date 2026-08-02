package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/spf13/cobra"
)

var orgMembersJSON bool

var orgMembersCmd = &cobra.Command{
	Use:   "members [root-dir]",
	Short: "List the repositories belonging to this workspace",
	Long: `List the repositories belonging to this workspace.

Members come from the repos: list in .roady/org.yaml when it is present.
A declared list is authoritative: it can name repositories outside the
workspace directory, and a repository it does not name is left out even if
one happens to sit inside.

With no org.yaml, or one declaring no repos, Roady falls back to walking
the tree for .roady directories.

A declared member that cannot be used — missing, or holding no Roady
project — is reported rather than skipped, because a workspace that
silently omits a repository reports progress across repositories nobody is
looking at.

Examples:
  roady org members
  roady org members ../workspace --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}

		set, err := application.NewOrgService(root).ResolveMembers()
		if err != nil {
			return MapError(err)
		}

		if orgMembersJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(set)
		}

		source := "discovered by walking the tree"
		if set.IsDeclared() {
			source = "declared in .roady/org.yaml"
		}

		usable := set.Usable()
		fmt.Printf("Workspace members (%d, %s)\n", len(usable), source)
		fmt.Println("---------------------")
		if len(usable) == 0 {
			fmt.Println("  (none)")
		}
		for _, m := range usable {
			fmt.Printf("  %-24s %s\n", m.Name(), m.Path)
		}

		// Problems go to stderr so a piped listing stays a listing, and are
		// never silent: an unresolvable member is the difference between a
		// report about the workspace and a report about part of it.
		for _, p := range set.Problems() {
			fmt.Fprintf(os.Stderr, "warning: %s\n", p)
		}

		return nil
	},
}

func init() {
	orgMembersCmd.Flags().BoolVar(&orgMembersJSON, "json", false, "Output as JSON")
	orgCmd.AddCommand(orgMembersCmd)
}
