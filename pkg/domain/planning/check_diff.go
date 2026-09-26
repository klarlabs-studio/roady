package planning

// TaskCheckChange is a difference in one task's check between two plans.
type TaskCheckChange struct {
	TaskID   string
	Kind     string // added | removed | modified
	Old, New *Check
}

// Weakens reports whether the change could make "done" easier to reach. See
// spec.CheckChangeKind.Weakens: a modified command may be looser, and roady
// cannot tell, so it counts.
func (c TaskCheckChange) Weakens() bool { return c.Kind == "removed" || c.Kind == "modified" }

// DiffTaskChecks lists tasks present in both plans whose check differs.
func DiffTaskChecks(prev, next *Plan) []TaskCheckChange {
	if prev == nil || next == nil {
		return nil
	}
	before := map[string]*Check{}
	for i := range prev.Tasks {
		before[prev.Tasks[i].ID] = prev.Tasks[i].Check
	}
	var out []TaskCheckChange
	for i := range next.Tasks {
		t := next.Tasks[i]
		old, existed := before[t.ID]
		if !existed {
			continue
		}
		switch {
		case old.IsZero() && t.Check.IsZero():
		case old.IsZero():
			out = append(out, TaskCheckChange{TaskID: t.ID, Kind: "added", New: t.Check})
		case t.Check.IsZero():
			out = append(out, TaskCheckChange{TaskID: t.ID, Kind: "removed", Old: old})
		case old.Run != t.Check.Run || old.Manual != t.Check.Manual:
			out = append(out, TaskCheckChange{TaskID: t.ID, Kind: "modified", Old: old, New: t.Check})
		}
	}
	return out
}

// Started reports whether work on a task has begun, which is when changing
// its check stops being planning and starts being moving the goalposts.
func (s TaskStatus) Started() bool {
	switch s {
	case StatusInProgress, StatusBlocked, StatusDone, StatusVerified:
		return true
	}
	return false
}
