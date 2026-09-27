package application

import (
	"fmt"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// Decide is one `roady decide`: a decision and what it applies to.
type Decide struct {
	ID           string // default decision-<title>
	Title        string
	Choice       string
	Context      string
	Consequences string
	Goals        []string
	Features     []string
	Requirements []string
	Supersedes   string
}

// DecideDoc builds the capture document recording d.
func DecideDoc(sp *spec.ProductSpec, d Decide) (CaptureDoc, string, error) {
	title, choice := strings.TrimSpace(d.Title), strings.TrimSpace(d.Choice)
	if title == "" || choice == "" {
		return CaptureDoc{}, "", fmt.Errorf("a decision needs a title and a choice (--choice)")
	}
	id := strings.TrimSpace(d.ID)
	if id == "" {
		id = "decision-" + spec.Slugify(title)
	}
	cd := CaptureDecision{ID: id, Title: &title, Choice: &choice}
	if d.Context != "" {
		cd.Context = strPtr(d.Context)
	}
	if d.Consequences != "" {
		cd.Consequences = strPtr(d.Consequences)
	}
	if len(d.Goals) > 0 {
		g := uniqueStrings(d.Goals)
		cd.Goals = &g
	}
	if len(d.Features) > 0 {
		f := uniqueStrings(d.Features)
		cd.Features = &f
	}
	if len(d.Requirements) > 0 {
		r := uniqueStrings(d.Requirements)
		cd.Requirements = &r
	}
	if d.Supersedes != "" {
		cd.Supersedes = strPtr(d.Supersedes)
	}
	return CaptureDoc{Decisions: []CaptureDecision{cd}}, id, nil
}

// BriefDecision is a decision shown in a task brief.
type BriefDecision struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Choice string `json:"choice"`
	Date   string `json:"date,omitempty"`
}

// briefDecisionLimit keeps the brief small: the newest few are the ones
// most likely to be forgotten.
const briefDecisionLimit = 3

// decisionsForTask returns the standing decisions that apply to task: via
// its requirement (task-<id>), its feature, or its goal.
func decisionsForTask(sp *spec.ProductSpec, task planning.Task) []BriefDecision {
	if sp == nil || len(sp.Decisions) == 0 {
		return nil
	}
	reqID := ""
	goal := task.Goal
	for _, f := range sp.Features {
		if f.ID == task.FeatureID && goal == "" {
			goal = f.Goal
		}
		for _, r := range f.Requirements {
			if "task-"+r.ID == task.ID {
				reqID = r.ID
				if r.Goal != "" {
					goal = r.Goal
				}
			}
		}
	}
	var out []BriefDecision
	for _, d := range sp.DecisionsFor(task.FeatureID, reqID, goal) {
		if len(out) == briefDecisionLimit {
			break
		}
		out = append(out, BriefDecision{ID: d.ID, Title: d.Title, Choice: truncateRunes(d.Choice, 160), Date: d.Date})
	}
	return out
}
