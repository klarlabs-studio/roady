package application

import "github.com/felixgeelhaar/roady/pkg/domain/planning"

// StatusTitledTasks lists tasks whose title is only a status word ("done"),
// in plan order. Capture refuses such titles now; plans written before that
// can still carry them.
func StatusTitledTasks(plan *planning.Plan) []string {
	var ids []string
	if plan == nil {
		return ids
	}
	for _, t := range plan.Tasks {
		if planning.IsStatusTitle(t.Title) {
			ids = append(ids, t.ID)
		}
	}
	return ids
}
