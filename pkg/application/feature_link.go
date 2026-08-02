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
// through roady_update_plan reached for the human-readable title, which is
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

	ids := make(map[string]bool, len(features))
	// byAlias maps every spelling of a feature Roady is willing to accept to
	// its canonical id: the title, the id, and the slug of each. Keys are
	// normalised so case and spacing do not orphan a task.
	byAlias := make(map[string]string, len(features)*3)

	for _, f := range features {
		if f.ID == "" {
			continue
		}
		ids[f.ID] = true
		for _, alias := range []string{f.ID, f.Title, spec.Slugify(f.Title), spec.Slugify(f.ID)} {
			if key := normaliseAlias(alias); key != "" {
				// First feature to claim an alias keeps it, so an ambiguous
				// title cannot silently steal another feature's tasks.
				if _, taken := byAlias[key]; !taken {
					byAlias[key] = f.ID
				}
			}
		}
	}

	var warnings []string
	for i := range tasks {
		current := tasks[i].FeatureID

		if strings.TrimSpace(current) == "" {
			warnings = append(warnings, fmt.Sprintf(
				"task %q names no feature, so it cannot be traced back to the spec", tasks[i].ID))
			continue
		}

		if ids[current] {
			continue
		}

		if canonical, ok := byAlias[normaliseAlias(current)]; ok {
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

// normaliseAlias reduces a spelling to the form used for matching. It is
// deliberately more forgiving than Slugify: matching is a lookup, not an id.
func normaliseAlias(s string) string {
	return spec.Slugify(strings.TrimSpace(strings.ToLower(s)))
}
