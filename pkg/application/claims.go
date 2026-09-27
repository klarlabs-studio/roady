package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
)

// Claims keep parallel agents off each other's work and stop a crashed
// agent from holding a task forever. Starting a task takes a lease; the
// holder's activity renews it; an expired lease releases the task back to
// pending, recorded in the audit log.

// claimRetries bounds how often a start is retried after another process
// changed the state between load and save.
const claimRetries = 5

// leaseTTL is the claim length the project's policy asks for.
func (s *TaskService) leaseTTL() time.Duration {
	cfg, err := s.repo.LoadPolicy()
	if err != nil {
		return planning.DefaultLeaseTTL
	}
	return cfg.LeaseTTL(planning.DefaultLeaseTTL)
}

// session is the session the audit service stamps on events, if any.
func (s *TaskService) session() string {
	if p, ok := s.audit.(interface{ Provenance() provenance.Context }); ok {
		return p.Provenance().SessionID
	}
	return ""
}

// claimTask starts taskID for owner under a lease. It retries when another
// process saved the state in between: the retry sees that process's claim
// and refuses, so two agents never both hold one task.
func (s *TaskService) claimTask(taskID, owner string) (*planning.Lease, error) {
	opt := project.ClaimOptions{Session: s.session(), TTL: s.leaseTTL()}
	var lastErr error
	for i := 0; i < claimRetries; i++ {
		replaced, err := s.coordinator.ClaimTask(context.Background(), taskID, owner, opt)
		var conflict *planning.ConflictError
		if !errors.As(err, &conflict) {
			return replaced, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// RenewClaim extends owner's claim on taskID. It fails when the task is
// not in progress under owner's live claim, so a holder learns it lost the
// task instead of carrying on unaware.
func (s *TaskService) RenewClaim(taskID, owner string) (*planning.Lease, error) {
	ttl := s.leaseTTL()
	for i := 0; i < claimRetries; i++ {
		state, err := s.repo.LoadState()
		if err != nil {
			return nil, err
		}
		result, _ := state.GetTaskResult(taskID)
		switch {
		case result.Status != planning.StatusInProgress:
			return nil, fmt.Errorf("task %s is %s, not in progress; start it to claim it", taskID, orPending(result.Status))
		case result.Lease == nil:
			return nil, fmt.Errorf("task %s has no claim to renew (claims are off, or it was started before they existed)", taskID)
		case !result.Lease.HeldBy(owner, s.session()):
			return nil, fmt.Errorf("task %s is claimed by %s, not %s", taskID, result.Lease.Holder, owner)
		}
		if !state.RenewClaim(taskID, owner, s.session(), ttl, time.Now()) {
			return nil, fmt.Errorf("your claim on %s expired at %s; start it again to reclaim it", taskID, result.Lease.ExpiresAt.Format(time.RFC3339))
		}
		err = s.repo.SaveState(state)
		var conflict *planning.ConflictError
		if errors.As(err, &conflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		lease := state.TaskStates[taskID].Lease
		return lease, nil
	}
	return nil, fmt.Errorf("could not renew the claim on %s: the state kept changing", taskID)
}

// Heartbeat renews the claims owner holds that are past half their length.
// It is cheap when there is nothing to renew: one read, no write.
func (s *TaskService) Heartbeat(owner string) { s.renewHeld(owner) }

// renewHeld renews every live claim owner holds. It is the heartbeat behind
// `roady next`: hooks run it at session start, after compaction and every
// few steps, so an agent at work keeps its claims without thinking about it.
func (s *TaskService) renewHeld(owner string) {
	if owner == "" {
		return
	}
	ttl, session := s.leaseTTL(), s.session()
	for i := 0; i < claimRetries; i++ {
		state, err := s.repo.LoadState()
		if err != nil || state == nil {
			return
		}
		now, renewed := time.Now(), false
		for id := range state.TaskStates {
			// Renewing on every brief would rewrite state.json each time;
			// once the lease is past half its length is often enough.
			r := state.TaskStates[id]
			if r.Lease != nil && r.Lease.ExpiresAt.Sub(now) > ttl/2 {
				continue
			}
			renewed = state.RenewClaim(id, owner, session, ttl, now) || renewed
		}
		if !renewed {
			return
		}
		var conflict *planning.ConflictError
		if err := s.repo.SaveState(state); !errors.As(err, &conflict) {
			return
		}
	}
}

// ReleaseExpiredClaims returns every task whose claim has run out to
// pending and records each release. It returns the released task IDs.
func (s *TaskService) ReleaseExpiredClaims(actor string) ([]string, error) {
	for i := 0; i < claimRetries; i++ {
		state, err := s.repo.LoadState()
		if err != nil || state == nil {
			return nil, err
		}
		now := time.Now()
		ids := state.ExpiredClaims(now)
		if len(ids) == 0 {
			return nil, nil
		}
		sort.Strings(ids)
		leases := map[string]planning.Lease{}
		for _, id := range ids {
			leases[id] = *state.TaskStates[id].Lease
			state.ReleaseClaim(id)
		}
		err = s.repo.SaveState(state)
		var conflict *planning.ConflictError
		if errors.As(err, &conflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			s.logClaimExpired(id, leases[id], actor)
		}
		return ids, nil
	}
	return nil, nil
}

func (s *TaskService) logClaimExpired(taskID string, l planning.Lease, actor string) {
	if s.audit == nil {
		return
	}
	if actor == "" {
		actor = "roady"
	}
	_ = s.audit.Log("task.claim_expired", actor, map[string]interface{}{
		"task_id":    taskID,
		"holder":     l.Holder,
		"session":    l.Session,
		"expired_at": l.ExpiresAt.Format(time.RFC3339),
	})
}

func orPending(st planning.TaskStatus) planning.TaskStatus {
	if st == "" {
		return planning.StatusPending
	}
	return st
}
