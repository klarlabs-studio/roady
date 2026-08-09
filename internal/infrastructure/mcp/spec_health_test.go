package mcp

import (
	"errors"
	"strings"
	"testing"
)

// Tools must not disagree about whether the project is broken. roady_status
// reads plan.json and state.json and never unmarshals the spec, so it answered
// normally while spec.yaml held merge-conflict markers — the server looked
// healthy while exactly one tool looked flaky, and the reporter concluded
// roady was broken rather than their file (#87, item 5).
func TestWithSpecWarning(t *testing.T) {
	tests := []struct {
		name   string
		answer string
		err    error
		want   []string
		absent []string
	}{
		{
			name:   "healthy spec leaves the answer untouched",
			answer: "Tasks: 3 total",
			err:    nil,
			want:   []string{"Tasks: 3 total"},
			absent: []string{"WARNING", "Cause:"},
		},
		{
			name:   "unparseable spec is announced, and the answer still returned",
			answer: "Tasks: 3 total",
			err:    errors.New("load spec: failed to unmarshal spec: yaml: line 11: could not find expected ':'"),
			// The answer survives: status computed from plan.json is useful even
			// when the spec is broken. What must not happen is returning it as
			// though nothing were wrong.
			want: []string{
				"WARNING",
				"does not currently parse",
				"roady_spec_validate",
				"yaml: line 11",
				"Tasks: 3 total",
			},
		},
		{
			name:   "the remedy is named, not just the symptom",
			answer: "No plan found. Run roady_plan_generate first.",
			err:    errors.New("load spec: bad yaml"),
			// "generate a plan" is bad advice when the spec cannot be read,
			// because generating one reads it and fails too.
			want: []string{"WARNING", "roady_spec_validate", "No plan found"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withSpecWarning(tc.answer, tc.err)
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("output = %q, want it to contain %q", got, w)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(got, a) {
					t.Errorf("output = %q, want it NOT to contain %q", got, a)
				}
			}
		})
	}
}

// specHealth must not invent a failure when there are no services to ask.
// Reporting "spec is broken" for a project that was never loaded would be the
// same class of error in the opposite direction.
func TestSpecHealthWithoutServices(t *testing.T) {
	if err := specHealth(nil); err != nil {
		t.Errorf("specHealth(nil) = %v, want nil", err)
	}
}
