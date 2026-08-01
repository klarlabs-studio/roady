package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	renderer "github.com/felixgeelhaar/roady/pkg/infrastructure/report"
	"github.com/spf13/cobra"
)

var (
	trailAgent   string
	trailSession string
	trailSince   string
	trailFormat  string
	trailOutput  string
)

var auditTrailCmd = &cobra.Command{
	Use:   "trail [task-id]",
	Short: "Produce an evidence trail for a task, agent, or session",
	Long: `Assemble the recorded history behind a task, an agent, or a session.

Answers the questions a GRC review asks: what proves this task was done, who
and what touched it, and has the record been altered since?

The trail reports hash-chain integrity, the task's evidence (commits, links,
external issue references), its citation back to the spec document that
motivated it, and every recorded event with the agent and session behind it.

Examples:
  roady audit trail task-42                      # everything about one task
  roady audit trail --agent claude-code --since 30d
  roady audit trail --session 9f3c1b2a-...       # one agent run
  roady audit trail task-42 --format json        # machine-readable

Note: actor, agent, and session are asserted by the caller and are not
authenticated. The trail attests to what was claimed, not to who acted.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAuditTrail,
}

func runAuditTrail(cmd *cobra.Command, args []string) error {
	var taskID string
	if len(args) == 1 {
		taskID = args[0]
	}

	if taskID == "" && trailAgent == "" && trailSession == "" {
		return fmt.Errorf("specify a task id, or --agent, or --session")
	}

	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}

	since, err := parseSince(trailSince, time.Now())
	if err != nil {
		return err
	}

	svc := application.NewAuditTrailService(
		services.Audit,
		services.Workspace.Audit,
		services.Plan,
		services.Workspace.Repo,
	)

	trail, err := svc.BuildTrail(cmd.Context(), application.TrailQuery{
		TaskID:    taskID,
		Agent:     trailAgent,
		SessionID: trailSession,
		Since:     since,
	})
	if err != nil {
		return MapError(fmt.Errorf("build audit trail: %w", err))
	}

	var rendered string
	switch strings.ToLower(trailFormat) {
	case "", "markdown", "md":
		rendered = renderer.TrailMarkdown(trail)
	case "json":
		encoded, mErr := json.MarshalIndent(trail, "", "  ")
		if mErr != nil {
			return fmt.Errorf("render json: %w", mErr)
		}
		rendered = string(encoded) + "\n"
	default:
		return fmt.Errorf("unknown format %q: use markdown or json", trailFormat)
	}

	if trailOutput == "" {
		fmt.Print(rendered)
	} else {
		if err := os.WriteFile(trailOutput, []byte(rendered), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", trailOutput, err)
		}
		fmt.Fprintf(os.Stderr, "Wrote %s\n", trailOutput)
	}

	// A failed chain is an audit finding, so it must also be a non-zero exit
	// for anything running this in CI.
	if trail.Integrity.CheckedChain && !trail.Integrity.Verified {
		return fmt.Errorf("audit chain verification failed")
	}
	return nil
}

func init() {
	auditTrailCmd.Flags().StringVar(&trailAgent, "agent", "", "Only events from this agent (e.g. claude-code)")
	auditTrailCmd.Flags().StringVar(&trailSession, "session", "", "Only events from this session ID")
	auditTrailCmd.Flags().StringVar(&trailSince, "since", "", "Only events since this point (e.g. 7d, 2w, 2026-07-01)")
	auditTrailCmd.Flags().StringVarP(&trailFormat, "format", "f", "markdown", "Output format (markdown, json)")
	auditTrailCmd.Flags().StringVarP(&trailOutput, "output", "o", "", "Write to a file instead of stdout")

	auditCmd.AddCommand(auditTrailCmd)
}
