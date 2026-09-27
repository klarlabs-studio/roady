package application

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// CaptureDoc is intent at whatever size it arrives: one task, a few, or a
// whole plan with the features and requirements behind it.
//
// It replaces the round trip an agent previously had to make to record a plan
// (spec_add, plan_generate --ai, run the prompt, plan_update, approve) with a
// single write. Writing to roady has to cost less than writing plan.md, or
// plans end up in plan.md.
//
// Every item is keyed by ID and upserted. Fields left out keep their current
// value, so the same document shape is also how an item is edited.
type CaptureDoc struct {
	Decisions []CaptureDecision `json:"decisions,omitempty" yaml:"decisions,omitempty" jsonschema:"description=Decisions to record or update: title, choice, context, consequences, and the goals/features/requirements they apply to"`
	Goals     []CaptureGoal     `json:"goals,omitempty" yaml:"goals,omitempty" jsonschema:"description=Roadmap goals to add or update: outcomes with a horizon (now, next, later) that features link to"`
	Features  []CaptureFeature  `json:"features,omitempty" yaml:"features,omitempty" jsonschema:"description=Features to add or update; each may carry requirements"`
	Tasks     []CaptureTask     `json:"tasks,omitempty" yaml:"tasks,omitempty" jsonschema:"description=Tasks to add or update. A requirement already gets a task (task-<requirement id>); list tasks here to add more or to edit one"`
}

// CaptureDecision upserts a decision record. Title and choice are required
// for a new one; lists replace the current ones when given.
type CaptureDecision struct {
	ID           string    `json:"id" yaml:"id" jsonschema:"description=Decision ID"`
	Title        *string   `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=What was decided about (required for a new decision)"`
	Choice       *string   `json:"choice,omitempty" yaml:"choice,omitempty" jsonschema:"description=What was chosen (required for a new decision)"`
	Context      *string   `json:"context,omitempty" yaml:"context,omitempty" jsonschema:"description=Why the question came up; options weighed"`
	Consequences *string   `json:"consequences,omitempty" yaml:"consequences,omitempty" jsonschema:"description=What follows from it"`
	Date         *string   `json:"date,omitempty" yaml:"date,omitempty" jsonschema:"description=YYYY-MM-DD (default today)"`
	Goals        *[]string `json:"goals,omitempty" yaml:"goals,omitempty"`
	Features     *[]string `json:"features,omitempty" yaml:"features,omitempty"`
	Requirements *[]string `json:"requirements,omitempty" yaml:"requirements,omitempty"`
	// Supersedes marks an earlier decision as replaced by this one.
	Supersedes *string `json:"supersedes,omitempty" yaml:"supersedes,omitempty" jsonschema:"description=ID of a decision this one replaces"`
}

// CaptureGoal upserts a roadmap goal. Title is required for a new goal.
type CaptureGoal struct {
	ID          string  `json:"id" yaml:"id" jsonschema:"description=Goal ID"`
	Title       *string `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Goal title (required for a new goal)"`
	Description *string `json:"description,omitempty" yaml:"description,omitempty"`
	Horizon     *string `json:"horizon,omitempty" yaml:"horizon,omitempty" jsonschema:"description=now, next or later; empty for shipped or out-of-scope goals"`
	Status      *string `json:"status,omitempty" yaml:"status,omitempty" jsonschema:"description=idea (needs no features yet), planned (default), shipped or out_of_scope"`
	Milestone   *string `json:"milestone,omitempty" yaml:"milestone,omitempty" jsonschema:"description=Release or checkpoint, e.g. v0.22.0"`
}

