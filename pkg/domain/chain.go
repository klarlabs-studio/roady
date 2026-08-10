package domain

import "fmt"

// ChainEntry is one link of the audit log, reduced to the terms verification
// needs.
//
// It exists so the two writers to events.jsonl are checked by one
// implementation. They previously had their own: AuditService verified the log
// as a hash-linked graph, while EventSourcedAuditService still required each
// entry to follow the previous line, so the same file could be pronounced
// intact by one and tampered-with by the other at the same moment. An audit
// chain whose verdict depends on which code path asked is not evidence of
// anything.
//
// Hashing stays with each event type, since they hash different fields.
// What is shared is what the links mean.
type ChainEntry struct {
	// ID identifies the entry in a finding. Empty is itself a signal.
	ID string
	// Hash is the entry's recorded content hash.
	Hash string
	// PrevHash names the entry this one was appended after. Empty marks a
	// root, which a fresh log and each independently-started branch have.
	PrevHash string
	// HashAlgo names the algorithm Hash was written with.
	HashAlgo string
	// Verifiable reports whether this build can check the entry at all.
	Verifiable bool
	// Matches reports whether the recorded hash reproduces from the content.
	Matches bool
}

// ViolationKind names why an entry failed, so a caller can count the kinds
// apart instead of re-deriving them from the message text.
//
// The distinction that carries weight is KindHashMismatch against
// KindLegacyUnverifiable. Both are entries whose hash does not reproduce, but
// only the first is written with an algorithm this build knows, which makes it
// the only one that can mean the content was altered. The second predates
// hash_algo and cannot be checked either way. Reporting a count of both
// together is what turns a real finding into one line among dozens nobody
// reads.
type ViolationKind int

const (
	KindDuplicate ViolationKind = iota
	KindUnhashed
	KindUnknownAlgo
	KindHashMismatch
	KindLegacyUnverifiable
	KindMissingParent
)

// ChainViolation is one finding, with the reason kept separate from the prose
// so callers do not match on message text.
type ChainViolation struct {
	Kind    ViolationKind
	Index   int
	ID      string
	Message string
}

// VerifyChain reports every way the log fails to hold together, as messages.
//
// It is a thin formatting layer over VerifyChainDetailed so that both share one
// implementation, for the same reason the two writers to events.jsonl do.
func VerifyChain(entries []ChainEntry) []string {
	detailed := VerifyChainDetailed(entries)
	if len(detailed) == 0 {
		return nil
	}
	out := make([]string, 0, len(detailed))
	for i := range detailed {
		out = append(out, detailed[i].Message)
	}
	return out
}

// VerifyChainDetailed reports every way the log fails to hold together.
//
// The log is verified as a hash-linked graph rather than a strict sequence.
// Two collaborators appending concurrently produce branches from a shared
// parent, and git union-merges them in whatever order the timestamps fall;
// requiring each entry to follow the previous line rejected every such merge,
// which is what made concurrent work impossible.
//
// Nothing is given up by relaxing order, because an entry's hash covers its
// PrevHash: altering content or reparenting an entry breaks that entry's own
// hash. What the links still prove is that no entry which something else
// references has been removed.
func VerifyChainDetailed(entries []ChainEntry) []ChainViolation {
	var violations []ChainViolation

	add := func(kind ViolationKind, i int, id, format string, args ...any) {
		violations = append(violations, ChainViolation{
			Kind:    kind,
			Index:   i,
			ID:      id,
			Message: fmt.Sprintf(format, args...),
		})
	}

	present := make(map[string]bool, len(entries))
	seenIDs := make(map[string]int, len(entries))
	for i := range entries {
		present[entries[i].Hash] = true
		if first, dup := seenIDs[entries[i].ID]; dup && entries[i].ID != "" {
			add(KindDuplicate, i, entries[i].ID,
				"Event %d (%s): duplicate of event %d. The log contains the same event twice.",
				i, entries[i].ID, first)
			continue
		}
		if entries[i].ID != "" {
			seenIDs[entries[i].ID] = i
		}
	}

	for i := range entries {
		e := entries[i]

		// An entry with no hash was never in the chain. That is a different
		// fact from a hash that does not match, and conflating them reads as
		// tampering when the cause is usually something appending to the log
		// without going through Roady.
		if e.Hash == "" {
			add(KindUnhashed, i, e.ID,
				"Event %d (%s): recorded without a hash, so it is outside the chain. Something appended to events.jsonl directly instead of through roady.",
				i, orUnidentified(e.ID))
			continue
		}

		// An entry stamped with an algorithm this build does not know cannot
		// be checked. Report that plainly instead of calling it tampering: a
		// false accusation is how a security signal gets ignored.
		if !e.Verifiable {
			add(KindUnknownAlgo, i, e.ID,
				"Event %d (%s): written with hash algorithm %q, which this build cannot verify. Upgrade roady or treat this entry as unverified.",
				i, e.ID, e.HashAlgo)
			continue
		}

		// Self-hash. Covers content and parentage together.
		if !e.Matches {
			// An entry that names no algorithm predates hash_algo, so its hash
			// was written by a function this build no longer has. It cannot be
			// confirmed and it cannot be convicted. Kept as a violation, since
			// unverifiable history is a real gap, but counted apart: an entry
			// that names a known algorithm and still fails to reproduce is the
			// only one of the two that can mean tampering, and it has to stay
			// findable rather than sit in a pile of entries that merely got old.
			if e.HashAlgo == "" {
				add(KindLegacyUnverifiable, i, e.ID,
					"Event %d (%s): predates the hash_algo field, so the algorithm that wrote its hash is unknown and the entry cannot be verified either way (see docs/audit-grc.md).",
					i, e.ID)
				continue
			}
			add(KindHashMismatch, i, e.ID,
				"Event %d (%s): content hash does not reproduce under %q, the algorithm it names. The entry was altered after it was written (see docs/audit-grc.md).",
				i, e.ID, e.HashAlgo)
			continue
		}

		// Parent resolution.
		if e.PrevHash != "" && !present[e.PrevHash] {
			add(KindMissingParent, i, e.ID,
				"Event %d (%s): missing parent %s. An earlier event has been removed.",
				i, e.ID, shortHash(e.PrevHash))
		}
	}

	return violations
}

// orUnidentified names an entry that arrived without an ID, which is itself a
// sign it did not come from Roady.
func orUnidentified(id string) string {
	if id == "" {
		return "no id"
	}
	return id
}

// shortHash trims a hash for a human-readable finding while staying long
// enough to identify the entry.
func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12]
}
