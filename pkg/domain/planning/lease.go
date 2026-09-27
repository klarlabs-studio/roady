package planning

import (
	"strings"
	"time"
)

// Lease is a claim on an in-progress task that has to be kept alive. Starting
// a task takes one; activity by the holder renews it (roady next, a check, an
// explicit renew). A holder that crashes or walks away stops renewing, the
// lease runs out, and the task goes back to pending for someone else instead
// of staying "in progress" forever.
type Lease struct {
	Holder    string    `json:"holder"`
	Session   string    `json:"session,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
	RenewedAt time.Time `json:"renewed_at"`
}

// DefaultLeaseTTL is how long a claim lasts without renewal when policy does
// not say otherwise. The brief renews it at session start and after
// compaction, and the write-guard hook each time the agent writes a file, so
// an agent at work keeps its claim indefinitely.
const DefaultLeaseTTL = 2 * time.Hour

// Expired reports whether the lease has run out at now.
func (l *Lease) Expired(now time.Time) bool {
	return l != nil && !now.Before(l.ExpiresAt)
}

// HeldBy reports whether holder, in session, holds the lease. Holders
// compare case-insensitively, as owners do elsewhere. Sessions tell apart
// agents that share a name ("ai-agent"): they must match when both are
// known, and are ignored when either is not.
func (l *Lease) HeldBy(holder, session string) bool {
	if l == nil || !strings.EqualFold(strings.TrimSpace(l.Holder), strings.TrimSpace(holder)) {
		return false
	}
	return l.Session == "" || session == "" || l.Session == session
}

// Claim puts a lease on taskID for holder. A ttl of zero or less takes no
// lease: the claim then lasts until the task moves on, as before leases.
func (s *ExecutionState) Claim(taskID, holder, session string, ttl time.Duration, now time.Time) {
	result := s.TaskStates[taskID]
	if ttl <= 0 {
		result.Lease = nil
	} else {
		result.Lease = &Lease{Holder: holder, Session: session, ExpiresAt: now.Add(ttl), RenewedAt: now}
	}
	s.TaskStates[taskID] = result
}

// RenewClaim extends holder's unexpired lease on taskID and reports whether
// it did. It never revives an expired lease or renews someone else's.
func (s *ExecutionState) RenewClaim(taskID, holder, session string, ttl time.Duration, now time.Time) bool {
	result, ok := s.TaskStates[taskID]
	if !ok || result.Status != StatusInProgress || result.Lease == nil || ttl <= 0 {
		return false
	}
	if !result.Lease.HeldBy(holder, session) || result.Lease.Expired(now) {
		return false
	}
	result.Lease.ExpiresAt = now.Add(ttl)
	result.Lease.RenewedAt = now
	s.TaskStates[taskID] = result
	return true
}

// ExpiredClaims lists in-progress tasks whose lease has run out.
func (s *ExecutionState) ExpiredClaims(now time.Time) []string {
	var ids []string
	for id, r := range s.TaskStates {
		if r.Status == StatusInProgress && r.Lease.Expired(now) {
			ids = append(ids, id)
		}
	}
	return ids
}

// ReleaseClaim returns an in-progress task to pending, unowned and without
// a lease. Evidence and check history stay.
func (s *ExecutionState) ReleaseClaim(taskID string) {
	result, ok := s.TaskStates[taskID]
	if !ok {
		return
	}
	result.Status = StatusPending
	result.Owner = ""
	result.Lease = nil
	s.TaskStates[taskID] = result
	s.UpdatedAt = time.Now()
}