// CaptureFeature upserts a feature. Title is required for a new feature.
type CaptureFeature struct {
	ID           string               `json:"id" yaml:"id" jsonschema:"description=Feature ID"`
	Title        *string              `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Feature title (required for a new feature)"`
	Description  *string              `json:"description,omitempty" yaml:"description,omitempty"`
	Goal         *string              `json:"goal,omitempty" yaml:"goal,omitempty" jsonschema:"description=ID of the goal this feature serves"`
	Requirements []CaptureRequirement `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

// CaptureRequirement upserts a requirement within its feature.
type CaptureRequirement struct {
	ID          string      `json:"id" yaml:"id" jsonschema:"description=Requirement ID, unique across the spec"`
	Title       *string     `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Requirement title (required for a new requirement)"`
	Description *string     `json:"description,omitempty" yaml:"description,omitempty"`
	Priority    *string     `json:"priority,omitempty" yaml:"priority,omitempty" jsonschema:"description=low, medium or high"`
	Estimate    *string     `json:"estimate,omitempty" yaml:"estimate,omitempty" jsonschema:"description=e.g. 4h or 1d"`
	DependsOn   *[]string   `json:"depends_on,omitempty" yaml:"depends_on,omitempty" jsonschema:"description=IDs of requirements this one depends on"`
	Check       *spec.Check `json:"check,omitempty" yaml:"check,omitempty" jsonschema:"description=How the requirement is shown to be met: run (a command) or manual (a description)"`
	Goal        *string     `json:"goal,omitempty" yaml:"goal,omitempty" jsonschema:"description=ID of a goal other than its feature's"`
}

// CaptureTask upserts a task. A new task needs a title. With a requirement
// or feature_id it traces to the spec; without, it is unplanned work — in
// the inbox, or under a goal.
type CaptureTask struct {
	ID          string          `json:"id" yaml:"id" jsonschema:"description=Task ID"`
	Title       *string         `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Task title (required for a new task)"`
	Description *string         `json:"description,omitempty" yaml:"description,omitempty"`
	Priority    *string         `json:"priority,omitempty" yaml:"priority,omitempty" jsonschema:"description=low, medium or high"`
	Estimate    *string         `json:"estimate,omitempty" yaml:"estimate,omitempty"`
	DependsOn   *[]string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty" jsonschema:"description=IDs of tasks this one depends on (replaces the current list)"`
	FeatureID   *string         `json:"feature_id,omitempty" yaml:"feature_id,omitempty" jsonschema:"description=Feature the task belongs to"`
	Requirement *string         `json:"requirement,omitempty" yaml:"requirement,omitempty" jsonschema:"description=Requirement the task serves; implies its feature"`
	Goal        *string         `json:"goal,omitempty" yaml:"goal,omitempty" jsonschema:"description=Goal for unplanned work (a task with no requirement or feature)"`
	Check       *planning.Check `json:"check,omitempty" yaml:"check,omitempty" jsonschema:"description=Acceptance check for this task: run or manual"`
	// Source cites where the task came from (doc:line), e.g. a step in an
	// imported plan file.
	Source *planning.TaskSource `json:"source,omitempty" yaml:"source,omitempty" jsonschema:"description=Where the task came from: doc and line"`
}

