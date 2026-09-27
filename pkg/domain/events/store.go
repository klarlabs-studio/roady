package events

import (
	"time"
)

// EventStore provides persistence for domain events.
type EventStore interface {
	// Append adds a new event to the store, chaining it to the previous event.
	Append(event *BaseEvent) error

	// LoadAll returns all events in chronological order.
	LoadAll() ([]*BaseEvent, error)

	// LoadByAggregate returns events for a specific aggregate.
	LoadByAggregate(aggregateType, aggregateID string) ([]*BaseEvent, error)

	// LoadByType returns events of a specific type.
	LoadByType(eventType string) ([]*BaseEvent, error)

	// LoadSince returns events that occurred after the given timestamp.
	LoadSince(since time.Time) ([]*BaseEvent, error)

	// LoadRange returns events within a time range.
	LoadRange(from, to time.Time) ([]*BaseEvent, error)

	// GetLastEvent returns the most recent event (for hash chaining).
	GetLastEvent() (*BaseEvent, error)

	// Count returns the total number of events.
	Count() (int, error)
}
