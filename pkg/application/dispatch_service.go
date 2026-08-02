package application

import (
	"context"
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/dispatch"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// DispatchService prepares a ready task for handoff to a subagent.
type DispatchService struct {
	repo    domain.WorkspaceRepository
	plan    *PlanService
	taskSvc *TaskService

	// audits are the services whose identity stamp is swapped while a
	// dispatch is recorded. There are two writers to events.jsonl and a task
	// start reaches the event-sourced one through the coordinator, so
	// stamping only the other leaves the claim attributed to the dispatcher.
	audits []provenanceSetter
}

// provenanceSetter is the slice of the audit service needed to attribute a
// dispatch to the agent receiving it.
type provenanceSetter interface {
	Provenance() provenance.Context
	SetProvenance(provenance.Context)
}

func NewDispatchService(repo domain.WorkspaceRepository, plan *PlanService, taskSvc *TaskService) *DispatchService {
	return &DispatchService{repo: repo, plan: plan, taskSvc: taskSvc}
}

// SetAuditProvenance supplies every audit service whose identity stamp should
// be swapped while a dispatch is recorded.
func (s *DispatchService) SetAuditProvenance(a ...provenanceSetter) { s.audits = a }

// DispatchOptions identifies who is being handed the work.
type DispatchOptions struct {
	// Agent names the subagent taking the task. It becomes the owner and is
	// recorded against the transition.
	Agent string
	// Session groups the subagent's events. Empty means the dispatching
	// process's own session.
	Session string
	// Start moves the task to in_progress as part of dispatching. Off for a
	// dry run, where the caller wants the brief without claiming the work.
	Start bool
}

// Dispatch builds the brief for a task and, when asked, claims it.
//
// Only a ready task can be dispatched. Handing out work whose dependencies
// are unmet produces an agent that either blocks or, worse, implements
// against something that does not exist yet.
func (s *DispatchService) Dispatch(ctx context.Context, taskID string, opts DispatchOptions) (*dispatch.Brief, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("a task id is required")
	}
	if strings.TrimSpace(opts.Agent) == "" {
		return nil, fmt.Errorf("an agent name is required: the completion contract records who did the work, and an unattributed dispatch defeats the audit trail")
	}

	plan, err := s.repo.LoadPlan()
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, fmt.Errorf("no plan found; run 'roady plan generate' first")
	}

	var task *planning.Task
	for i := range plan.Tasks {
		if plan.Tasks[i].ID == taskID {
			task = &plan.Tasks[i]
			break
		}
	}
	if task == nil {
		return nil, fmt.Errorf("task %q is not in the plan", taskID)
	}

	if err := s.assertReady(ctx, taskID); err != nil {
		return nil, err
	}

	brief := s.buildBrief(task, opts)

	if opts.Start {
		// Record the claim under the subagent's identity. Otherwise the
		// trail splits one piece of work across two sessions: the claim
		// under whoever dispatched, the completion under the agent that did
		// it — and "which agent worked on this" gets two answers.
		for _, a := range s.audits {
			if a == nil {
				continue
			}
			previous := a.Provenance()
			a.SetProvenance(previous.WithSession(opts.Session, opts.Agent))
			defer a.SetProvenance(previous)
		}

		if err := s.taskSvc.StartTask(ctx, taskID, opts.Agent, ""); err != nil {
			return nil, fmt.Errorf("claim task for %s: %w", opts.Agent, err)
		}
	}

	return brief, nil
}

// assertReady refuses to dispatch work that is not actually available.
func (s *DispatchService) assertReady(ctx context.Context, taskID string) error {
	ready, err := s.plan.GetReadyTasks(ctx)
	if err != nil {
		return err
	}
	for _, t := range ready {
		if t.ID == taskID {
			return nil
		}
	}

	// Say why, since "not ready" covers several different situations the
	// dispatcher can act on.
	summaries, sErr := s.plan.GetTaskSummaries(ctx)
	if sErr == nil {
		for _, t := range summaries {
			if t.ID != taskID {
				continue
			}
			switch {
			case t.Status.IsComplete():
				return fmt.Errorf("task %q is already %s", taskID, t.Status)
			case t.Status == planning.StatusInProgress:
				return fmt.Errorf("task %q is already in progress (owner: %s)", taskID, orUnowned(t.Owner))
			case t.IsBlocked:
				return fmt.Errorf("task %q is blocked", taskID)
			default:
				return fmt.Errorf("task %q is not ready: it depends on %s", taskID, strings.Join(t.DependsOn, ", "))
			}
		}
	}
	return fmt.Errorf("task %q is not ready", taskID)
}

func orUnowned(owner string) string {
	if owner == "" {
		return "unassigned"
	}
	return owner
}

func (s *DispatchService) buildBrief(task *planning.Task, opts DispatchOptions) *dispatch.Brief {
	brief := &dispatch.Brief{
		TaskID:      task.ID,
		Title:       task.Title,
		Description: task.Description,
		Priority:    task.Priority,
		Estimate:    task.Estimate,
		DependsOn:   task.DependsOn,
	}

	if !task.Source.IsZero() {
		brief.Citation = fmt.Sprintf("%s:%d", task.Source.Doc, task.Source.Line)
	}

	// Pull the originating feature and requirement so the agent works from
	// intent rather than from a task title.
	if productSpec, err := s.repo.LoadSpec(); err == nil && productSpec != nil {
		if feature := findFeature(productSpec, task.FeatureID); feature != nil {
			brief.Feature = feature.Title
			if req := findRequirementForTask(feature, task.ID); req != nil {
				brief.Requirement = req.Description
				brief.Acceptance = req.Description
			}
		}
	}

	args := map[string]string{
		"task_id": task.ID,
		"event":   "complete",
		"actor":   opts.Agent,
		"agent":   opts.Agent,
	}
	cli := fmt.Sprintf("roady task complete %s --evidence <commit-or-link>", task.ID)
	if opts.Session != "" {
		args["session_id"] = opts.Session
		cli = fmt.Sprintf("ROADY_SESSION_ID=%s ROADY_AGENT=%s %s", opts.Session, opts.Agent, cli)
	} else {
		cli = fmt.Sprintf("ROADY_AGENT=%s %s", opts.Agent, cli)
	}

	brief.Completion = dispatch.CompletionContract{
		Tool:             "roady_transition_task",
		CLI:              cli,
		Arguments:        args,
		EvidenceRequired: true,
		Instructions: "When the work is done, call roady_transition_task with these arguments and an evidence value " +
			"(a commit hash or link). The transition is what records the work against you in the audit trail — " +
			"without it the task stays in progress and nothing attributes it.",
	}

	return brief
}

func findFeature(productSpec *spec.ProductSpec, featureID string) *spec.Feature {
	for i := range productSpec.Features {
		if productSpec.Features[i].ID == featureID {
			return &productSpec.Features[i]
		}
	}
	return nil
}

// findRequirementForTask matches the requirement a task was generated from.
// The heuristic planner names tasks `task-<requirement-id>`, so that mapping
// recovers the originating requirement without storing a second link.
func findRequirementForTask(feature *spec.Feature, taskID string) *spec.Requirement {
	want := strings.TrimPrefix(taskID, "task-")
	for i := range feature.Requirements {
		if feature.Requirements[i].ID == want {
			return &feature.Requirements[i]
		}
	}
	return nil
}
