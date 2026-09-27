package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/provenance"
)

// Adoption stats answer, from the local event log alone, whether roady is
// doing its three jobs on this project. Nothing leaves the machine.
//
//   - Captured: of the plans agents wrote, how many reached roady without
//     anyone typing a command (the plan-approved hook imported them).
//   - Proven: of the tasks verified, how many were verified on a passing
//     acceptance check rather than on someone's word.
//   - Resumed: of the sessions that began with work already in progress,
//     how many picked that work up first rather than starting something new.

// Ratio is a count out of a total.
type Ratio struct {
	Count int `json:"count"`
	Total int `json:"total"`
}

// Percent is the ratio as a percentage; -1 when there is nothing to count.
func (r Ratio) Percent() float64 {
	if r.Total == 0 {
		return -1
	}
	return float64(r.Count) / float64(r.Total) * 100
}

func (r Ratio) String() string {
	if r.Total == 0 {
		return "n/a (nothing to count yet)"
	}
	return fmt.Sprintf("%.0f%% (%d of %d)", r.Percent(), r.Count, r.Total)
}

// AdoptionStats are the three numbers.
type AdoptionStats struct {
	PlansCapturedAutomatically Ratio `json:"plans_captured_automatically"`
	VerifiedWithPassingCheck   Ratio `json:"verified_with_passing_check"`
	SessionsResumedRightTask   Ratio `json:"sessions_resumed_right_task"`
}

// ComputeAdoptionStats reads the three numbers from events.
func ComputeAdoptionStats(events []domain.Event) AdoptionStats {
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp.Before(events[j].Timestamp) })
	var st AdoptionStats

	// Captured.
	for _, e := range events {
		if e.Action != "plan.capture" {
			continue
		}
		switch metaString(e.Metadata, "via") {
		case ViaPlanImportAuto:
			st.PlansCapturedAutomatically.Count++
			st.PlansCapturedAutomatically.Total++
		case ViaPlanImport:
			st.PlansCapturedAutomatically.Total++
		}
	}

	// Proven: the latest verification of each task counts.
	proven := map[string]bool{}
	for _, e := range events {
		if e.Action == "task.transition" && metaString(e.Metadata, "event") == "verify" {
			proven[metaString(e.Metadata, "task_id")] = metaString(e.Metadata, "check_kind") != ""
		}
	}
	for _, ok := range proven {
		st.VerifiedWithPassingCheck.Total++
		if ok {
			st.VerifiedWithPassingCheck.Count++
		}
	}

	// Resumed: replay which tasks are in progress; for each session, look at
	// the first task it touches. Only sessions that span an agent's
	// conversation count: an MCP server process, or a CLI session the caller
	// named. A CLI session minted per invocation is one command, not a
	// resumption.
	inProgress := map[string]bool{}
	firstSeen := map[string]bool{}
	for _, e := range events {
		task := metaString(e.Metadata, "task_id")
		session := metaString(e.Metadata, "session_id")
		if !conversationSession(e.Metadata) {
			session = ""
		}
		if task != "" && session != "" && !firstSeen[session] {
			firstSeen[session] = true
			if len(inProgress) > 0 {
				st.SessionsResumedRightTask.Total++
				if inProgress[task] {
					st.SessionsResumedRightTask.Count++
				}
			}
		}
		if task == "" {
			continue
		}
		switch progressChange(e) {
		case "start":
			inProgress[task] = true
		case "stop":
			delete(inProgress, task)
		}
	}
	return st
}

// progressChange says whether an event puts a task in progress or takes it
// out, from either the transition or the coordinator's own event.
func progressChange(e domain.Event) string {
	switch e.Action {
	case "task.started":
		return "start"
	case "task.completed", "task.blocked", "task.claim_expired":
		return "stop"
	case "task.transition":
		switch metaString(e.Metadata, "event") {
		case "start":
			return "start"
		case "complete", "block", "stop", "verify":
			return "stop"
		}
	}
	return ""
}

// Stats computes the adoption numbers for this project.
func (s *TaskService) Stats() (AdoptionStats, error) {
	src, ok := s.repo.(historySource)
	if !ok {
		return AdoptionStats{}, fmt.Errorf("this repository keeps no event log")
	}
	events, err := src.LoadEvents()
	if err != nil {
		return AdoptionStats{}, err
	}
	return ComputeAdoptionStats(events), nil
}

// Render formats the stats for a terminal or an agent.
func (s AdoptionStats) Render() string {
	var b strings.Builder
	b.WriteString("Roady adoption (from this project's event log; nothing leaves the machine)\n")
	fmt.Fprintf(&b, "  Plans captured automatically:    %s\n", s.PlansCapturedAutomatically)
	fmt.Fprintf(&b, "  Verified with a passing check:   %s\n", s.VerifiedWithPassingCheck)
	fmt.Fprintf(&b, "  Sessions resumed the right task: %s\n", s.SessionsResumedRightTask)
	return b.String()
}

func conversationSession(meta map[string]any) bool {
	if given, _ := meta[provenance.KeySessionGiven].(bool); given {
		return true
	}
	return metaString(meta, provenance.KeySurface) == string(provenance.SurfaceMCP)
}
