package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// NeedsDecision is a task an agent could not do as specified, waiting for a
// person: shown in status, the brief and drift until someone resolves it.
type NeedsDecision struct {
	TaskID string             `json:"task_id"`
	Kind   planning.BlockKind `json:"kind"`
	Detail string             `json:"detail,omitempty"`
	By     string             `json:"by,omitempty"`
}

// NeedsDecisions lists them in task ID order.
func NeedsDecisions(state *planning.ExecutionState) []NeedsDecision {
	if state == nil {
		return nil
	}
	var out []NeedsDecision
	for id, b := range state.NeedsDecision() {
		out = append(out, NeedsDecision{TaskID: id, Kind: b.Kind, Detail: b.Detail, By: b.By})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	return out
}

// RenderNeedsDecisions formats them for a terminal or an agent; empty when
// there are none.
func RenderNeedsDecisions(items []NeedsDecision) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Needs a decision (%d):\n", len(items))
	for _, it := range items {
		line := fmt.Sprintf("- %s [%s]", it.TaskID, it.Kind)
		if it.Detail != "" {
			line += " " + it.Detail
		}
		if it.By != "" {
			line += " — " + it.By
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
