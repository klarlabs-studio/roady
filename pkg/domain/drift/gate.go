package drift

import (
	"fmt"
	"strings"
)

// severityRank orders severities for threshold comparison. Lower is worse.
// An unrecognised severity ranks at the bottom rather than being discarded:
// a typo in a severity string must not quietly remove an issue from every
// gate.
func severityRank(s Severity) int {
	switch Severity(strings.ToLower(strings.TrimSpace(string(s)))) {
	case SeverityCritical:
		return 0
	case SeverityHigh:
		return 1
	case SeverityMedium:
		return 2
	default:
		return 3
	}
}

// ParseSeverity reads a severity threshold supplied by a caller.
func ParseSeverity(raw string) (Severity, error) {
	switch Severity(strings.ToLower(strings.TrimSpace(raw))) {
	case SeverityLow:
		return SeverityLow, nil
	case SeverityMedium:
		return SeverityMedium, nil
	case SeverityHigh:
		return SeverityHigh, nil
	case SeverityCritical:
		return SeverityCritical, nil
	default:
		return "", fmt.Errorf("unknown severity %q: use low, medium, high, or critical", raw)
	}
}

// AtOrAbove returns the issues at least as severe as the threshold.
//
// This is what makes drift usable as a CI gate. Failing a build on any drift
// at all means a low-severity note blocks a merge, and a gate that blocks on
// noise gets switched off.
func (r *Report) AtOrAbove(threshold Severity) []Issue {
	if r == nil {
		return nil
	}

	limit := severityRank(threshold)
	var matched []Issue
	for _, issue := range r.Issues {
		if severityRank(issue.Severity) <= limit {
			matched = append(matched, issue)
		}
	}
	return matched
}
