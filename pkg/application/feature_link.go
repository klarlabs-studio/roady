package application

import (
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// resolveFeatureLinks repairs and validates the link from each task to the
// feature it implements, mutating tasks in place and returning what it could
// not fix.
//
// Drift matches tasks to features by id, so a task whose feature_id holds
// anything else is reported as an orphan the moment it is written — the plan
// is born drifted, and nothing said so at the time. Agents writing plans back
// through a plan update reached for the human-readable title, which is
// the obvious thing to reach for and which Roady is perfectly able to
// translate.
//
// So a link Roady can resolve is repaired silently, and a link it cannot is
// reported to the caller rather than stored and complained about later.
func resolveFeatureLinks(features []spec.Feature, tasks []planning.Task) []string {
	if len(features) == 0 {
		// Nothing to match against. Warning once per task here would bury
		// the real problem, which is that there is no spec.
		return nil
	}

	aliases := spec.NewFeatureAliases(features)

	var warnings []string
	for i := range tasks {
		current := tasks[i].FeatureID

		if strings.TrimSpace(current) == "" {
			warnings = append(warnings, fmt.Sprintf(
				"task %q names no feature, so it cannot be traced back to the spec", tasks[i].ID))
			continue
		}

		if aliases.IsID(current) {
			continue
		}

		if canonical, ok := aliases.Resolve(current); ok {
			tasks[i].FeatureID = canonical
			continue
		}

		// Left as written: guessing would attach the task to the wrong
		// feature, which is harder to notice than an orphan.
		warnings = append(warnings, fmt.Sprintf(
			"task %q names feature %q, which is not in the spec; it will be reported as an orphan until the spec or the task is corrected",
			tasks[i].ID, current))
	}

	return warnings
}

// TitleLinkedTasks lists the plan's tasks whose feature_id names a feature by
// its title or a slug rather than its id, mapped to the id it resolves to.
// Plans written before writes repaired these links still hold them.
func TitleLinkedTasks(sp *spec.ProductSpec, plan *planning.Plan) map[string]string {
	if sp == nil || plan == nil {
		return nil
	}
	aliases := spec.NewFeatureAliases(sp.Features)
	out := map[string]string{}
	for _, t := range plan.Tasks {
		if t.FeatureID == "" || aliases.IsID(t.FeatureID) {
			continue
		}
		if id, ok := aliases.Resolve(t.FeatureID); ok {
			out[t.ID] = id
		}
	}
	return out
}

// planRewriter is a store that can save a plan without stamping it as
// updated, for corrections that do not change what the plan describes.
type planRewriter interface {
	RewritePlan(*planning.Plan) error
}

// RepairFeatureLinks points title-linked tasks at their feature ids and
// returns what it changed. The feature each task serves is the same before
// and after, so the plan keeps its approval and its updated_at: a repair is
// not a sign the plan still describes the work, and drift's staleness check
// must not read it as one.
func (s *PlanService) RepairFeatureLinks() (map[string]string, error) {
	sp, err := s.repo.LoadSpec()
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return nil, fmt.Errorf("load plan: %w", err)
	}
	fixes := TitleLinkedTasks(sp, plan)
	if len(fixes) == 0 {
		return nil, nil
	}
	for i := range plan.Tasks {
		if id, ok := fixes[plan.Tasks[i].ID]; ok {
			plan.Tasks[i].FeatureID = id
		}
	}
	save := s.repo.SavePlan
	if rw, ok := s.repo.(planRewriter); ok {
		save = rw.RewritePlan
	}
	if err := save(plan); err != nil {
		return nil, fmt.Errorf("save plan: %w", err)
	}
	if err := s.audit.Log("plan.links_repaired", "cli", map[string]any{
		"plan_id": plan.ID,
		"tasks":   fixes,
	}); err != nil {
		return fixes, fmt.Errorf("repaired, but the audit log was not written: %w", err)
	}
	return fixes, nil
}
