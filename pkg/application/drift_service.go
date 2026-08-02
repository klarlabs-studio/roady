package application

import (
	"context"
	"fmt"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// RepoActivityInspector reports how far the repository has moved since a
// point in time. Injected so the domain stays free of git.
type RepoActivityInspector interface {
	ActivitySince(since time.Time) drift.RepoActivity
}

// planUpdatedAt is the reference point for staleness. A plan with no
// timestamp yields the zero time, which the inspector reports as unavailable
// rather than as infinitely stale.
func planUpdatedAt(plan *planning.Plan) time.Time {
	if plan == nil {
		return time.Time{}
	}
	return plan.UpdatedAt
}

type DriftService struct {
	repo      domain.WorkspaceRepository
	audit     domain.AuditLogger
	inspector drift.CodeInspector
	policy    *PolicyService
	detector  *drift.DriftDetector

	// activity is optional; a nil inspector skips the staleness check
	// rather than reporting a plan as stale on no evidence.
	activity RepoActivityInspector
}

// SetActivityInspector supplies the repository-movement signal used for
// staleness detection.
func (s *DriftService) SetActivityInspector(a RepoActivityInspector) {
	s.activity = a
}

func NewDriftService(repo domain.WorkspaceRepository, audit domain.AuditLogger, inspector drift.CodeInspector, policy *PolicyService) *DriftService {
	return &DriftService{
		repo:      repo,
		audit:     audit,
		inspector: inspector,
		policy:    policy,
		detector:  drift.NewDriftDetector(),
	}
}

func (s *DriftService) DetectDrift(ctx context.Context) (*drift.Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	spec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, err
	}

	plan, err := s.repo.LoadPlan()
	if err != nil {
		return nil, err
	}

	state, err := s.repo.LoadState()
	if err != nil {
		return nil, err
	}

	report := &drift.Report{
		ID:        fmt.Sprintf("drift-%d", time.Now().Unix()),
		CreatedAt: time.Now(),
		Issues:    make([]drift.Issue, 0),
	}

	// 0. Intent Drift (Spec vs Lock)
	lock, _ := s.repo.LoadSpecLock()
	if intentIssues := s.detector.DetectIntentDrift(spec, lock); len(intentIssues) > 0 {
		report.Issues = append(report.Issues, intentIssues...)
	}

	// 1. Plan vs Spec
	if planIssues := s.detector.DetectPlanDrift(spec, plan); len(planIssues) > 0 {
		report.Issues = append(report.Issues, planIssues...)
	}

	// 1b. Staleness — the plan itself falling behind the repository.
	// Every other check compares Roady's artifacts against each other, so a
	// plan nobody edits stays internally consistent while the code moves on.
	if s.activity != nil {
		if staleIssues := s.detector.DetectStalenessDrift(plan, s.activity.ActivitySince(planUpdatedAt(plan)), time.Now()); len(staleIssues) > 0 {
			report.Issues = append(report.Issues, staleIssues...)
		}
	}

	// 2. Code vs State (Implementation Drift)
	if codeIssues := s.detector.DetectCodeDrift(plan, state, s.inspector); len(codeIssues) > 0 {
		report.Issues = append(report.Issues, codeIssues...)
	}

	// 3. Policy vs State (Policy Drift)
	violations, _ := s.policy.CheckCompliance()
	if policyIssues := s.detector.DetectPolicyDrift(violations); len(policyIssues) > 0 {
		report.Issues = append(report.Issues, policyIssues...)
	}

	return report, nil
}

// AcceptDrift locks the current spec snapshot and records the acceptance event.
func (s *DriftService) AcceptDrift() error {
	spec, err := s.repo.LoadSpec()
	if err != nil {
		return fmt.Errorf("load spec: %w", err)
	}
	if spec == nil {
		return fmt.Errorf("no spec found to accept drift")
	}

	if err := s.repo.SaveSpecLock(spec); err != nil {
		return fmt.Errorf("save spec lock: %w", err)
	}

	if s.audit == nil {
		return fmt.Errorf("audit service is not configured")
	}

	if err := s.audit.Log("drift.accepted", "cli", map[string]interface{}{
		"spec_id":   spec.ID,
		"spec_hash": spec.Hash(),
	}); err != nil {
		return fmt.Errorf("log drift acceptance: %w", err)
	}

	return nil
}

// RecordSemanticDrift stores the judgements a caller's model returned for
// SemanticDrift, turning divergences into drift issues.
//
// The verdict comes from outside Roady, so it is recorded as what it is: an
// audited assertion by a named caller, not something Roady established by
// comparing artifacts. Agreement records nothing — this reports drift, not a
// tally of everything checked.
func (s *DriftService) RecordSemanticDrift(ctx context.Context, judgements []drift.SemanticJudgement, questions []drift.SemanticQuestion) (*drift.Report, error) {
	if len(judgements) == 0 {
		return nil, fmt.Errorf("no judgements to record")
	}
	if len(questions) == 0 {
		return nil, fmt.Errorf("no questions to match the judgements against; call semantic drift first and pass back the questions it returned")
	}

	issues := drift.IssuesFrom(judgements, questions)
	report := &drift.Report{Issues: issues, CreatedAt: time.Now()}

	if s.audit != nil {
		// Recorded whether or not anything diverged: that the check ran, and
		// on how many requirements, is itself the evidence a reviewer needs.
		if err := s.audit.Log("drift.semantic_recorded", "ai", map[string]interface{}{
			"judgements": len(judgements),
			"questions":  len(questions),
			"divergent":  len(issues),
		}); err != nil {
			return nil, fmt.Errorf("write audit log: %w", err)
		}
	}

	return report, nil
}
