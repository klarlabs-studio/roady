package application

import (
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
	"github.com/google/uuid"
)

type AuditService struct {
	repo domain.WorkspaceRepository

	// prov identifies the agent and session behind every event this service
	// records. Stamped in Log so no call site can omit it.
	prov provenance.Context
}

// SetProvenance sets the identity stamped onto subsequently recorded events.
func (s *AuditService) SetProvenance(ctx provenance.Context) {
	s.prov = ctx
}

// Provenance returns the identity currently being stamped.
func (s *AuditService) Provenance() provenance.Context {
	return s.prov
}

// Compile-time check that AuditService implements AuditLogger
var _ domain.AuditLogger = (*AuditService)(nil)

func NewAuditService(repo domain.WorkspaceRepository) *AuditService {
	return &AuditService{repo: repo}
}

func (s *AuditService) Log(action string, actor string, metadata map[string]any) error {
	// Get the latest event to continue the hash chain
	events, _ := s.repo.LoadEvents()
	prevHash := ""
	if len(events) > 0 {
		prevHash = events[len(events)-1].Hash
	}

	event := domain.Event{
		HashAlgo:  domain.HashAlgoCurrent,
		ID:        uuid.New().String(),
		Timestamp: time.Now(),
		Action:    action,
		Actor:     actor,
		Metadata:  s.prov.Apply(metadata),
		PrevHash:  prevHash,
	}
	event.Hash = event.CalculateHash()

	return s.repo.RecordEvent(event)
}

func (s *AuditService) GetTimeline() ([]domain.Event, error) {
	return s.repo.LoadEvents()
}

// rawEventLoader exposes the undeduplicated log. Asserted optionally so
// repositories and test doubles without it keep working.
type rawEventLoader interface {
	LoadEventsRaw() ([]domain.Event, error)
}

func (s *AuditService) VerifyIntegrity() ([]string, error) {
	violations, err := s.VerifyIntegrityDetailed()
	if err != nil {
		return nil, err
	}
	if len(violations) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(violations))
	for i := range violations {
		out = append(out, violations[i].Message)
	}
	return out, nil
}

// VerifyIntegrityDetailed is VerifyIntegrity with the reason for each finding
// kept alongside it, so a caller can report how many entries failed under an
// algorithm this build knows — the only count that can mean tampering —
// separately from history it merely cannot check.
func (s *AuditService) VerifyIntegrityDetailed() ([]domain.ChainViolation, error) {
	// Verify against the raw log: LoadEvents deduplicates so projections
	// stay correct, but a reviewer should still be told the log contains
	// the same event twice.
	load := s.repo.LoadEvents
	if raw, ok := s.repo.(rawEventLoader); ok {
		load = raw.LoadEventsRaw
	}

	events, err := load()
	if err != nil {
		return nil, err
	}

	entries := make([]domain.ChainEntry, 0, len(events))
	for i := range events {
		e := events[i]
		entries = append(entries, domain.ChainEntry{
			ID:         e.ID,
			Hash:       e.Hash,
			PrevHash:   e.PrevHash,
			HashAlgo:   e.HashAlgo,
			Verifiable: e.Verifiable(),
			Matches:    e.HashMatches(),
		})
	}

	return domain.VerifyChainDetailed(entries), nil
}

// GetVelocity returns the average verified tasks per day over the last 7 days.
func (s *AuditService) GetVelocity() (float64, error) {
	events, err := s.repo.LoadEvents()
	if err != nil {
		return 0, err
	}

	if len(events) == 0 {
		return 0, nil
	}

	var firstVerify time.Time
	verifiedCount := 0

	for _, e := range events {
		if e.Action == "task.transition" && e.Metadata["status"] == "verified" {
			if firstVerify.IsZero() {
				firstVerify = e.Timestamp
			}
			verifiedCount++
		}
	}

	if verifiedCount == 0 {
		return 0, nil
	}

	days := time.Since(firstVerify).Hours() / 24.0
	if days < 1 {
		days = 1 // Floor at 1 day to avoid infinity/large spikes
	}

	return float64(verifiedCount) / days, nil
}
