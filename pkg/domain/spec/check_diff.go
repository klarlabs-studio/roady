package spec

// CheckChangeKind names how a check differs between two versions.
type CheckChangeKind string

const (
	CheckAdded    CheckChangeKind = "added"
	CheckRemoved  CheckChangeKind = "removed"
	CheckModified CheckChangeKind = "modified"
)

// Weakens reports whether a change could make "done" easier to reach. Adding
// a check only tightens. A removed check obviously weakens; a modified one
// may — roady cannot tell whether a new command is stricter than the old, so
// it treats every modification as possibly weakening.
func (k CheckChangeKind) Weakens() bool { return k == CheckRemoved || k == CheckModified }

// RequirementCheckChange is a difference in one requirement's check.
type RequirementCheckChange struct {
	RequirementID string
	Kind          CheckChangeKind
	Old, New      *Check
}

// DiffChecks lists requirements whose check differs between old and next. A
// requirement present in only one version contributes nothing: its removal or
// addition is a change of scope, not of its check.
func DiffChecks(old, next *ProductSpec) []RequirementCheckChange {
	if old == nil || next == nil {
		return nil
	}
	before := map[string]*Check{}
	for _, f := range old.Features {
		for i := range f.Requirements {
			before[f.Requirements[i].ID] = f.Requirements[i].Check
		}
	}
	var out []RequirementCheckChange
	for _, f := range next.Features {
		for i := range f.Requirements {
			r := f.Requirements[i]
			prev, existed := before[r.ID]
			if !existed {
				continue
			}
			if kind, changed := compareChecks(prev, r.Check); changed {
				out = append(out, RequirementCheckChange{RequirementID: r.ID, Kind: kind, Old: prev, New: r.Check})
			}
		}
	}
	return out
}

func isZero(c *Check) bool { return c == nil || (c.Run == "" && c.Manual == "") }

func compareChecks(prev, next *Check) (CheckChangeKind, bool) {
	switch {
	case isZero(prev) && isZero(next):
		return "", false
	case isZero(prev):
		return CheckAdded, true
	case isZero(next):
		return CheckRemoved, true
	case prev.Run != next.Run || prev.Manual != next.Manual:
		return CheckModified, true
	}
	return "", false
}
