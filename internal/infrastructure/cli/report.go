package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	renderer "github.com/felixgeelhaar/roady/pkg/infrastructure/report"
	"github.com/spf13/cobra"
)

var (
	reportFormat  string
	reportOutput  string
	reportSince   string
	reportProject string
	reportMax     int
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a progress report for people who do not run the CLI",
	Long: `Generate a stakeholder-facing progress report.

The report answers the three questions a lead or stakeholder asks: how far
along are we, what is at risk, and who is working on what. Output is a single
self-contained document — commit it, email it, paste it into a pull request,
or publish it to a static host. Nothing to install and nothing to log into.

Examples:
  roady report                                  # Markdown to stdout
  roady report --since 7d                       # only the last week's changes
  roady report --format html -o status.html     # a self-contained page
  roady report --format json | jq .risks        # machine-readable`,
	Args: cobra.NoArgs,
	RunE: runReport,
}

func runReport(cmd *cobra.Command, _ []string) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}

	since, err := parseSince(reportSince, time.Now())
	if err != nil {
		return err
	}

	name := reportProject
	if name == "" {
		name = inferProjectName()
	}

	rep, err := services.Report.Generate(cmd.Context(), application.ReportOptions{
		Project:    name,
		Since:      since,
		MaxChanges: reportMax,
	})
	if err != nil {
		return MapError(fmt.Errorf("generate report: %w", err))
	}

	var rendered string
	switch strings.ToLower(reportFormat) {
	case "", "markdown", "md":
		rendered = renderer.Markdown(rep)
	case "html":
		rendered, err = renderer.HTML(rep)
		if err != nil {
			return fmt.Errorf("render html: %w", err)
		}
	case "json":
		encoded, mErr := json.MarshalIndent(rep, "", "  ")
		if mErr != nil {
			return fmt.Errorf("render json: %w", mErr)
		}
		rendered = string(encoded) + "\n"
	default:
		return fmt.Errorf("unknown format %q: use markdown, html, or json", reportFormat)
	}

	if reportOutput == "" {
		fmt.Print(rendered)
		return nil
	}

	if err := os.WriteFile(reportOutput, []byte(rendered), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", reportOutput, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote %s\n", reportOutput)
	return nil
}

// parseSince accepts a relative duration in days or weeks ("7d", "2w") or an
// absolute date ("2026-07-01"). An empty value means the whole history.
// parseSince delegates to the shared parser so the CLI and the MCP server
// cannot drift apart on what `7d` means. They previously had separate copies
// and disagreed: this one used fmt.Sscanf and silently accepted "7xd" as seven
// days.
func parseSince(value string, now time.Time) (time.Time, error) {
	return application.ParseSince(value, now)
}

// inferProjectName falls back to the directory name so a report is never
// titled "Project" when something better is available for free.
func inferProjectName() string {
	root, err := getProjectRoot()
	if err != nil {
		return ""
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	return filepath.Base(abs)
}

func init() {
	reportCmd.Flags().StringVarP(&reportFormat, "format", "f", "markdown", "Output format (markdown, html, json)")
	reportCmd.Flags().StringVarP(&reportOutput, "output", "o", "", "Write to a file instead of stdout")
	reportCmd.Flags().StringVar(&reportSince, "since", "", "Only include changes since this point (e.g. 7d, 2w, 2026-07-01)")
	reportCmd.Flags().StringVar(&reportProject, "name", "", "Project name for the report header (default: directory name)")
	reportCmd.Flags().IntVar(&reportMax, "max-changes", 0, "Maximum number of changes to list (default 25)")

	RootCmd.AddCommand(reportCmd)
}
