package audit

import (
	"testing"
	"time"
)

func TestEntryFromShowsCheckVerdict(t *testing.T) {
	tests := []struct {
		meta map[string]any
		want string
	}{
		{map[string]any{"task_id": "t", "kind": "run", "passed": true}, "passed"},
		{map[string]any{"task_id": "t", "kind": "run", "passed": false}, "failed"},
		{map[string]any{"task_id": "t", "kind": "manual", "passed": true}, "confirmed"},
	}
	for _, tc := range tests {
		if got := EntryFrom(time.Now(), "task.check", "dev", "h", tc.meta).Detail; got != tc.want {
			t.Errorf("Detail = %q, want %q", got, tc.want)
		}
	}
}
