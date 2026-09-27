package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	captureFile   string
	captureDryRun bool
	captureJSON   bool
	captureChecks bool
)

var captureCmd = &cobra.Command{
	Use:   "capture",
	Short: "Record features, requirements and tasks in one write (YAML or JSON)",
	Long: `Record intent at whatever size it arrives: one task, a few, or a whole plan
with the features and requirements behind it.

Every item is keyed by id and upserted; fields left out keep their current
value, so the same shape edits an item. A requirement gets its task
(task-<requirement id>) automatically. The capture is all or nothing: if any
item is invalid, nothing is written and each rejection is reported. Sending the
same document again changes nothing.

Reads from --file, or stdin when no file is given:

  roady capture <<'EOF'
  features:
    - id: invoices
      title: Invoice generation
      requirements:
        - id: seq-numbers
          title: Sequential gap-free invoice numbers per year
          priority: high
          check:
            run: go test ./internal/invoice -run TestNumbering
  tasks:
    - id: task-invoice-migration
      title: Migrate existing invoice numbers
      requirement: seq-numbers
      depends_on: [task-seq-numbers]
  EOF`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := getProjectRoot()
		if err != nil {
			return fmt.Errorf("resolve project path: %w", err)
		}
		in := cmd.InOrStdin()
		if captureFile != "" && captureFile != "-" {
			f, err := os.Open(captureFile) // #nosec G304 -- user-named input file
			if err != nil {
				return fmt.Errorf("open %s: %w", captureFile, err)
			}
			defer func() { _ = f.Close() }()
			in = f
		}
		raw, err := io.ReadAll(in)
		if err != nil {
			return fmt.Errorf("read capture: %w", err)
		}
		doc, err := parseCaptureDoc(raw)
		if err != nil {
			return err
		}

		ws := wiring.NewWorkspace(root)
		svc := application.NewCaptureService(ws.Repo, ws.Audit)
		actor := resolveCurrentOwner(gitConfigUserName)
		if actor == "" {
			actor = "unknown-human"
		}
		result, err := svc.Capture(doc, application.CaptureOptions{Actor: actor, DryRun: captureDryRun, Origin: planning.OriginHuman, AllowCheckChange: captureChecks})
		if err != nil {
			return MapError(fmt.Errorf("capture failed: %w", err))
		}
		if captureJSON {
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			if err := enc.Encode(result); err != nil {
				return err
			}
		} else {
			printCaptureResult(cmd.OutOrStdout(), result)
		}
		if len(result.Rejected) > 0 {
			os.Exit(1)
		}
		return nil
	},
}

// parseCaptureDoc accepts YAML or JSON; JSON is valid YAML, so one decoder
// covers both. Unknown fields are refused, so a misspelt key is reported
// rather than silently dropped.
func parseCaptureDoc(raw []byte) (application.CaptureDoc, error) {
	var doc application.CaptureDoc
	if len(strings.TrimSpace(string(raw))) == 0 {
		return doc, fmt.Errorf("nothing to capture: provide YAML or JSON on stdin or with --file")
	}
	dec := yaml.NewDecoder(strings.NewReader(string(raw)))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil {
		return doc, fmt.Errorf("parse capture document: %w", err)
	}
	return doc, nil
}

func printCaptureResult(w io.Writer, r *application.CaptureResult) {
	switch {
	case len(r.Rejected) > 0:
		_, _ = fmt.Fprintln(w, "Nothing written: the capture was rejected.")
		for _, rej := range r.Rejected {
			_, _ = fmt.Fprintf(w, "  ✗ %s: %s\n", rej.Item, rej.Reason)
		}
		return
	case !r.Changed():
		_, _ = fmt.Fprintf(w, "No changes (%d item(s) already as captured).\n", r.Unchanged)
		return
	case r.DryRun:
		_, _ = fmt.Fprintln(w, "Dry run — nothing written. Would apply:")
	default:
		_, _ = fmt.Fprintln(w, "Captured:")
	}
	for _, c := range r.Created {
		_, _ = fmt.Fprintf(w, "  + %s\n", c)
	}
	for _, u := range r.Updated {
		_, _ = fmt.Fprintf(w, "  ~ %s\n", u)
	}
	if r.Unchanged > 0 {
		_, _ = fmt.Fprintf(w, "  (%d unchanged)\n", r.Unchanged)
	}
	if r.PlanApproval != "" {
		_, _ = fmt.Fprintf(w, "Plan %s: %s", r.PlanID, r.PlanApproval)
		if r.ApprovalReason != "" {
			_, _ = fmt.Fprintf(w, " (%s)", r.ApprovalReason)
		}
		_, _ = fmt.Fprintln(w)
	}
}

func init() {
	captureCmd.Flags().StringVarP(&captureFile, "file", "f", "", "Read the capture document from a file instead of stdin")
	captureCmd.Flags().BoolVar(&captureDryRun, "dry-run", false, "Report what would change without writing")
	captureCmd.Flags().BoolVar(&captureJSON, "json", false, "Print the result as JSON")
	captureCmd.Flags().BoolVar(&captureChecks, "change-checks", false, "Allow removing or changing the acceptance check of work already started (reopens done tasks; recorded as an override)")
	RootCmd.AddCommand(captureCmd)
}
