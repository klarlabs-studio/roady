package application

import (
	"context"
	"sort"
	"strings"
	"time"

	domainaudit "github.com/felixgeelhaar/roady/pkg/domain/audit"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
)

// AuditTrailService assembles evidence trails for GRC review from the event
// log, the plan, and execution state.
type AuditTrailService struct {
	audit *EventSourcedAuditService
	plain *AuditService
	plan  *PlanService
	repo  planStateLoader
}

// planStateLoader is the slice of the repository this service reads.
type planStateLoader interface {
	LoadPlan() (*planning.Plan, error)
	LoadState() (*planning.ExecutionState, error)
}

// NewAuditTrailService wires the service. plain supplies chain verification;
// a nil plain simply reports the chain as unchecked rather than failing.
func NewAuditTrailService(audit *EventSourcedAuditService, plain *AuditService, plan *PlanService, repo planStateLoader) *AuditTrailService {
	return &AuditTrailService{audit: audit, plain: plain, plan: plan, repo: repo}
}

// TrailQuery selects what to build a trail about. Exactly one of TaskID,
// Agent, or SessionID identifies the subject.
type TrailQuery struct {
	TaskID    string
	Agent     string
	SessionID string
	Since     time.Time
	Now       time.Time
}

// BuildTrail assembles the trail for the given query.
func (s *AuditTrailService) BuildTrail(ctx context.Context, q TrailQuery) (*domainaudit.Trail, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	now := q.Now
	if now.IsZero() {
		now = time.Now()
	}

	loaded, err := s.audit.LoadEventsSince(q.Since)
	if err != nil {
		return nil, err
	}

	trail := &domainaudit.Trail{
		Subject:     subjectFor(q),
		GeneratedAt: now,
		Integrity:   s.verifyChain(len(loaded)),
	}
	if !q.Since.IsZero() {
		since := q.Since
		trail.Subject.Since = &since
	}

	for _, e := range loaded {
		if e == nil || !matchesQuery(e.Metadata, e.AggregateID(), q) {
			continue
		}
		trail.Entries = append(trail.Entries, domainaudit.EntryFrom(
			e.Timestamp, eventAction(e.Type, e.Action), e.Actor, e.Hash, e.Metadata,
		))
	}

	// Oldest first: a trail is read as a narrative of what happened.
	sort.SliceStable(trail.Entries, func(i, j int) bool {
		return trail.Entries[i].At.Before(trail.Entries[j].At)
	})

	trail.Actors = domainaudit.BuildActorRoll(trail.Entries)

	if q.TaskID != "" {
		trail.Task = s.taskFacts(ctx, q.TaskID)
	}

	return trail, nil
}

// eventAction picks the name of what happened. Events written through the
// event-sourced path carry it in Type; those written through the original
// domain.Event path carry it in Action, and both shapes coexist in
// events.jsonl. Reading only one leaves half the log's entries nameless.
func eventAction(eventType, action string) string {
	if strings.TrimSpace(eventType) != "" {
		return eventType
	}
	return action
}

func subjectFor(q TrailQuery) domainaudit.Subject {
	switch {
	case q.TaskID != "":
		return domainaudit.Subject{Kind: "task", ID: q.TaskID}
	case q.SessionID != "":
		return domainaudit.Subject{Kind: "session", ID: q.SessionID, SessionID: q.SessionID}
	case q.Agent != "":
		return domainaudit.Subject{Kind: "agent", ID: q.Agent, AgentName: q.Agent}
	default:
		return domainaudit.Subject{Kind: "project", ID: "all"}
	}
}

// matchesQuery decides whether one event belongs in the trail. Task and
// agent/session filters compose, so "everything claude-code did to task-3"
// is expressible.
func matchesQuery(metadata map[string]any, aggregateID string, q TrailQuery) bool {
	if q.TaskID != "" {
		taskID, _ := metadata["task_id"].(string)
		if !strings.EqualFold(taskID, q.TaskID) && !strings.EqualFold(aggregateID, q.TaskID) {
			return false
		}
	}

	if q.Agent != "" || q.SessionID != "" {
		if !provenance.FromMetadata(metadata).Matches(q.Agent, q.SessionID) {
			return false
		}
	}

	return true
}

// verifyChain checks hash-chain integrity. A verification failure is reported
// in the trail rather than returned as an error: a tampered log is precisely
// what a reviewer needs to see, so suppressing the trail would hide it.
func (s *AuditTrailService) verifyChain(eventCount int) domainaudit.Integrity {
	integrity := domainaudit.Integrity{EventsInLog: eventCount}

	if s.plain == nil {
		return integrity
	}

	problems, err := s.plain.VerifyIntegrity()
	integrity.CheckedChain = true
	if err != nil {
		integrity.Problems = []string{"chain verification could not run: " + err.Error()}
		return integrity
	}

	integrity.Problems = problems
	integrity.Verified = len(problems) == 0
	return integrity
}

func (s *AuditTrailService) taskFacts(_ context.Context, taskID string) *domainaudit.TaskFacts {
	if s.repo == nil {
		return nil
	}

	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return nil
	}

	var task *planning.Task
	for i := range plan.Tasks {
		if strings.EqualFold(plan.Tasks[i].ID, taskID) {
			task = &plan.Tasks[i]
			break
		}
	}
	if task == nil {
		return nil
	}

	facts := &domainaudit.TaskFacts{
		ID:         task.ID,
		Title:      task.Title,
		FeatureID:  task.FeatureID,
		Origin:     string(task.Origin),
		SourceDoc:  task.Source.Doc,
		SourceLine: task.Source.Line,
	}

	state, err := s.repo.LoadState()
	if err != nil || state == nil {
		return facts
	}

	result, ok := state.GetTaskResult(task.ID)
	if !ok {
		return facts
	}

	facts.Status = result.Status.String()
	facts.Owner = result.Owner
	facts.Evidence = result.Evidence
	facts.StartedAt = result.StartedAt
	facts.CompletedAt = result.CompletedAt

	if len(result.ExternalRefs) > 0 {
		facts.ExternalRefs = map[string]string{}
		for provider, ref := range result.ExternalRefs {
			label := ref.Identifier
			if label == "" {
				label = ref.ID
			}
			if ref.URL != "" {
				label += " (" + ref.URL + ")"
			}
			facts.ExternalRefs[provider] = label
		}
	}

	return facts
}
