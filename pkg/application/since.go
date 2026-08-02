package application

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sinceDateLayout is the absolute form accepted by every surface.
const sinceDateLayout = "2006-01-02"

// ParseSince interprets a "how far back" window: a relative count of days
// (`7d`) or weeks (`2w`), or an absolute date (`2026-07-01`). An empty value
// means the whole history and returns the zero time.
//
// It lives here because the CLI and the MCP server each had their own copy,
// and the copies disagreed: the CLI parsed with fmt.Sscanf, which stops at the
// first non-digit and reports no error, so `--since 7xd` silently meant seven
// days there while the identical string was rejected over MCP. A value a person
// types by hand should not mean two different things depending on which surface
// received it, and a malformed one should be refused rather than guessed at.
//
// now is injected so callers can be deterministic in tests.
func ParseSince(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	if n, ok := strings.CutSuffix(value, "d"); ok {
		days, err := positiveInt(n)
		if err != nil {
			return time.Time{}, invalidSince(value)
		}
		return now.AddDate(0, 0, -days), nil
	}

	if n, ok := strings.CutSuffix(value, "w"); ok {
		weeks, err := positiveInt(n)
		if err != nil {
			return time.Time{}, invalidSince(value)
		}
		return now.AddDate(0, 0, -weeks*7), nil
	}

	parsed, err := time.Parse(sinceDateLayout, value)
	if err != nil {
		return time.Time{}, invalidSince(value)
	}
	return parsed, nil
}

// positiveInt accepts only a whole positive number, with no trailing
// characters — strconv.Atoi rather than fmt.Sscanf for exactly that reason.
func positiveInt(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return n, nil
}

func invalidSince(value string) error {
	return fmt.Errorf("invalid since %q: expected a form like 7d, 2w, or 2026-07-01", value)
}
