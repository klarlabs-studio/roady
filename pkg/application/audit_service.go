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

	return domain.VerifyChain(entries), nil
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

// GetAITelemetry returns aggregated AI usage metrics from events.
func (s *AuditService) GetAITelemetry() (*AITelemetrySummary, error) {
	events, err := s.repo.LoadEvents()
	if err != nil {
		return nil, err
	}

	summary := &AITelemetrySummary{
		CallsByAction: make(map[string]int),
		TokensByModel: make(map[string]int),
	}

	for _, e := range events {
		// Filter for AI-related events
		if e.Actor != "ai" {
			continue
		}

		// Check for AI-specific event types
		switch e.Action {
		case "plan.ai_decomposition", "spec.reconcile", "spec.ai_explanation", "drift.ai_explanation":
			summary.TotalCalls++
			summary.CallsByAction[e.Action]++

			if e.Metadata != nil {
				if inputTokens, ok := e.Metadata["input_tokens"].(float64); ok {
					summary.TotalInputTokens += int(inputTokens)
				}
				if outputTokens, ok := e.Metadata["output_tokens"].(float64); ok {
					summary.TotalOutputTokens += int(outputTokens)
				}
				if model, ok := e.Metadata["model"].(string); ok {
					tokens := 0
					if it, ok := e.Metadata["input_tokens"].(float64); ok {
						tokens += int(it)
					}
					if ot, ok := e.Metadata["output_tokens"].(float64); ok {
						tokens += int(ot)
					}
					summary.TokensByModel[model] += tokens
				}
			}

		case "plan.ai_decomposition_retry":
			summary.RetryCount++
		}
	}

	return summary, nil
}
