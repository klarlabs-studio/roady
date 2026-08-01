package planning

// EventForTransition returns the single event that moves a task from one
// status to another, if such an event exists.
//
// This is the reverse of the FSM table: callers that know a desired *state*
// rather than an event — an external tracker reporting "this issue is now
// blocked" — need to work out which event gets there from where the task
// currently is. "Blocked" is reached by `block` from pending or in_progress,
// while "pending" is reached by `stop` from in_progress but by `unblock` from
// blocked, so the answer depends on both ends.
func EventForTransition(from, to TaskStatus) (string, bool) {
	if from == to {
		return "", false
	}

	for event, target := range validTransitions[from] {
		if target == to {
			return event, true
		}
	}

	return "", false
}

// PathToStatus returns the shortest sequence of events moving a task from one
// status to another, or false when no path exists.
//
// Some transitions an external tracker can report are legal but not single
// steps: an issue moving from blocked straight to done is `unblock` then
// `start` then `complete`. Applying those one event at a time keeps every
// intermediate state a real, audited transition rather than letting sync
// write a status the FSM would have refused.
func PathToStatus(from, to TaskStatus) ([]string, bool) {
	if from == to {
		return nil, true
	}

	type node struct {
		status TaskStatus
		path   []string
	}

	visited := map[TaskStatus]bool{from: true}
	queue := []node{{status: from}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Iterate the canonical status order rather than the transition map
		// so an equal-length path is chosen deterministically.
		for _, next := range AllTaskStatuses() {
			event, ok := EventForTransition(current.status, next)
			if !ok || visited[next] {
				continue
			}

			path := append(append([]string{}, current.path...), event)
			if next == to {
				return path, true
			}

			visited[next] = true
			queue = append(queue, node{status: next, path: path})
		}
	}

	return nil, false
}