// CaptureRejection names an item that could not be applied and why.
type CaptureRejection struct {
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

// CaptureResult reports what a capture changed. Items are named kind:id
// (goal:, feature:, requirement:, task:). A goal link changing on a feature
// or requirement is reported as link:feature:<id> / link:requirement:<id>.
type CaptureResult struct {
	Created   []string           `json:"created"`
	Updated   []string           `json:"updated"`
	Unchanged int                `json:"unchanged"`
	Rejected  []CaptureRejection `json:"rejected,omitempty"`
	// Applied is false when nothing was written: a rejection (the capture is
	// all or nothing), a dry run, or a document that changed nothing.
	Applied        bool   `json:"applied"`
	DryRun         bool   `json:"dry_run,omitempty"`
	PlanID         string `json:"plan_id,omitempty"`
	PlanApproval   string `json:"plan_approval,omitempty"`
	ApprovalReason string `json:"approval_reason,omitempty"`
}

// Changed reports whether the capture altered anything.
func (r *CaptureResult) Changed() bool { return len(r.Created)+len(r.Updated) > 0 }

// CaptureService applies CaptureDocs to the spec and plan.
type CaptureService struct {
	repo  domain.WorkspaceRepository
	audit domain.AuditLogger
}

func NewCaptureService(repo domain.WorkspaceRepository, audit domain.AuditLogger) *CaptureService {
	return &CaptureService{repo: repo, audit: audit}
}

// CaptureOptions controls one capture.
type CaptureOptions struct {
	Actor  string
	DryRun bool
	// Origin is recorded on tasks the capture creates directly: human from
	// the CLI, ai from MCP.
	Origin planning.TaskOrigin
	// AllowCheckChange permits removing or changing the check of work
	// already started. CLI only (--change-checks); never set from MCP.
	AllowCheckChange bool
	// Note says in words what the capture was for ("Split task-x into 3
	// parts"); recorded with the event.
	Note string
	// Via names the path the capture came through, for adoption metrics:
	// ViaPlanImport for a plan file brought in by hand, ViaPlanImportAuto
	// for one a hook imported when the user approved it.
	Via string
}

// Capture paths recorded as "via" on the event.
const (
	ViaPlanImport     = "plan-import"
	ViaPlanImportAuto = "plan-import-auto"
)

// Capture applies doc atomically: either every item is applied or none is.
// Sending the same document twice changes nothing the second time.
func (s *CaptureService) Capture(doc CaptureDoc, opts CaptureOptions) (*CaptureResult, error) {
	actor, dryRun := opts.Actor, opts.DryRun
	result := &CaptureResult{Created: []string{}, Updated: []string{}, DryRun: dryRun}

	current, err := s.repo.LoadSpec()
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if current == nil {
		return nil, fmt.Errorf("no spec found; run `roady init` first")
	}
	prevPlan, _ := s.repo.LoadPlan()

	nextSpec := cloneSpec(current)
	nextPlan := clonePlan(prevPlan, current.ID)

	tracker := newChangeTracker()
	applyGoals(doc.Goals, nextSpec, tracker, result)
	applyDecisions(doc.Decisions, nextSpec, tracker, result)
	derived := s.applyFeatures(doc.Features, nextSpec, nextPlan, tracker, result)
	s.applyTasks(doc.Tasks, nextSpec, nextPlan, derived, opts.Origin, tracker, result)
	validateCapture(nextSpec, nextPlan, result)
	guard := NewCheckGuard(s.repo, s.audit)
	guarded := guard.Inspect(nextSpec, nextPlan)
	if err := guard.Authorize(guarded, opts.AllowCheckChange); err != nil {
		result.Rejected = append(result.Rejected, CaptureRejection{Item: "checks", Reason: err.Error()})
	}

	result.Created, result.Updated, result.Unchanged = tracker.summary()
	result.PlanID = nextPlan.ID
	if len(result.Rejected) > 0 {
		return result, nil
	}

	// Compare normalised copies: decoding applies defaults (an empty task
	// priority reads back as medium), so an in-memory plan and its round-trip
	// can differ without anything having changed.
	specChanged := !sameJSON(cloneSpec(current), cloneSpec(nextSpec))
	planChanged := (prevPlan == nil && len(nextPlan.Tasks) > 0) ||
		(prevPlan != nil && !sameJSON(clonePlan(prevPlan, current.ID), clonePlan(nextPlan, current.ID)))
	if !specChanged && !planChanged {
		result.PlanApproval = approvalOf(prevPlan)
		return result, nil
	}

	mode := policy.PlanApprovalScope
	if cfg, err := s.repo.LoadPolicy(); err == nil && cfg != nil {
		mode = cfg.ApprovalMode()
	}
	nextPlan.ApprovalStatus, result.ApprovalReason = approvalAfterCapture(prevPlan, tracker, mode)
	result.PlanApproval = string(nextPlan.ApprovalStatus)
	if dryRun {
		return result, nil
	}

	nextPlan.UpdatedAt = time.Now()
	if specChanged {
		if err := s.repo.SaveSpec(nextSpec); err != nil {
			return nil, fmt.Errorf("save spec: %w", err)
		}
	}
	if err := s.repo.SavePlan(nextPlan); err != nil {
		return nil, fmt.Errorf("save plan: %w", err)
	}
	// The lock records the intent the plan was made from, as plan updates
	// already do; otherwise every capture would read as spec drift.
	if err := s.repo.SaveSpecLock(nextSpec); err != nil {
		return nil, fmt.Errorf("save spec lock: %w", err)
	}
	result.Applied = true
	if err := guard.Record(guarded, actor); err != nil {
		return result, err
	}

	if s.audit != nil {
		meta := map[string]any{
			"plan_id":  nextPlan.ID,
			"created":  result.Created,
			"updated":  result.Updated,
			"approval": result.PlanApproval,
			"changes":  tracker.changes(),
		}
		if opts.Note != "" {
			meta["note"] = opts.Note
		}
		if opts.Via != "" {
			meta["via"] = opts.Via
		}
		if err := s.audit.Log("plan.capture", actor, meta); err != nil {
			return result, fmt.Errorf("write audit log: %w", err)
		}
	}
	return result, nil
}

// approvalAfterCapture decides the plan's approval status after a change.
//
// Under the default "scope" mode an approved plan stays approved when only
// tasks changed: adding, splitting or editing work within requirements that
// were already approved does not change what was agreed. A created or edited
// feature or requirement — including its check, which is part of what "done"
// means — changes the intent and needs re-approval. "every_change" restores
// the old behaviour, where any change did.
func approvalAfterCapture(prev *planning.Plan, t *changeTracker, mode string) (planning.ApprovalStatus, string) {
	if prev == nil {
		return planning.ApprovalPending, "new plan"
	}
	if prev.ApprovalStatus != planning.ApprovalApproved {
		return prev.ApprovalStatus, ""
	}
	if mode == policy.PlanApprovalEveryChange {
		return planning.ApprovalPending, "the plan changed and plan_approval is every_change"
	}
	if scope := t.scopeChanges(); len(scope) > 0 {
		return planning.ApprovalPending, "intent changed (" + strings.Join(scope, ", ") + ") and needs re-approval"
	}
	return planning.ApprovalApproved, "only tasks, goals or decisions changed; the approval stands (plan_approval: scope)"
}

func approvalOf(p *planning.Plan) string {
	if p == nil {
		return ""
	}
	return string(p.ApprovalStatus)
}

// applyDecisions upserts decision records. Like goals, they never reopen
// the plan's approval.
func applyDecisions(decisions []CaptureDecision, sp *spec.ProductSpec, t *changeTracker, result *CaptureResult) {
	for _, cd := range decisions {
		id := strings.TrimSpace(cd.ID)
		if id == "" {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: "decision:", Reason: "a decision needs an id"})
			continue
		}
		di := sp.DecisionIndex(id)
		if di < 0 {
			if cd.Title == nil || strings.TrimSpace(*cd.Title) == "" || cd.Choice == nil || strings.TrimSpace(*cd.Choice) == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "decision:" + id, Reason: "a new decision needs a title and a choice"})
				continue
			}
			sp.Decisions = append(sp.Decisions, spec.Decision{ID: id, Date: time.Now().Format("2006-01-02")})
			di = len(sp.Decisions) - 1
			t.created("decision:" + id)
		}
		d := &sp.Decisions[di]
		before := cloneDecision(*d)
		setStr(&d.Title, cd.Title)
		setStr(&d.Choice, cd.Choice)
		setStr(&d.Context, cd.Context)
		setStr(&d.Consequences, cd.Consequences)
		setStr(&d.Date, cd.Date)
		if cd.Goals != nil {
			d.Goals = append([]string{}, (*cd.Goals)...)
		}
		if cd.Features != nil {
			d.Features = append([]string{}, (*cd.Features)...)
		}
		if cd.Requirements != nil {
			d.Requirements = append([]string{}, (*cd.Requirements)...)
		}
		t.compare("decision:"+id, before, *d)
		if cd.Supersedes != nil && *cd.Supersedes != "" {
			old := sp.DecisionIndex(*cd.Supersedes)
			if old < 0 || *cd.Supersedes == id {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "decision:" + id, Reason: fmt.Sprintf("cannot supersede %q: no such other decision", *cd.Supersedes)})
				continue
			}
			prev := cloneDecision(sp.Decisions[old])
			sp.Decisions[old].Status = spec.DecisionSuperseded
			sp.Decisions[old].SupersededBy = id
			t.compare("decision:"+sp.Decisions[old].ID, prev, sp.Decisions[old])
		}
	}
}

