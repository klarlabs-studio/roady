package application

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/debt"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/felixgeelhaar/roady/pkg/domain/report"
)

// ReportService assembles a stakeholder-facing progress report from the
// planning, forecasting, drift, debt, and audit services. It owns no state of
// its own — it is a read-only composition over what those services already
// know.
type ReportService struct {
	plan     *PlanService
	forecast *ForecastService
	drift    *DriftService
	debt     *DebtService
	audit    *EventSourcedAuditService
}

// NewReportService wires a ReportService. Every collaborator except plan is
// optional; a nil collaborator simply omits its section from the report, so a
// project without drift history or an event log still produces something
// useful.
func NewReportService(
	plan *PlanService,
	forecast *ForecastService,
	driftSvc *DriftService,
	debtSvc *DebtService,
	audit *EventSourcedAuditService,
) *ReportService {
	return &ReportService{plan: plan, forecast: forecast, drift: driftSvc, debt: debtSvc, audit: audit}
}

// ReportOptions controls what a generated report covers.
type ReportOptions struct {
	// Project names the project in the report header.
	Project string
	// Since bounds the "what changed" section. Zero means the whole history.
	Since time.Time
	// MaxChanges caps the change list so a long-running project does not
	// produce an unreadable report. Zero applies a sensible default.
	MaxChanges int
	// Now is the report timestamp, injectable for deterministic tests.
	Now time.Time
}

const defaultMaxChanges = 25

// Generate builds a report. Sections whose underlying service is unavailable
// or errors are omitted rather than failing the whole report: a stakeholder
// report that renders without a forecast is far more useful than no report.
func (s *ReportService) Generate(ctx context.Context, opts ReportOptions) (*report.Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	summaries, err := s.plan.GetTaskSummaries(ctx)
	if err != nil {
		return nil, err
	}

	rep := &report.Report{
		Project:     opts.Project,
		GeneratedAt: now,
		Progress:    buildProgress(summaries),
		Assignments: buildAssignments(summaries),
		Risks:       s.buildRisks(ctx),
		Changes:     s.buildChanges(opts),
		Forecast:    s.buildForecast(),
	}

	if !opts.Since.IsZero() {
		since := opts.Since
		rep.Since = &since
	}

	return rep, nil
}

func buildProgress(summaries []project.TaskSummary) report.Progress {
	p := report.Progress{Total: len(summaries)}

	for _, t := range summaries {
		switch {
		case t.Status == planning.StatusVerified:
			p.Verified++
		case t.Status == planning.StatusDone:
			p.Done++
		case t.Status == planning.StatusInProgress:
			p.InProgress++
		case t.Status == planning.StatusBlocked:
			p.Blocked++
		case t.IsUnlocked:
			p.Ready++
		default:
			p.Pending++
		}
	}

	if p.Total > 0 {
		p.Percent = float64(p.Done+p.Verified) / float64(p.Total) * 100
	}

	return p
}

// buildAssignments groups open work by owner so a lead can see who is loaded
// and what is unowned. Completed tasks are counted but not listed — the point
// is what is outstanding.
func buildAssignments(summaries []project.TaskSummary) []report.Assignment {
	byOwner := map[string]*report.Assignment{}

	for _, t := range summaries {
		key := strings.TrimSpace(t.Owner)
		a, ok := byOwner[key]
		if !ok {
			a = &report.Assignment{Owner: key, Unassigned: key == ""}
			byOwner[key] = a
		}

		switch {
		case t.Status.IsComplete():
			a.Done++
			continue
		case t.Status == planning.StatusBlocked:
			a.Blocked++
		default:
			a.Active++
		}

		a.OpenTasks = append(a.OpenTasks, report.TaskLine{
			ID:     t.ID,
			Title:  t.Title,
			Status: t.Status.String(),
		})
	}

	assignments := make([]report.Assignment, 0, len(byOwner))
	for _, a := range byOwner {
		assignments = append(assignments, *a)
	}

	// Named owners first, alphabetically; unassigned last so it reads as the
	// closing "and nobody owns this" note.
	sort.Slice(assignments, func(i, j int) bool {
		if assignments[i].Unassigned != assignments[j].Unassigned {
			return !assignments[i].Unassigned
		}
		return assignments[i].Owner < assignments[j].Owner
	})

	return assignments
}

