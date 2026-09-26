package application

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
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
	Features []CaptureFeature `json:"features,omitempty" yaml:"features,omitempty" jsonschema:"description=Features to add or update; each may carry requirements"`
	Tasks    []CaptureTask    `json:"tasks,omitempty" yaml:"tasks,omitempty" jsonschema:"description=Tasks to add or update. A requirement already gets a task (task-<requirement id>); list tasks here to add more or to edit one"`
}

// CaptureFeature upserts a feature. Title is required for a new feature.
type CaptureFeature struct {
	ID           string               `json:"id" yaml:"id" jsonschema:"description=Feature ID"`
	Title        *string              `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Feature title (required for a new feature)"`
	Description  *string              `json:"description,omitempty" yaml:"description,omitempty"`
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
}

// CaptureTask upserts a task. A new task needs a title and either a
// requirement or a feature_id to belong to.
type CaptureTask struct {
	ID          string          `json:"id" yaml:"id" jsonschema:"description=Task ID"`
	Title       *string         `json:"title,omitempty" yaml:"title,omitempty" jsonschema:"description=Task title (required for a new task)"`
	Description *string         `json:"description,omitempty" yaml:"description,omitempty"`
	Priority    *string         `json:"priority,omitempty" yaml:"priority,omitempty" jsonschema:"description=low, medium or high"`
	Estimate    *string         `json:"estimate,omitempty" yaml:"estimate,omitempty"`
	DependsOn   *[]string       `json:"depends_on,omitempty" yaml:"depends_on,omitempty" jsonschema:"description=IDs of tasks this one depends on (replaces the current list)"`
	FeatureID   *string         `json:"feature_id,omitempty" yaml:"feature_id,omitempty" jsonschema:"description=Feature the task belongs to"`
	Requirement *string         `json:"requirement,omitempty" yaml:"requirement,omitempty" jsonschema:"description=Requirement the task serves; implies its feature"`
	Check       *planning.Check `json:"check,omitempty" yaml:"check,omitempty" jsonschema:"description=Acceptance check for this task: run or manual"`
}

// CaptureRejection names an item that could not be applied and why.
type CaptureRejection struct {
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

// CaptureResult reports what a capture changed. Items are named kind:id
// (feature:, requirement:, task:).
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
}

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
	derived := s.applyFeatures(doc.Features, nextSpec, nextPlan, tracker, result)
	s.applyTasks(doc.Tasks, nextSpec, nextPlan, derived, opts.Origin, tracker, result)
	validateCapture(nextSpec, nextPlan, result)

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

	nextPlan.ApprovalStatus, result.ApprovalReason = approvalAfterCapture(prevPlan, tracker)
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

	if s.audit != nil {
		if err := s.audit.Log("plan.capture", actor, map[string]any{
			"plan_id":  nextPlan.ID,
			"created":  result.Created,
			"updated":  result.Updated,
			"approval": result.PlanApproval,
		}); err != nil {
			return result, fmt.Errorf("write audit log: %w", err)
		}
	}
	return result, nil
}

// approvalAfterCapture decides the plan's approval status after a change.
// Any change to an approved plan returns it to pending, as regenerating a plan
// always has.
func approvalAfterCapture(prev *planning.Plan, _ *changeTracker) (planning.ApprovalStatus, string) {
	if prev == nil {
		return planning.ApprovalPending, "new plan"
	}
	if prev.ApprovalStatus == planning.ApprovalApproved {
		return planning.ApprovalPending, "the plan changed and needs re-approval"
	}
	return prev.ApprovalStatus, ""
}

func approvalOf(p *planning.Plan) string {
	if p == nil {
		return ""
	}
	return string(p.ApprovalStatus)
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
		t.compare("feature:"+id, before, after)

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
			if !t.compare("requirement:"+rid, before, *req) && !t.isCreated("requirement:"+rid) {
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

		ti := taskIndex(plan, id)
		if ti < 0 {
			if ct.Title == nil || strings.TrimSpace(*ct.Title) == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: "a new task needs a title"})
				continue
			}
			if featureID == "" {
				result.Rejected = append(result.Rejected, CaptureRejection{Item: item, Reason: "a new task needs a requirement or feature_id to belong to"})
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
		if ct.Check != nil {
			c := *ct.Check
			task.Check = &c
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
}

func newChangeTracker() *changeTracker { return &changeTracker{state: map[string]string{}} }

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
		return true
	case changed:
		c.mark(item, "updated")
	case c.state[item] == "":
		c.mark(item, "unchanged")
	}
	return changed
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
