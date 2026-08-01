package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	msginfra "github.com/felixgeelhaar/roady/internal/infrastructure/messaging"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/messaging"
	renderer "github.com/felixgeelhaar/roady/pkg/infrastructure/report"
	"github.com/spf13/cobra"
)

var (
	digestSince   string
	digestAdapter string
	digestDryRun  bool
)

var notifyDigestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Send a progress summary to configured notification channels",
	Long: `Send a single progress summary to your notification adapters.

Per-event notifications tell a channel that one task moved. A digest tells it
where the project stands — progress, pace, and what is at risk — in one
message. Run it on a schedule (cron, a CI job, a launchd timer) to keep a
channel informed without anyone opening a dashboard.

Examples:
  roady notify digest --dry-run           # print what would be sent
  roady notify digest --since 7d          # weekly summary
  roady notify digest --adapter team-chat # one channel only`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return notifyDigest(cmd.Context(), cmd.OutOrStdout())
	},
}

func notifyDigest(ctx context.Context, out io.Writer) error {
	services, err := loadServicesForCurrentDir()
	if err != nil {
		return err
	}

	since, err := parseSince(digestSince, time.Now())
	if err != nil {
		return err
	}

	rep, err := services.Report.Generate(ctx, application.ReportOptions{
		Project: inferProjectName(),
		Since:   since,
	})
	if err != nil {
		return MapError(fmt.Errorf("generate digest: %w", err))
	}

	summary := renderer.Digest(rep)

	if digestDryRun {
		_, _ = fmt.Fprint(out, summary)
		return nil
	}

	config, err := services.Workspace.Repo.LoadMessagingConfig()
	if err != nil || config == nil || len(config.Adapters) == 0 {
		return fmt.Errorf("no notification adapters configured: add one with 'roady notify add <name> <type> <url>'")
	}

	selected := selectAdapters(config, digestAdapter)
	if len(selected.Adapters) == 0 {
		if digestAdapter != "" {
			return fmt.Errorf("adapter %q not found or disabled", digestAdapter)
		}
		return fmt.Errorf("no enabled notification adapters")
	}

	registry, err := msginfra.NewRegistry(selected)
	if err != nil {
		return fmt.Errorf("create adapters: %w", err)
	}

	event := &events.BaseEvent{
		Type:      events.EventTypeReportDigest,
		Timestamp: time.Now(),
		Actor:     "roady-cli",
		Metadata: map[string]any{
			"summary":  summary,
			"project":  rep.Project,
			"headline": rep.Headline(),
			"percent":  rep.Progress.Percent,
			"risks":    len(rep.Risks),
		},
	}

	var failures int
	for _, adapter := range registry.Adapters() {
		if sendErr := adapter.Send(ctx, event); sendErr != nil {
			failures++
			_, _ = fmt.Fprintf(out, "Failed to send digest to %q: %v\n", adapter.Name(), sendErr)
			continue
		}
		_, _ = fmt.Fprintf(out, "Sent digest to %q\n", adapter.Name())
	}

	if failures > 0 {
		return fmt.Errorf("%d of %d adapters failed", failures, len(registry.Adapters()))
	}
	return nil
}

// selectAdapters narrows the configured adapters to the enabled ones, or to a
// single named adapter when one was requested.
func selectAdapters(config *messaging.MessagingConfig, name string) *messaging.MessagingConfig {
	selected := &messaging.MessagingConfig{}
	for _, a := range config.Adapters {
		if !a.Enabled {
			continue
		}
		if name != "" && a.Name != name {
			continue
		}
		selected.Adapters = append(selected.Adapters, a)
	}
	return selected
}

func init() {
	notifyDigestCmd.Flags().StringVar(&digestSince, "since", "", "Only summarise changes since this point (e.g. 7d, 2w, 2026-07-01)")
	notifyDigestCmd.Flags().StringVar(&digestAdapter, "adapter", "", "Send to a single named adapter instead of all enabled ones")
	notifyDigestCmd.Flags().BoolVar(&digestDryRun, "dry-run", false, "Print the digest instead of sending it")

	notifyCmd.AddCommand(notifyDigestCmd)
}