func cloneDecision(d spec.Decision) spec.Decision {
	d.Goals = append([]string(nil), d.Goals...)
	d.Features = append([]string(nil), d.Features...)
	d.Requirements = append([]string(nil), d.Requirements...)
	return d
}

// applyGoals upserts roadmap goals. Goals order work rather than define it,
// so a goal change never reopens the plan's approval.
func applyGoals(goals []CaptureGoal, sp *spec.ProductSpec, t *changeTracker, result *CaptureResult) {
	for _, cg := range goals {
		id := strings.TrimSpace(cg.ID)
		if id == "" {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: "goal:", Reason: "a goal needs an id"})
			continue
		}
		var horizon spec.Horizon
		var status spec.GoalStatus
		var err error
		if cg.Horizon != nil {
			if horizon, err = spec.ParseHorizon(*cg.Horizon); err != nil {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "goal:" + id, Reason: err.Error()})
				continue
			}
		}
		if cg.Status != nil {
			if status, err = spec.ParseGoalStatus(*cg.Status); err != nil {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "goal:" + id, Reason: err.Error()})
				continue
			}
		}
		gi := sp.GoalIndex(id)
		if gi < 0 {
			if cg.Title == nil || strings.TrimSpace(*cg.Title) == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "goal:" + id, Reason: "a new goal needs a title"})
				continue
			}
			sp.Goals = append(sp.Goals, spec.Goal{ID: id})
			gi = len(sp.Goals) - 1
			t.created("goal:" + id)
		}
		g := &sp.Goals[gi]
		before := *g
		setStr(&g.Title, cg.Title)
		setStr(&g.Description, cg.Description)
		setStr(&g.Milestone, cg.Milestone)
		if cg.Horizon != nil {
			g.Horizon = horizon
		}
		if cg.Status != nil {
			g.Status = status
		}
		t.compare("goal:"+id, before, *g)
	}
}

