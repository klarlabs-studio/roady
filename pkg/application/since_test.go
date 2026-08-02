package application_test

import (
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		value   string
		want    time.Time
		wantErr bool
	}{
		{"empty means whole history", "", time.Time{}, false},
		{"whitespace is empty", "   ", time.Time{}, false},
		{"days", "7d", now.AddDate(0, 0, -7), false},
		{"weeks", "2w", now.AddDate(0, 0, -14), false},
		{"absolute date", "2026-07-01", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), false},

		// The CLI used fmt.Sscanf, which stops at the first non-digit and
		// reports no error, so "7xd" silently meant seven days there while the
		// same string was rejected over MCP. Two parsers, two answers, for the
		// one flag a person types by hand.
		{"trailing garbage is rejected", "7xd", time.Time{}, true},
		{"hex is rejected", "0x10d", time.Time{}, true},
		{"zero is rejected", "0d", time.Time{}, true},
		{"negative is rejected", "-3d", time.Time{}, true},
		{"bare number is rejected", "7", time.Time{}, true},
		{"nonsense is rejected", "yesterday", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := application.ParseSince(tt.value, now)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSince(%q) = %v, want an error", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSince(%q): %v", tt.value, err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseSince(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

// The message has to name the accepted forms, since the value is typed by hand.
func TestParseSinceErrorNamesTheAcceptedForms(t *testing.T) {
	_, err := application.ParseSince("yesterday", time.Now())
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"7d", "2w", "2026-07-01"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not show the %q form", err, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 || indexOf(haystack, needle) >= 0)
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
