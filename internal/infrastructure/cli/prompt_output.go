package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	promptdomain "github.com/felixgeelhaar/roady/pkg/domain/prompt"
)

// printPromptRequest renders work that needs a language model.
//
// Roady assembles the context and hands it over; it does not run inference.
// The human-readable form goes to stdout so it can be piped straight into
// whatever model the caller already has, with the framing and write-back path
// on stderr so they never contaminate the prompt itself.
func printPromptRequest(req *promptdomain.Request, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(req)
	}

	if req.System != "" {
		fmt.Fprintf(os.Stderr, "System: %s\n\n", req.System)
	}

	fmt.Print(req.Prompt)
	if len(req.Prompt) > 0 && req.Prompt[len(req.Prompt)-1] != '\n' {
		fmt.Println()
	}

	if req.ExpectedFormat != "" {
		fmt.Fprintf(os.Stderr, "\nExpected answer format:\n%s\n", req.ExpectedFormat)
	}
	if req.Guidance != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", req.Guidance)
	}
	if req.NeedsWriteBack() {
		fmt.Fprintf(os.Stderr, "Send the result back with: %s\n", req.WriteBack)
	}
	return nil
}

// promptJSON is set by the --json flag on every command that emits a prompt.
var promptJSON bool

// addPromptJSONFlag registers the shared flag, so an agent can consume the
// request as data rather than parsing terminal output.
func addPromptJSONFlag(cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.Flags().BoolVar(&promptJSON, "json", false, "Emit the prompt as JSON for an agent to consume")
	}
}