// applyFeatures upserts features and requirements, and derives a task for
// each requirement created or changed. It returns the derived task IDs, which
// an explicit task in the same document may refine.
func (s *CaptureService) applyFeatures(features []CaptureFeature, sp *spec.ProductSpec, plan *planning.Plan, t *changeTracker, result *CaptureResult) map[string]bool {
	derived := map[string]bool{}
	reqOwner := map[string]string{} // requirement ID -> feature ID
	for _, f := range sp.Features {
		for _, r := range f.Requirements {
			reqOwner[r.ID] = f.ID
		}
	}

	for _, cf := range features {
		id := strings.TrimSpace(cf.ID)
		if id == "" {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: "feature:", Reason: "a feature needs an id"})
			continue
		}
		fi := featureIndex(sp, id)
		if fi < 0 {
			if cf.Title == nil || strings.TrimSpace(*cf.Title) == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "feature:" + id, Reason: "a new feature needs a title"})
				continue
			}
			sp.Features = append(sp.Features, spec.Feature{ID: id, Requirements: []spec.Requirement{}})
			fi = len(sp.Features) - 1
			t.created("feature:" + id)
		}
		before := sp.Features[fi]
		before.Requirements = nil
		setStr(&sp.Features[fi].Title, cf.Title)
		setStr(&sp.Features[fi].Description, cf.Description)
		after := sp.Features[fi]
		after.Requirements = nil
		// Which goal a feature serves is roadmap order, not intent: a new
		// link is reported on its own and does not reopen the approval.
		oldGoal := before.Goal
		setStr(&sp.Features[fi].Goal, cf.Goal)
		after.Goal = oldGoal
		t.compare("feature:"+id, before, after)
		if !t.isCreated("feature:"+id) && sp.Features[fi].Goal != oldGoal {
			t.mark("link:feature:"+id, "updated")
		}

		for _, cr := range cf.Requirements {
			rid := strings.TrimSpace(cr.ID)
			if rid == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "requirement:", Reason: "a requirement needs an id"})
				continue
			}
			if owner, ok := reqOwner[rid]; ok && owner != id {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "requirement:" + rid,
					Reason: fmt.Sprintf("already belongs to feature %q; requirement IDs are unique across the spec", owner)})
				continue
			}
			if cr.Title != nil && planning.IsStatusTitle(*cr.Title) {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "requirement:" + rid, Reason: statusTitleReason(*cr.Title, "task-"+rid)})
				continue
			}
			ri := requirementIndex(sp.Features[fi], rid)
			if ri < 0 {
				if cr.Title == nil || strings.TrimSpace(*cr.Title) == "" {
					result.Rejected = append(result.Rejected, CaptureRejection{Item: "requirement:" + rid, Reason: "a new requirement needs a title"})
					continue
				}
				sp.Features[fi].Requirements = append(sp.Features[fi].Requirements, spec.Requirement{ID: rid, DependsOn: []string{}})
				ri = len(sp.Features[fi].Requirements) - 1
				reqOwner[rid] = id
				t.created("requirement:" + rid)
			}
			req := &sp.Features[fi].Requirements[ri]
			before := cloneRequirement(*req)
			setStr(&req.Title, cr.Title)
			setStr(&req.Description, cr.Description)
			setStr(&req.Priority, cr.Priority)
			setStr(&req.Estimate, cr.Estimate)
			if cr.DependsOn != nil {
				req.DependsOn = append([]string{}, (*cr.DependsOn)...)
			}
			if cr.Check != nil {
				c := *cr.Check
				req.Check = &c
			}
			oldGoal := req.Goal
			setStr(&req.Goal, cr.Goal)
			if !t.isCreated("requirement:"+rid) && req.Goal != oldGoal {
				t.mark("link:requirement:"+rid, "updated")
			}
			cmp := *req
			cmp.Goal = oldGoal
			if !t.compare("requirement:"+rid, before, cmp) && !t.isCreated("requirement:"+rid) {
				continue
			}
			taskID := "task-" + rid
			upsertDerivedTask(plan, sp.Features[fi], *req, cr.DependsOn != nil, t)
			derived[taskID] = true
		}
	}
	return derived
}

