package planning

import (
	"strings"
	"testing"
)

func TestCheckValidate(t *testing.T) {
	tests := []struct {
		name  string
		check *Check
		ok    bool
		kind  string
	}{
		{"run", &Check{Run: "go test ./..."}, true, CheckKindRun},
		{"manual", &Check{Manual: "Looks right in the browser"}, true, CheckKindManual},
		{"both", &Check{Run: "x", Manual: "y"}, false, CheckKindRun},
		{"blank", &Check{Run: "  "}, false, CheckKindManual},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.check.Validate(); (err == nil) != tc.ok {
				t.Errorf("Validate() = %v, want ok=%v", err, tc.ok)
			}
			if tc.ok && tc.check.Kind() != tc.kind {
				t.Errorf("Kind() = %q, want %q", tc.check.Kind(), tc.kind)
			}
		})
	}
	var none *Check
	if !none.IsZero() || none.Validate() != nil {
		t.Error("a nil check is simply absent")
	}
}

func TestTailOutputBoundsLength(t *testing.T) {
	long := strings.Repeat("a", MaxCheckOutput) + "THE-END"
	got := TailOutput([]byte(long))
	if !strings.HasSuffix(got, "THE-END") || len(got) > MaxCheckOutput+len("…") {
		t.Errorf("tail not kept or not bounded: len=%d", len(got))
	}
}
