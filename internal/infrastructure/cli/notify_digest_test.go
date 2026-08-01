package cli

import (
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/messaging"
)

func TestSelectAdapters(t *testing.T) {
	config := &messaging.MessagingConfig{Adapters: []messaging.AdapterConfig{
		{Name: "team-chat", Type: "slack", Enabled: true},
		{Name: "archive", Type: "webhook", Enabled: true},
		{Name: "retired", Type: "slack", Enabled: false},
	}}

	tests := []struct {
		name      string
		requested string
		wantNames []string
	}{
		{name: "all enabled by default", wantNames: []string{"team-chat", "archive"}},
		{name: "single named adapter", requested: "archive", wantNames: []string{"archive"}},
		{name: "disabled adapter is never selected", requested: "retired", wantNames: nil},
		{name: "unknown adapter selects nothing", requested: "nope", wantNames: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectAdapters(config, tt.requested)
			if len(got.Adapters) != len(tt.wantNames) {
				t.Fatalf("expected %v, got %d adapters", tt.wantNames, len(got.Adapters))
			}
			for i, want := range tt.wantNames {
				if got.Adapters[i].Name != want {
					t.Errorf("index %d: expected %s, got %s", i, want, got.Adapters[i].Name)
				}
			}
		})
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{name: "empty means all history", input: "", want: time.Time{}},
		{name: "days", input: "7d", want: now.AddDate(0, 0, -7)},
		{name: "weeks", input: "2w", want: now.AddDate(0, 0, -14)},
		{name: "absolute date", input: "2026-07-01", want: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		{name: "whitespace tolerated", input: "  7d ", want: now.AddDate(0, 0, -7)},
		{name: "zero days rejected", input: "0d", wantErr: true},
		{name: "negative rejected", input: "-3d", wantErr: true},
		{name: "garbage rejected", input: "soon", wantErr: true},
		{name: "bad date rejected", input: "2026-13-45", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSince(tt.input, now)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %v", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("parseSince(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