// upsertDerivedTask keeps a requirement's task in step with the requirement,
// in the shape plan generation gives it. Dependencies are only rewritten when
// the capture stated the requirement's dependencies, so ones set on the task
// directly are not wiped by an unrelated edit.
func upsertDerivedTask(plan *planning.Plan, f spec.Feature, req spec.Requirement, depsGiven bool, t *changeTracker) {
	id := "task-" + req.ID
	ti := taskIndex(plan, id)
	if ti < 0 {
		plan.Tasks = append(plan.Tasks, planning.Task{ID: id, DependsOn: []string{}, Origin: planning.OriginHeuristic})
		ti = len(plan.Tasks) - 1
		t.created("task:" + id)
		depsGiven = true
	}
	task := &plan.Tasks[ti]
	before := cloneTask(*task)
	task.Title = fmt.Sprintf("%s (%s)", req.Title, f.Title)
	task.Description = req.Description
	task.Priority = planning.TaskPriority(req.Priority)
	task.Estimate = req.Estimate
	task.FeatureID = f.ID
	task.Check = taskCheck(req.Check)
	if req.Source.Doc != "" {
		task.Source = planning.TaskSource{Doc: req.Source.Doc, Line: req.Source.Line}
	}
	if depsGiven {
		deps := make([]string, 0, len(req.DependsOn))
		for _, d := range req.DependsOn {
			deps = append(deps, "task-"+d)
		}
		task.DependsOn = deps
	}
	t.compare("task:"+id, before, *task)
}

func (s *CaptureService) applyTasks(tasks []CaptureTask, sp *spec.ProductSpec, plan *planning.Plan, _ map[string]bool, origin planning.TaskOrigin, t *changeTracker, result *CaptureResult) {
	if origin == "" {
		origin = planning.OriginAI
	}
	for _, ct := range tasks {
		id := strings.TrimSpace(ct.ID)
		if id == "" {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: "task:", Reason: "a task needs an id"})
			continue
		}
		item := "task:" + id

		featureID := ""
		if ct.Requirement != nil && *ct.Requirement != "" {
			owner := ""
			for _, f := range sp.Features {
				if requirementIndex(f, *ct.Requirement) >= 0 {
					owner = f.ID
				}
			}
			if owner == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: fmt.Sprintf("requirement %q does not exist", *ct.Requirement)})
				continue
			}
			featureID = owner
		}
		if ct.FeatureID != nil && *ct.FeatureID != "" {
			if featureIndex(sp, *ct.FeatureID) < 0 {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: fmt.Sprintf("feature %q does not exist", *ct.FeatureID)})
				continue
			}
			if featureID != "" && featureID != *ct.FeatureID {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item,
					Reason: fmt.Sprintf("requirement %q belongs to feature %q, not %q", *ct.Requirement, featureID, *ct.FeatureID)})
				continue
			}
			featureID = *ct.FeatureID
		}

		if ct.Title != nil && planning.IsStatusTitle(*ct.Title) {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: statusTitleReason(*ct.Title, id)})
			continue
		}
		ti := taskIndex(plan, id)
		if ti < 0 {
			if ct.Title == nil || strings.TrimSpace(*ct.Title) == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: "a new task needs a title"})
				continue
			}
			plan.Tasks = append(plan.Tasks, planning.Task{ID: id, DependsOn: []string{}, Origin: origin})
			ti = len(plan.Tasks) - 1
			t.created(item)
		}
		task := &plan.Tasks[ti]
		before := cloneTask(*task)
		setStr(&task.Title, ct.Title)
		setStr(&task.Description, ct.Description)
		if ct.Priority != nil {
			task.Priority = planning.TaskPriority(*ct.Priority)
		}
		setStr(&task.Estimate, ct.Estimate)
		if ct.DependsOn != nil {
			task.DependsOn = append([]string{}, (*ct.DependsOn)...)
		}
		if featureID != "" {
			task.FeatureID = featureID
		}
		if ct.Goal != nil {
			if g := strings.TrimSpace(*ct.Goal); g != "" && sp.GoalIndex(g) < 0 {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: fmt.Sprintf("goal %q does not exist", g)})
				continue
			}
			task.Goal = strings.TrimSpace(*ct.Goal)
		}
		if ct.Check != nil {
			c := *ct.Check
			task.Check = &c
		}
		if ct.Source != nil {
			task.Source = *ct.Source
		}
		t.compare(item, before, *task)
	}
}

