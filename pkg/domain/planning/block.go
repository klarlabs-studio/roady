package planning

import (
	"fmt"
	"strings"
	"time"
)

// Block records why a task is blocked. Two reasons are an agent's honest
// way out of work it cannot do as specified: rather than bending the tests
// or declaring done, it says so, and a person decides. Offering that exit
// is what keeps agents from gaming impossible tasks (in ImpossibleBench an
// explicit abort cut test manipulation from 54% to 9%).
type Block struct {
	// Kind is spec-conflict, cannot-complete, or empty for an ordinary
	// block (waiting on something).
	Kind   BlockKind `json:"kind,omitempty"`
	Detail string    `json:"detail,omitempty"`
	By     string    `json:"by,omitempty"`
	At     time.Time `json:"at"`
}

// BlockKind names why a task is blocked.
type BlockKind string

const (
	// BlockSpecConflict: the requirement contradicts itself, another
	// requirement, or the code it must live with.
	BlockSpecConflict BlockKind = "spec-conflict"
	// BlockCannotComplete: the task cannot be done as specified — missing
	// access, an impossible check, a dependency that does not exist.
	BlockCannotComplete BlockKind = "cannot-complete"
)

// NeedsDecision reports whether the block is one only a person can resolve.
func (k BlockKind) NeedsDecision() bool {
	return k == BlockSpecConflict || k == BlockCannotComplete
}

// ParseBlockKind accepts a block reason; empty is an ordinary block.
func ParseBlockKind(s string) (BlockKind, error) {
	k := BlockKind(strings.ToLower(strings.TrimSpace(strings.ReplaceAll(s, "_", "-"))))
	switch k {
	case "", BlockSpecConflict, BlockCannotComplete:
		return k, nil
	}
	return "", fmt.Errorf("unknown block reason %q: use spec-conflict or cannot-complete, or none for an ordinary block", s)
}

// NeedsDecision lists blocked tasks only a person can unblock, by task ID.
func (s *ExecutionState) NeedsDecision() map[string]Block {
	out := map[string]Block{}
	for id, r := range s.TaskStates {
		if r.Status == StatusBlocked && r.Block != nil && r.Block.Kind.NeedsDecision() {
			out[id] = *r.Block
		}
	}
	return out
}
