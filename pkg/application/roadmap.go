package application

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// The roadmap is the spec's goals, grouped the way a ROADMAP.md is read:
// what is on now, next and later, what shipped, and what was ruled out —
// each open goal with the features serving it and how far their tasks are.

// GoalView is one goal on the roadmap with the work linked to it.
type GoalView struct {
	spec.Goal
	Status   spec.GoalStatus `json:"status"`
	Features []string        `json:"features,omitempty"`
	// Tasks counts the plan tasks serving the goal; Done counts those done
	// or verified.
	Tasks int `json:"tasks"`
	Done  int `json:"done"`
}

// RoadmapSection is one heading of the roadmap.
type RoadmapSection struct {
	Name  string     `json:"name"`
	Goals []GoalView `json:"goals"`
}

// Roadmap is the spec's goals in reading order.
type Roadmap struct {
	Sections []RoadmapSection `json:"sections"`
	// Unlinked lists features that serve no goal.
	Unlinked []string `json:"unlinked,omitempty"`
}

// Section names, in the order they are rendered.
const (
	SectionNow        = "Now"
	SectionNext       = "Next"
	SectionLater      = "Later"
	SectionIdeas      = "Ideas"
	SectionShipped    = "Shipped"
	SectionOutOfScope = "Out of scope"
)

// BuildRoadmap groups the goals of sp and counts the work serving each.
// plan and state may be nil.
func BuildRoadmap(sp *spec.ProductSpec, plan *planning.Plan, state *planning.ExecutionState) Roadmap {
	if sp == nil {
		return Roadmap{Sections: []RoadmapSection{}}
	}
	// requirement -> goal, and feature -> goal, for counting tasks.
	reqGoal, featGoal := map[string]string{}, map[string]string{}
	features := map[string][]string{}
	var unlinked []string
	for _, f := range sp.Features {
		featGoal[f.ID] = f.Goal
		if f.Goal != "" {
			features[f.Goal] = append(features[f.Goal], f.ID)
		} else {
			unlinked = append(unlinked, f.ID)
		}
		for _, r := range f.Requirements {
			if g := sp.GoalOf(f, r); g != "" {
				reqGoal[r.ID] = g
			}
		}
	}
	tasks, done := map[string]int{}, map[string]int{}
	if plan != nil {
		for _, t := range plan.Tasks {
			// A requirement's own task is task-<requirement id>; its goal link
			// overrides the feature's.
			g := featGoal[t.FeatureID]
			if t.Goal != "" {
				g = t.Goal
			}
			if rid := strings.TrimPrefix(t.ID, "task-"); reqGoal[rid] != "" {
				g = reqGoal[rid]
			}
			if g == "" {
				continue
			}
			tasks[g]++
			if state != nil {
				if st := state.TaskStates[t.ID].Status; st == planning.StatusDone || st == planning.StatusVerified {
					done[g]++
				}
			}
		}
	}

	order := []string{SectionNow, SectionNext, SectionLater, SectionIdeas, SectionShipped, SectionOutOfScope}
	bySection := map[string][]GoalView{}
	for _, g := range sp.Goals {
		v := GoalView{Goal: g, Status: g.EffectiveStatus(), Features: features[g.ID], Tasks: tasks[g.ID], Done: done[g.ID]}
		bySection[sectionOf(g)] = append(bySection[sectionOf(g)], v)
	}
	// Shipped reads newest first, however the goals were recorded.
	sort.SliceStable(bySection[SectionShipped], func(i, j int) bool {
		return milestoneAfter(bySection[SectionShipped][i].Milestone, bySection[SectionShipped][j].Milestone)
	})
	rm := Roadmap{Sections: []RoadmapSection{}, Unlinked: unlinked}
	for _, name := range order {
		if goals := bySection[name]; len(goals) > 0 {
			rm.Sections = append(rm.Sections, RoadmapSection{Name: name, Goals: goals})
		}
	}
	return rm
}

func sectionOf(g spec.Goal) string {
	switch g.EffectiveStatus() {
	case spec.GoalShipped:
		return SectionShipped
	case spec.GoalOutOfScope:
		return SectionOutOfScope
	}
	switch g.Horizon {
	case spec.HorizonNow:
		return SectionNow
	case spec.HorizonNext:
		return SectionNext
	case spec.HorizonLater:
		return SectionLater
	}
	return SectionIdeas
}

