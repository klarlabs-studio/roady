package messaging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/messaging"
)

// SlackAdapter sends events to a Slack incoming webhook URL.
type SlackAdapter struct {
	config messaging.AdapterConfig
	client *http.Client
}

// NewSlackAdapter creates a Slack adapter from config.
func NewSlackAdapter(config messaging.AdapterConfig) *SlackAdapter {
	return &SlackAdapter{
		config: config,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *SlackAdapter) Name() string { return a.config.Name }
func (a *SlackAdapter) Type() string { return "slack" }

func (a *SlackAdapter) Send(ctx context.Context, event *events.BaseEvent) error {
	text := formatSlackMessage(event)

	payload := map[string]interface{}{
		"text": text,
		"blocks": []map[string]interface{}{
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": text,
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("send to slack: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close on read body

	if resp.StatusCode >= 300 {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	return nil
}

func formatSlackMessage(event *events.BaseEvent) string {
	// A digest carries its own prerendered body; per-event formatting would
	// throw away the summary it exists to deliver.
	if summary, ok := event.Metadata["summary"].(string); ok && summary != "" {
		return summary
	}

	switch event.Type {
	case events.EventTypeTaskStarted:
		return fmt.Sprintf(":arrow_forward: Task started: %s", event.AggregateID())
	case events.EventTypeTaskCompleted:
		return fmt.Sprintf(":white_check_mark: Task completed: %s", event.AggregateID())
	case events.EventTypeDriftDetected:
		return ":warning: Drift detected in project"
	case events.EventTypePlanCreated:
		return fmt.Sprintf(":clipboard: New plan created: %s", event.AggregateID())
	default:
		return fmt.Sprintf("Roady event: %s", event.Type)
	}
}
