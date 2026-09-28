package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	gitSuggestLimit int
	gitSuggestJSON  bool
)

var gitSuggestCmd = &cobra.Command{
	Use:   "suggest",
	Short: "Name the task each commit without a roady marker most likely served",
	Long: `List commits since the plan was last updated that no task claims — no
[roady:<task>] marker for a task in the plan, not linked as a task's evidence —
with the task whose id and title share most words with the subject. Writes
nothing.

These are the commits drift counts when it calls the plan stale. Planned work
committed without markers is most of them in a project that adopted roady
late; link each with ` + "`roady git link <commit> <task>`" + `, and drift stops counting it.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		sugs, err := services.Git.Suggest(gitSuggestLimit)
		if err != nil {
			return MapError(err)
		}
		out := cmd.OutOrStdout()
		if gitSuggestJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(sugs)
		}
		if len(sugs) == 0 {
			_, _ = fmt.Fprintln(out, "Every commit since the plan was last updated is claimed by a task.")
			return nil
		}
		_, _ = fmt.Fprintf(out, "%d commit(s) since the plan was last updated that no task claims:\n", len(sugs))
		for _, s := range sugs {
			_, _ = fmt.Fprintf(out, "  %s %s\n", s.Commit.Hash[:8], s.Commit.Subject)
			if s.TaskID != "" {
				_, _ = fmt.Fprintf(out, "      → %s (%s)  shares: %s\n", s.TaskID, s.TaskTitle, strings.Join(s.Shared, ", "))
				_, _ = fmt.Fprintf(out, "        roady git link %s %s\n", s.Commit.Hash[:8], s.TaskID)
			} else {
				_, _ = fmt.Fprintln(out, "      → no likely task")
			}
		}
		return nil
	},
}

var gitLinkCmd = &cobra.Command{
	Use:   "link <commit> <task-id>",
	Short: "Record a commit as a task's evidence, for work committed without a roady marker",
	Long: `Record a commit as evidence on a task: the link a [roady:<task>] marker would
have made. The task's status does not change — complete or verify it as usual.
Drift stops counting the commit as work the plan does not cover.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}
		actor := resolveCurrentOwner(gitConfigUserName)
		if actor == "" {
			actor = "unknown-human"
		}
		hash, linked, err := services.Git.Link(args[0], args[1], actor)
		if err != nil {
			return MapError(fmt.Errorf("link: %w", err))
		}
		if linked {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Linked %s to %s.\n", hash[:8], args[1])
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s was already linked to %s.\n", hash[:8], args[1])
		}
		return nil
	},
}

func init() {
	gitSuggestCmd.Flags().IntVar(&gitSuggestLimit, "limit", 50, "Show at most this many commits (0 for all)")
	gitSuggestCmd.Flags().BoolVar(&gitSuggestJSON, "json", false, "Print the suggestions as JSON")
	gitCmd.AddCommand(gitSuggestCmd, gitLinkCmd)
}