// validateCapture checks the result as a whole. Nothing is written when any
// check fails, so a plan is never left half-applied.
func validateCapture(sp *spec.ProductSpec, plan *planning.Plan, result *CaptureResult) {
	for _, err := range sp.Validate() {
		result.Rejected = append(result.Rejected, CaptureRejection{Item: "spec", Reason: err.Error()})
	}
	ids := map[string]bool{}
	for _, task := range plan.Tasks {
		ids[task.ID] = true
	}
	for _, task := range plan.Tasks {
		for _, dep := range task.LocalDependencies() {
			if !ids[dep] {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: "task:" + task.ID, Reason: fmt.Sprintf("depends on %q, which is not in the plan", dep)})
			}
		}
		if err := task.Check.Validate(); err != nil {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: "task:" + task.ID, Reason: err.Error()})
		}
		if task.Priority != "" && !validPriority(string(task.Priority)) {
			result.Rejected = append(result.Rejected, CaptureRejection{Item: "task:" + task.ID, Reason: fmt.Sprintf("priority %q is not low, medium or high", task.Priority)})
		}
	}
	if err := plan.ValidateDAG(); err != nil {
		result.Rejected = append(result.Rejected, CaptureRejection{Item: "plan", Reason: err.Error()})
	}
}

func validPriority(p string) bool {
	switch p {
	case "low", "medium", "high":
		return true
	}
	return false
}

// changeTracker records what happened to each item a capture touched.
type changeTracker struct {
	state map[string]string // item -> created | updated | unchanged
	order []string
	// What changed, for the event log: field diffs of updated items and the
	// full shape of created ones, so every edit is an appended record the
	// plan's history can be read back from.
	diffs map[string]map[string]FieldChange
	snaps map[string]any
}

// FieldChange is one field's value before and after an edit.
type FieldChange struct {
	From any `json:"from"`
	To   any `json:"to"`
}

func newChangeTracker() *changeTracker {
	return &changeTracker{state: map[string]string{}, diffs: map[string]map[string]FieldChange{}, snaps: map[string]any{}}
}

func (c *changeTracker) mark(item, st string) {
	if _, seen := c.state[item]; !seen {
		c.order = append(c.order, item)
	}
	c.state[item] = st
}

func (c *changeTracker) created(item string)        { c.mark(item, "created") }
func (c *changeTracker) isCreated(item string) bool { return c.state[item] == "created" }

// compare marks item updated when before and after differ, and reports it.
// A created item stays created.
func (c *changeTracker) compare(item string, before, after any) bool {
	changed := !sameJSON(before, after)
	switch {
	case c.state[item] == "created":
		c.snaps[item] = jsonValue(after)
		return true
	case changed:
		c.mark(item, "updated")
		c.recordDiff(item, before, after)
	case c.state[item] == "":
		c.mark(item, "unchanged")
	}
	return changed
}

