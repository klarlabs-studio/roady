package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// emptyStateStep represents a remaining onboarding step. The first stage that
// applies is what the user is told to run next. Stages are listed in order.
type emptyStateStep struct {
	Stage   string `json:"stage"`          // machine-readable id
	Reason  string `json:"reason"`         // why we are stopping here
	Command string `json:"command"`        // suggested command
	Hint    string `json:"hint,omitempty"` // extra context
}

// emptyStateHintForCurrentDir inspects the project root and returns a hint
// when an upstream artifact is missing (no .roady/, no spec.yaml, etc).
// Callers can short-circuit `status` and other read-only commands with the
// hint instead of surfacing a raw filesystem error to the user.
func emptyStateHintForCurrentDir() (emptyStateStep, bool) {
	root, err := getProjectRoot()
	if err != nil {
		return emptyStateStep{}, false
	}
	return emptyStateHintForRoot(root)
}

func emptyStateHintForRoot(root string) (emptyStateStep, bool) {
	roadyDir := filepath.Join(root, ".roady")
	if !pathExists(roadyDir) {
		return emptyStateStep{
			Stage:   "uninitialised",
			Reason:  "No .roady/ directory found in this project.",
			Command: "roady init",
			Hint:    "Then `roady setup <agent>` connects your coding agent (claude-code, codex, gemini, cursor, ...).",
		}, true
	}

	if !pathExists(filepath.Join(roadyDir, "spec.yaml")) {
		return emptyStateStep{
			Stage:   "no-spec",
			Reason:  "No spec.yaml yet — Roady doesn't know what you're building.",
			Command: "roady spec analyze docs/",
			Hint:    "Or rerun `roady init` with --interactive to pick a starter template.",
		}, true
	}

	if !pathExists(filepath.Join(roadyDir, "plan.json")) {
		return emptyStateStep{
			Stage:   "no-plan",
			Reason:  "Spec exists but no plan has been generated.",
			Command: "roady plan generate",
			Hint:    "Add --ai if your AI provider is configured.",
		}, true
	}

	return emptyStateStep{}, false
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeEmptyStateText(cmd *cobra.Command, step emptyStateStep) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, step.Reason)
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "Next: %s\n", step.Command)
	if step.Hint != "" {
		_, _ = fmt.Fprintf(out, "Hint: %s\n", step.Hint)
	}
}

func writeEmptyStateJSON(cmd *cobra.Command, step emptyStateStep) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		EmptyState emptyStateStep `json:"empty_state"`
	}{EmptyState: step})
}
