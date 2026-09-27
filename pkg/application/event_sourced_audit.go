// Package application provides application services.
package application

import (
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
	"github.com/google/uuid"
)

// EventSourcedAuditService implements AuditLogger using the event store.
// It bridges the existing audit interface with the new event sourcing system.
type EventSourcedAuditService struct {
	store events.EventStore

	// prov identifies the agent and session behind every event this service
	// records. Stamping it in Log rather than at each call site means no
	// event can be written without provenance by forgetting to pass it.
	prov provenance.Context
}

// SetProvenance sets the identity stamped onto subsequently recorded events.
func (s *EventSourcedAuditService) SetProvenance(ctx provenance.Context) {
	s.prov = ctx
}

// Provenance returns the identity currently being stamped.
func (s *EventSourcedAuditService) Provenance() provenance.Context {
	return s.prov
}

// Compile-time check that EventSourcedAuditService implements AuditLogger.
var _ domain.AuditLogger = (*EventSourcedAuditService)(nil)

// NewEventSourcedAuditService creates an audit service over store.
func NewEventSourcedAuditService(store events.EventStore) *EventSourcedAuditService {
	return &EventSourcedAuditService{store: store}
}

// Log implements domain.AuditLogger.
func (s *EventSourcedAuditService) Log(action string, actor string, metadata map[string]any) error {
	event := &events.BaseEvent{
		ID:        uuid.New().String(),
		Type:      action,
		Timestamp: time.Now(),
		Actor:     actor,
		Metadata:  s.prov.Apply(metadata),
	}

	// Extract aggregate info from metadata if available
	if taskID, ok := metadata["task_id"].(string); ok {
		event.AggregateID_ = taskID
		event.AggregateType_ = events.AggregateTypeTask
	} else if planID, ok := metadata["plan_id"].(string); ok {
		event.AggregateID_ = planID
		event.AggregateType_ = events.AggregateTypePlan
	}

	if err := s.store.Append(event); err != nil {
		return err
	}

	return nil
}

// VerifyIntegrity checks the audit chain.
//
// It delegates to domain.VerifyChain, the same implementation AuditService
// uses. This function previously carried its own, which required each entry to
// follow the previous line and so reported tampering for the branch-and-merge
// shape concurrent appends legitimately produce — the case AuditService was
// fixed for in 0.14.0 and this copy never received. The same events.jsonl
// could be pronounced intact by one service and tampered-with by the other at
// the same moment, which makes the verdict evidence of nothing.
func (s *EventSourcedAuditService) VerifyIntegrity() ([]string, error) {
	evts, err := s.store.LoadAll()
	if err != nil {
		return nil, err
	}

	entries := make([]domain.ChainEntry, 0, len(evts))
	for _, e := range evts {
		if e == nil {
			continue
		}
		entries = append(entries, domain.ChainEntry{
			ID:         e.ID,
			Hash:       e.Hash,
			PrevHash:   e.PrevHash,
			HashAlgo:   e.HashAlgo,
			Verifiable: e.Verifiable(),
			Matches:    e.HashMatches(),
		})
	}

	return domain.VerifyChain(entries), nil
}

// LoadEvents returns all events from the store.
func (s *EventSourcedAuditService) LoadEvents() ([]*events.BaseEvent, error) {
	return s.store.LoadAll()
}

// LoadEventsSince returns events since the given time.
func (s *EventSourcedAuditService) LoadEventsSince(since time.Time) ([]*events.BaseEvent, error) {
	return s.store.LoadSince(since)
}
