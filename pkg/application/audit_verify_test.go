package application

import (
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
)

// chainRepo serves a fixed event log.
type chainRepo struct {
	domainWorkspaceRepo
	events []domain.Event
	err    error
}

func (r *chainRepo) LoadEvents() ([]domain.Event, error) { return r.events, r.err }

// linked builds a correctly hash-chained run of events, each parented to the
// previous one.
func linked(actor string, n int, parent string) []domain.Event {
	var out []domain.Event
	prev := parent
	for i := range n {
		e := domain.Event{
			ID:        actor + "-" + string(rune('a'+i)),
			Timestamp: time.Date(2026, 8, 1, 9, i, 0, 0, time.UTC),
			Action:    "task.transition",
			Actor:     actor,
			PrevHash:  prev,
		}
		e.Hash = e.CalculateHash()
		out = append(out, e)
		prev = e.Hash
	}
	return out
}

func verify(t *testing.T, events []domain.Event) []string {
	t.Helper()
	svc := NewAuditService(&chainRepo{events: events})
	problems, err := svc.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity: %v", err)
	}
	return problems
}

func TestVerifyIntegrityAcceptsLinearLog(t *testing.T) {
	// The pre-existing single-writer shape must keep verifying unchanged.
	if p := verify(t, linked("alice", 5, "")); len(p) != 0 {
		t.Errorf("a valid linear chain should verify cleanly, got %v", p)
	}
}

func TestVerifyIntegrityAcceptsMergedBranches(t *testing.T) {
	// Two collaborators append concurrently from a shared parent, then the
	// branches are union-merged by git. Strict sequential linkage rejected
	// this, which is what made concurrent work impossible.
	base := linked("base", 2, "")
	head := base[len(base)-1].Hash

	alice := linked("alice", 3, head)
	bob := linked("bob", 2, head)

	merged := append(append(append([]domain.Event{}, base...), alice...), bob...)

	if p := verify(t, merged); len(p) != 0 {
		t.Errorf("a union-merged log should verify, got %v", p)
	}
}

func TestVerifyIntegrityAcceptsInterleavedOrder(t *testing.T) {
	// git may interleave lines from both branches by timestamp. Events must
	// verify by reference, not by adjacency.
	base := linked("base", 1, "")
	head := base[0].Hash
	alice := linked("alice", 2, head)
	bob := linked("bob", 2, head)

	interleaved := []domain.Event{base[0], alice[0], bob[0], alice[1], bob[1]}

	if p := verify(t, interleaved); len(p) != 0 {
		t.Errorf("interleaved branches should verify, got %v", p)
	}
}

func TestVerifyIntegrityStillDetectsContentTampering(t *testing.T) {
	events := linked("alice", 3, "")
	events[1].Actor = "someone-else"

	problems := verify(t, events)

	if len(problems) == 0 {
		t.Fatal("altering an event must be detected")
	}
	if !strings.Contains(strings.Join(problems, " "), "Content hash mismatch") {
		t.Errorf("expected a content hash finding, got %v", problems)
	}
}

func TestVerifyIntegrityStillDetectsDeletion(t *testing.T) {
	events := linked("alice", 4, "")
	// Remove a middle event; its child's PrevHash now dangles.
	events = append(events[:1], events[2:]...)

	problems := verify(t, events)

	if len(problems) == 0 {
		t.Fatal("removing a referenced event must be detected")
	}
	if !strings.Contains(strings.Join(problems, " "), "missing parent") {
		t.Errorf("expected a dangling-parent finding, got %v", problems)
	}
}

func TestVerifyIntegrityDetectsReparenting(t *testing.T) {
	events := linked("alice", 3, "")
	// Point the last event at the first, without recomputing its hash.
	events[2].PrevHash = events[0].Hash

	problems := verify(t, events)

	// PrevHash is inside CalculateHash, so reparenting breaks the self-hash.
	if len(problems) == 0 {
		t.Fatal("reparenting must be detected")
	}
}

func TestVerifyIntegrityDetectsDuplicateIDs(t *testing.T) {
	events := linked("alice", 2, "")
	events = append(events, events[0])

	problems := verify(t, events)

	if !strings.Contains(strings.Join(problems, " "), "duplicate") {
		t.Errorf("expected a duplicate-event finding, got %v", problems)
	}
}

func TestVerifyIntegrityEmptyLog(t *testing.T) {
	if p := verify(t, nil); len(p) != 0 {
		t.Errorf("an empty log has nothing to violate, got %v", p)
	}
}
