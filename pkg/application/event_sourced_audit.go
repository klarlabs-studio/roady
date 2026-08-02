// Package application provides application services.
package application

import (
	"context"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/events"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
	"github.com/google/uuid"
)

// EventSourcedAuditService implements AuditLogger using the event store.
// It bridges the existing audit interface with the new event sourcing system.
type EventSourcedAuditService struct {
	store      events.EventStore
	publisher  events.EventPublisher
	dispatcher *events.EventDispatcher
	taskProj   *events.TaskStateProjection
	velProj    *events.VelocityProjection
	auditProj  *events.AuditTimelineProjection

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

// NewEventSourcedAuditService creates a new event-sourced audit service.
func NewEventSourcedAuditService(store events.EventStore, publisher events.EventPublisher) (*EventSourcedAuditService, error) {
	svc := &EventSourcedAuditService{
		store:     store,
		publisher: publisher,
		taskProj:  events.NewTaskStateProjection(),
		velProj:   events.NewVelocityProjection(7),
		auditProj: events.NewAuditTimelineProjection(),
	}

	// Rebuild projections from existing events
	if err := svc.rebuildProjections(); err != nil {
		return nil, err
	}

	// Subscribe projections to new events (errors non-fatal for projections)
	if publisher != nil {
		publisher.Subscribe(func(e *events.BaseEvent) error {
			_ = svc.taskProj.Apply(e)
			_ = svc.velProj.Apply(e)
			_ = svc.auditProj.Apply(e)
			return nil
		})
	}

	return svc, nil
}

func (s *EventSourcedAuditService) rebuildProjections() error {
	evts, err := s.store.LoadAll()
	if err != nil {
		return err
	}

	if err := s.taskProj.Rebuild(evts); err != nil {
		return err
	}
	if err := s.velProj.Rebuild(evts); err != nil {
		return err
	}
	if err := s.auditProj.Rebuild(evts); err != nil {
		return err
	}

	return nil
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

	// Publish to subscribers (projections, fire-and-forget)
	if s.publisher != nil {
		_ = s.publisher.Publish(event)
	}

	// Dispatch to event handlers
	if s.dispatcher != nil {
		// Use background context for dispatch - handlers should not block audit logging
		go func() {
			_ = s.dispatcher.Dispatch(context.Background(), event)
		}()
	}

	return nil
}

// GetTimeline returns the audit timeline from the projection.
func (s *EventSourcedAuditService) GetTimeline() []events.TimelineEntry {
	return s.auditProj.GetTimeline()
}

// GetRecentTimeline returns the most recent n timeline entries.
func (s *EventSourcedAuditService) GetRecentTimeline(n int) []events.TimelineEntry {
	return s.auditProj.GetRecentEntries(n)
}

// GetTaskState returns the current state of a task from the projection.
func (s *EventSourcedAuditService) GetTaskState(taskID string) *events.TaskState {
	return s.taskProj.GetState(taskID)
}

// GetAllTaskStates returns all task states from the projection.
func (s *EventSourcedAuditService) GetAllTaskStates() map[string]*events.TaskState {
	return s.taskProj.GetAllStates()
}

// GetCompletionVelocity returns tasks completed per day.
func (s *EventSourcedAuditService) GetCompletionVelocity() float64 {
	return s.velProj.GetCompletionVelocity()
}

// GetVerificationVelocity returns tasks verified per day.
func (s *EventSourcedAuditService) GetVerificationVelocity() float64 {
	return s.velProj.GetVerificationVelocity()
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

// SetDispatcher sets the event dispatcher for this service.
func (s *EventSourcedAuditService) SetDispatcher(dispatcher *events.EventDispatcher) {
	s.dispatcher = dispatcher
}

// GetDispatcher returns the event dispatcher.
func (s *EventSourcedAuditService) GetDispatcher() *events.EventDispatcher {
	return s.dispatcher
}

// RegisterHandler registers an event handler with the dispatcher.
// If no dispatcher is set, this creates one.
func (s *EventSourcedAuditService) RegisterHandler(reg events.HandlerRegistration) {
	if s.dispatcher == nil {
		s.dispatcher = events.NewEventDispatcher()
	}
	s.dispatcher.Register(reg)
}