// buildRisks merges current drift with debt that has gone sticky. Both are
// best-effort: a project that has never run drift detection reports no risks
// rather than erroring.
func (s *ReportService) buildRisks(ctx context.Context) []report.Risk {
	var risks []report.Risk

	if s.drift != nil {
		if driftReport, err := s.drift.DetectDrift(ctx); err == nil && driftReport != nil {
			for _, issue := range driftReport.Issues {
				risks = append(risks, report.Risk{
					Severity:  string(issue.Severity),
					Kind:      "drift/" + string(issue.Type),
					Component: issue.ComponentID,
					Message:   issue.Message,
				})
			}
		}
	}

	if s.debt != nil {
		if sticky, err := s.debt.GetStickyDrift(); err == nil {
			for _, item := range sticky {
				if item == nil {
					continue
				}
				risks = append(risks, report.Risk{
					Severity:    stickySeverity(item),
					Kind:        "debt/" + string(item.Category),
					Component:   item.ComponentID,
					Message:     item.Message,
					DaysPending: item.DaysPending,
				})
			}
		}
	}

	sort.SliceStable(risks, func(i, j int) bool {
		return severityRank(risks[i].Severity) < severityRank(risks[j].Severity)
	})

	return risks
}

// stickySeverity escalates debt by how long it has sat unresolved. Debt that
// has outlived a month is a different conversation from debt found last week.
func stickySeverity(item *debt.DebtItem) string {
	switch {
	case item.DaysPending >= 30:
		return string(drift.SeverityHigh)
	case item.DaysPending >= 14:
		return string(drift.SeverityMedium)
	default:
		return string(drift.SeverityLow)
	}
}

func severityRank(severity string) int {
	switch strings.ToLower(severity) {
	case string(drift.SeverityCritical):
		return 0
	case string(drift.SeverityHigh):
		return 1
	case string(drift.SeverityMedium):
		return 2
	default:
		return 3
	}
}

// buildChanges summarises the event log for the reporting window, newest
// first, capped so the report stays readable.
func (s *ReportService) buildChanges(opts ReportOptions) []report.Change {
	if s.audit == nil {
		return nil
	}

	var (
		raw []*eventRecord
		max = opts.MaxChanges
	)
	if max <= 0 {
		max = defaultMaxChanges
	}

	loaded, err := s.audit.LoadEventsSince(opts.Since)
	if err != nil {
		return nil
	}

	for _, e := range loaded {
		if e == nil || !isReportableAction(e.Action) {
			continue
		}
		raw = append(raw, &eventRecord{
			at:     e.Timestamp,
			action: e.Action,
			actor:  e.Actor,
			target: eventTarget(e.Metadata),
		})
	}

	sort.SliceStable(raw, func(i, j int) bool { return raw[i].at.After(raw[j].at) })
	if len(raw) > max {
		raw = raw[:max]
	}

	changes := make([]report.Change, 0, len(raw))
	for _, r := range raw {
		changes = append(changes, report.Change{
			At:      r.at,
			Action:  r.action,
			Actor:   r.actor,
			Summary: summariseEvent(r.action, r.target),
		})
	}

	return changes
}

type eventRecord struct {
	at     time.Time
	action string
	actor  string
	target string
}

// reportableActions is the subset of the event log a stakeholder cares about.
// Routine bookkeeping (assignment, linking, billing) is deliberately excluded
// so the "what changed" section stays a narrative rather than an audit dump.
var reportableActions = map[string]string{
	"task.completed": "completed",
	"task.verified":  "verified",
	"task.blocked":   "was blocked",
	"task.unblocked": "was unblocked",
	"task.started":   "started",
	"plan.approved":  "plan approved",
	"plan.rejected":  "plan rejected",
	"drift.detected": "drift detected",
	"drift.accepted": "drift accepted",
	"drift.resolved": "drift resolved",
}

func isReportableAction(action string) bool {
	_, ok := reportableActions[action]
	return ok
}

func summariseEvent(action, target string) string {
	phrase, ok := reportableActions[action]
	if !ok {
		phrase = action
	}
	if target == "" {
		return phrase
	}
	if strings.HasPrefix(action, "task.") {
		return target + " " + phrase
	}
	return phrase + ": " + target
}

func eventTarget(metadata map[string]any) string {
	for _, key := range []string{"task_id", "taskID", "component_id", "component", "plan_id"} {
		if v, ok := metadata[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func (s *ReportService) buildForecast() *report.Forecast {
	if s.forecast == nil {
		return nil
	}

	result, err := s.forecast.GetForecast()
	if err != nil || result == nil {
		return nil
	}

	return &report.Forecast{
		Velocity:      result.Velocity,
		EstimatedDays: result.EstimatedDays,
		LowDays:       result.ConfidenceInterval.Low,
		HighDays:      result.ConfidenceInterval.High,
		Trend:         string(result.Trend.Direction),
		DataPoints:    result.DataPoints,
	}
}
