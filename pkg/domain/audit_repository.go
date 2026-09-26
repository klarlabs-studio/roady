package domain

// AuditRepository handles persistence of events.
type AuditRepository interface {
	RecordEvent(event Event) error
	LoadEvents() ([]Event, error)
}
