package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var semanticJSON bool

var driftSemanticCmd = &cobra.Command{
	Use:   "semantic",
	Short: "Build the prompt for judging whether implementations still mean what their requirements say",
	Long: `Build the question the structural drift checks cannot ask.

Every other check decides by comparing artifacts: a task is missing, an id is
orphaned, a file does not exist. None of them can tell whether code that exists
still does what the requirement asked for — "sessions expire after 30 minutes"
is structurally satisfied by an implementation that expires them after thirty
days.

Answering that needs a reader, so Roady assembles the pairing and hands it
over: the requirement's own words, where the work landed, and the doc:line to
check against. Roady does not judge — you already have a model, and it has the
working tree in view.

Only requirements something claims to implement are included. A requirement
with no task is structural drift the other checks already report.

Run the prompt yourself, then send the judgements to roady_record_semantic_drift.

Examples:
  roady drift semantic
  roady drift semantic --json | your-model`,
	RunE: func(cmd *cobra.Command, args []string) error {
		services, err := loadServicesForCurrentDir()
		if err != nil {
			return err
		}

		req, questions, err := services.Prompt.SemanticDrift(cmd.Context())
		if err != nil {
			return MapError(err)
		}

		if semanticJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"request": req, "questions": questions})
		}

		fmt.Printf("# Semantic drift — %d requirement(s) to judge\n\n", len(questions))
		if req.System != "" {
			fmt.Printf("System:\n%s\n\n", req.System)
		}
		fmt.Println(req.Prompt)
		if req.ExpectedFormat != "" {
			fmt.Printf("\nExpected format:\n%s\n", req.ExpectedFormat)
		}
		// Guidance goes to stderr so a piped prompt stays a prompt.
		fmt.Fprintf(os.Stderr, "\n%s\n", req.Guidance)
		return nil
	},
}

func init() {
	driftSemanticCmd.Flags().BoolVar(&semanticJSON, "json", false, "Emit the request and questions as JSON for a model to consume")
	driftCmd.AddCommand(driftSemanticCmd)
}