// Render writes the roadmap as plain text.
func (r Roadmap) Render(w io.Writer) {
	if len(r.Sections) == 0 {
		_, _ = fmt.Fprintln(w, "No goals yet. Add one with `roady goal add \"<title>\" --horizon next`.")
	}
	for i, s := range r.Sections {
		if i > 0 {
			_, _ = fmt.Fprintln(w)
		}
		_, _ = fmt.Fprintf(w, "%s\n", s.Name)
		for _, g := range s.Goals {
			line := fmt.Sprintf("  %s  %s", g.ID, g.Title)
			var tags []string
			if g.Milestone != "" {
				tags = append(tags, g.Milestone)
			}
			if g.Status == spec.GoalIdea && s.Name != SectionIdeas {
				tags = append(tags, "idea")
			}
			if g.Tasks > 0 {
				tags = append(tags, fmt.Sprintf("%d/%d tasks done", g.Done, g.Tasks))
			}
			if len(tags) > 0 {
				line += "  (" + strings.Join(tags, ", ") + ")"
			}
			_, _ = fmt.Fprintln(w, line)
			if len(g.Features) > 0 {
				_, _ = fmt.Fprintf(w, "      features: %s\n", strings.Join(g.Features, ", "))
			}
		}
	}
	if len(r.Unlinked) > 0 {
		sorted := append([]string{}, r.Unlinked...)
		sort.Strings(sorted)
		// A project that adopts goals late has dozens of these; naming them
		// all buries the roadmap under one line.
		const show = 8
		if len(sorted) > show {
			_, _ = fmt.Fprintf(w, "\n%d features serve no goal: %s, … %d more\n  Link them with `roady goal edit <goal> --feature <id>`; `--json` lists all.\n",
				len(sorted), strings.Join(sorted[:show], ", "), len(sorted)-show)
			return
		}
		_, _ = fmt.Fprintf(w, "\nFeatures serving no goal: %s\n", strings.Join(sorted, ", "))
	}
}

// GoalEdit is a change to one goal from the single-goal commands. Nil
// fields stay as they are.
type GoalEdit struct {
	ID          string
	Title       *string
	Description *string
	Horizon     *string
	Status      *string
	Milestone   *string
	// Features are linked to the goal.
	Features []string
}

// AddGoalDoc builds the capture document for a new goal. The id is
// goal-<title> unless given; adding the same title again changes nothing.
func AddGoalDoc(sp *spec.ProductSpec, e GoalEdit) (CaptureDoc, string, error) {
	if e.Title == nil || strings.TrimSpace(*e.Title) == "" {
		return CaptureDoc{}, "", fmt.Errorf("a goal needs a title")
	}
	if strings.TrimSpace(e.ID) == "" {
		e.ID = "goal-" + spec.Slugify(*e.Title)
	}
	return EditGoalDoc(sp, e)
}

// EditGoalDoc builds the capture document changing an existing goal, or
// creating it when AddGoalDoc called it.
func EditGoalDoc(sp *spec.ProductSpec, e GoalEdit) (CaptureDoc, string, error) {
	id := strings.TrimSpace(e.ID)
	if id == "" {
		return CaptureDoc{}, "", fmt.Errorf("say which goal")
	}
	if sp != nil && sp.GoalIndex(id) < 0 && e.Title == nil {
		return CaptureDoc{}, "", fmt.Errorf("no goal %q; add it with `roady goal add`", id)
	}
	doc := CaptureDoc{Goals: []CaptureGoal{{
		ID: id, Title: e.Title, Description: e.Description,
		Horizon: e.Horizon, Status: e.Status, Milestone: e.Milestone,
	}}}
	for _, f := range e.Features {
		gid := id
		doc.Features = append(doc.Features, CaptureFeature{ID: f, Goal: &gid})
	}
	if sp != nil {
		for _, f := range e.Features {
			if featureIndex(sp, f) < 0 {
				return CaptureDoc{}, "", fmt.Errorf("no feature %q to link", f)
			}
		}
	}
	return doc, id, nil
}

// milestoneAfter orders milestones like "v0.22.x" and "v1.2.0" newest
// first, comparing their numeric parts; anything unnumbered sorts last.
func milestoneAfter(a, b string) bool {
	na, nb := milestoneNumbers(a), milestoneNumbers(b)
	if len(na) == 0 || len(nb) == 0 {
		return len(na) > len(nb)
	}
	for i := 0; i < len(na) && i < len(nb); i++ {
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}
	return len(na) > len(nb)
}

func milestoneNumbers(m string) []int {
	var out []int
	for _, part := range strings.FieldsFunc(strings.TrimPrefix(strings.ToLower(m), "v"), func(r rune) bool { return r == '.' || r == '-' }) {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		out = append(out, n)
	}
	return out
}
