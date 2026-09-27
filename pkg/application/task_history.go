package application

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
)

// A task's history is read back from the event log: every capture records
// the fields it changed (or the task it created), every transition and check
// its own event. Nothing is rewritten, so how a task came to look the way it
// does — split, moved, re-estimated, blocked, reopened — stays visible.

// HistoryEntry is one thing that happened to a task.
type HistoryEntry struct {
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Action string    `json:"action"`
	What   string    `json:"what"`
}

// historySource is what TaskHistory needs from a repository.
type historySource interface {
	LoadEvents() ([]domain.Event, error)
}

// TaskHistory lists what happened to taskID, oldest first.
func (s *TaskService) TaskHistory(taskID string) ([]HistoryEntry, error) {
	src, ok := s.repo.(historySource)
	if !ok {
		return nil, fmt.Errorf("this repository keeps no event log")
	}
	events, err := src.LoadEvents()
	if err != nil {
		return nil, err
	}
	item := "task:" + taskID
	var out []HistoryEntry
	for _, e := range events {
		var what string
		switch {
		case e.Action == "plan.capture":
			what = captureHistory(e.Metadata, item)
		case metaString(e.Metadata, "task_id") == taskID:
			what = eventHistory(e)
		}
		if what != "" {
			out = append(out, HistoryEntry{At: e.Timestamp, Actor: e.Actor, Action: e.Action, What: what})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	// One transition can be logged twice — the transition itself and the
	// coordinator's own event, which carries less. A reader wants it once,
	// with the detail.
	deduped := out[:0]
	for _, e := range out {
		if n := len(deduped); n > 0 && e.At.Sub(deduped[n-1].At) < 5*time.Second && sameStep(deduped[n-1], e) {
			if e.Action == "task.transition" || len(e.What) > len(deduped[n-1].What) {
				deduped[n-1] = e
			}
			continue
		}
		deduped = append(deduped, e)
	}
	return deduped, nil
}

func captureHistory(meta map[string]any, item string) string {
	changes, _ := meta["changes"].([]any)
	note := metaString(meta, "note")
	for _, c := range changes {
		m, _ := c.(map[string]any)
		if m["item"] != item {
			continue
		}
		var what string
		switch m["change"] {
		case "created":
			as, _ := m["as"].(map[string]any)
			what = "created: " + describeTask(as)
		case "updated":
			fields, _ := m["fields"].(map[string]any)
			what = "edited: " + describeFields(fields)
		}
		if note != "" && !strings.HasPrefix(note, "Task ") {
			what += " (" + note + ")"
		}
		return what
	}
	return ""
}

func describeTask(t map[string]any) string {
	parts := []string{fmt.Sprintf("%q", metaString(t, "title"))}
	if f := metaString(t, "feature_id"); f != "" {
		parts = append(parts, "feature "+f)
	} else {
		parts = append(parts, "unplanned")
	}
	if g := metaString(t, "goal"); g != "" {
		parts = append(parts, "goal "+g)
	}
	if deps := listOf(t["depends_on"]); len(deps) > 0 {
		parts = append(parts, "after "+strings.Join(deps, ", "))
	}
	if e := metaString(t, "estimate"); e != "" {
		parts = append(parts, "estimate "+e)
	}
	return strings.Join(parts, ", ")
}

func describeFields(fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		fc, _ := fields[k].(map[string]any)
		parts = append(parts, fmt.Sprintf("%s %s → %s", strings.ReplaceAll(k, "_", " "), showValue(fc["from"]), showValue(fc["to"])))
	}
	return strings.Join(parts, "; ")
}

func eventHistory(e domain.Event) string {
	m := e.Metadata
	switch e.Action {
	case "task.transition":
		what := metaString(m, "event")
		if kind := metaString(m, "kind"); kind != "" {
			what += " (" + kind + ")"
		}
		for _, k := range []string{"evidence", "reason"} {
			if v := metaString(m, k); v != "" {
				what += ": " + v
				break
			}
		}
		return what
	case "task.started":
		return "start"
	case "task.completed":
		return "complete"
	case "task.blocked":
		return "block"
	case "task.unblocked":
		return "unblock"
	case "task.check":
		verdict := "failed"
		if b, _ := m["passed"].(bool); b {
			verdict = "passed"
		}
		return fmt.Sprintf("check %s at %s", verdict, shortCommit(metaString(m, "commit")))
	case "task.claim_expired":
		return "claim by " + metaString(m, "holder") + " expired; back to pending"
	case "task.regression":
		return "regression: its check fails at " + shortCommit(metaString(m, "commit"))
	}
	return strings.TrimPrefix(e.Action, "task.")
}

func metaString(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func listOf(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, i := range items {
		if s, ok := i.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func showValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "(none)"
	case string:
		if x == "" {
			return "(none)"
		}
		return fmt.Sprintf("%q", x)
	case []any:
		if len(x) == 0 {
			return "(none)"
		}
		return "[" + strings.Join(listOf(x), ", ") + "]"
	case map[string]any:
		if run := metaString(x, "run"); run != "" {
			return "run " + fmt.Sprintf("%q", run)
		}
		if manual := metaString(x, "manual"); manual != "" {
			return "manual " + fmt.Sprintf("%q", manual)
		}
	}
	return fmt.Sprint(v)
}

// RenderHistory formats a history for the terminal.
func RenderHistory(taskID string, entries []HistoryEntry) string {
	if len(entries) == 0 {
		return fmt.Sprintf("No recorded history for %s.\n", taskID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "History of %s\n", taskID)
	for _, e := range entries {
		fmt.Fprintf(&b, "%s  %-12s %s\n", e.At.Local().Format("2006-01-02 15:04"), e.Actor, e.What)
	}
	return b.String()
}

// sameStep reports whether two entries record one transition: the same
// verb, one of them from the coordinator's terse event.
func sameStep(a, b HistoryEntry) bool {
	verb := func(e HistoryEntry) string { return strings.TrimRight(strings.Fields(e.What + " ")[0], ":") }
	terse := func(e HistoryEntry) bool { return e.Action != "task.transition" }
	return verb(a) == verb(b) && (terse(a) || terse(b)) && !strings.HasPrefix(a.What, "created") && !strings.HasPrefix(a.What, "edited")
}