// scopeChanges lists created or updated features and requirements: the
// changes to intent that an approval covered.
func (c *changeTracker) scopeChanges() []string {
	var out []string
	for _, item := range c.order {
		st := c.state[item]
		if (st == "created" || st == "updated") && (strings.HasPrefix(item, "feature:") || strings.HasPrefix(item, "requirement:")) {
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

func (c *changeTracker) summary() (created, updated []string, unchanged int) {
	created, updated = []string{}, []string{}
	for _, item := range c.order {
		switch c.state[item] {
		case "created":
			created = append(created, item)
		case "updated":
			updated = append(updated, item)
		default:
			unchanged++
		}
	}
	sort.Strings(created)
	sort.Strings(updated)
	return created, updated, unchanged
}

// sameJSON compares by serialised form. reflect.DeepEqual reports times that
// went through a JSON round trip as different (their locations are distinct
// pointers), which would turn every no-op capture into a write.
func sameJSON(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}

func setStr(dst *string, v *string) {
	if v != nil {
		*dst = *v
	}
}

func featureIndex(sp *spec.ProductSpec, id string) int {
	for i := range sp.Features {
		if sp.Features[i].ID == id {
			return i
		}
	}
	return -1
}

func requirementIndex(f spec.Feature, id string) int {
	for i := range f.Requirements {
		if f.Requirements[i].ID == id {
			return i
		}
	}
	return -1
}

func taskIndex(p *planning.Plan, id string) int {
	for i := range p.Tasks {
		if p.Tasks[i].ID == id {
			return i
		}
	}
	return -1
}

// Deep copies through JSON, so the originals stay untouched for comparison
// and for a rejected or dry-run capture. The types are plain data.
func cloneSpec(s *spec.ProductSpec) *spec.ProductSpec {
	var out spec.ProductSpec
	b, _ := json.Marshal(s)
	_ = json.Unmarshal(b, &out)
	return &out
}

func clonePlan(p *planning.Plan, specID string) *planning.Plan {
	if p == nil {
		now := time.Now()
		return &planning.Plan{
			ID:             fmt.Sprintf("plan-%s-%d", specID, now.Unix()),
			SpecID:         specID,
			ApprovalStatus: planning.ApprovalPending,
			CreatedAt:      now,
			UpdatedAt:      now,
			Tasks:          []planning.Task{},
		}
	}
	var out planning.Plan
	b, _ := json.Marshal(p)
	_ = json.Unmarshal(b, &out)
	return &out
}

func cloneRequirement(r spec.Requirement) spec.Requirement {
	var out spec.Requirement
	b, _ := json.Marshal(r)
	_ = json.Unmarshal(b, &out)
	return out
}

func cloneTask(t planning.Task) planning.Task {
	var out planning.Task
	b, _ := json.Marshal(t)
	_ = json.Unmarshal(b, &out)
	return out
}

// recordDiff merges the field-level difference of before and after into the
// item's diff, keeping the earliest "from" when an item is touched twice.
func (c *changeTracker) recordDiff(item string, before, after any) {
	b, _ := jsonValue(before).(map[string]any)
	a, _ := jsonValue(after).(map[string]any)
	d := c.diffs[item]
	if d == nil {
		d = map[string]FieldChange{}
		c.diffs[item] = d
	}
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}
	for k := range keys {
		if sameJSON(b[k], a[k]) {
			continue
		}
		from := b[k]
		if prev, ok := d[k]; ok {
			from = prev.From
		}
		if sameJSON(from, a[k]) {
			delete(d, k)
			continue
		}
		d[k] = FieldChange{From: from, To: a[k]}
	}
}

// changes lists what the capture did to each created or updated item, as
// plain JSON values so the event hashes the same after a reload.
func (c *changeTracker) changes() []any {
	var out []map[string]any
	for _, item := range c.order {
		switch c.state[item] {
		case "created":
			out = append(out, map[string]any{"item": item, "change": "created", "as": c.snaps[item]})
		case "updated":
			if d := c.diffs[item]; len(d) > 0 {
				out = append(out, map[string]any{"item": item, "change": "updated", "fields": d})
			}
		}
	}
	v, _ := jsonValue(out).([]any)
	return v
}

// jsonValue round-trips v through JSON, yielding maps, slices, strings,
// float64s, bools and nils only.
func jsonValue(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// statusTitleReason explains why a status word is refused as a title and
// what the caller most likely meant.
func statusTitleReason(title, taskID string) string {
	return fmt.Sprintf("%q is a status, not a title: it would replace what the task is. To record progress use `roady task complete %s` (MCP roady_task action complete); a title says what the work is", strings.TrimSpace(title), taskID)
}
