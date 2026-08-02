package planning

import "fmt"

// ValidateDAG checks if the plan has circular dependencies.
// It returns a list of task IDs involved in a cycle if found.
func (p *Plan) ValidateDAG() error {
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	// Build ID map for easy lookup
	taskMap := make(map[string]Task)
	for _, t := range p.Tasks {
		taskMap[t.ID] = t
	}

	var visit func(taskID string) error
	visit = func(taskID string) error {
		visited[taskID] = true
		recursionStack[taskID] = true

		task, exists := taskMap[taskID]
		if !exists {
			// Dependency on non-existent task is not a cycle, but invalid graph.
			// We skip for cycle detection, but strictly this is an error.
			recursionStack[taskID] = false
			return nil
		}

		// Only local edges participate in cycle detection. A cross-project
		// dependency cannot be resolved from inside one plan, and treating
		// it as an unknown task would have worked here only by accident —
		// the branch above silently tolerates anything it cannot find.
		for _, depID := range task.LocalDependencies() {
			if !visited[depID] {
				if err := visit(depID); err != nil {
					return err
				}
			} else if recursionStack[depID] {
				return fmt.Errorf("cycle detected involving task: %s", depID)
			}
		}

		recursionStack[taskID] = false
		return nil
	}

	for _, t := range p.Tasks {
		if !visited[t.ID] {
			if err := visit(t.ID); err != nil {
				return err
			}
		}
	}

	return nil
}
