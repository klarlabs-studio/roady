package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

// TaskBrief is the smallest useful statement of what an agent should be doing
// now: the active task, why it exists, what done means, and what it unblocks.
//
// It is meant to be pushed into an agent's context — at session start, after
// compaction, every few steps — rather than fetched on demand. Agents rarely
// call a recall tool on their own (in one 2026 harness study most runs never
// did), while re-showing the plan measurably improved plan adherence. So the
// brief is small enough to inject unconditionally.
type TaskBrief struct {
	Mode       string                `json:"mode"` // active | suggested | none
	Owner      string                `json:"owner,omitempty"`
	Task       *BriefTask            `json:"task,omitempty"`
	AlsoActive []BriefRef            `json:"also_active,omitempty"`
	Next       *BriefRef             `json:"next,omitempty"`
	Ready      int                   `json:"ready"`
	Blocked    int                   `json:"blocked"`
	PlanStatus string                `json:"plan_status,omitempty"`
	Remaining  int                   `json:"remaining"`
	Rules      []string              `json:"rules,omitempty"`
	LastCheck  *planning.CheckResult `json:"last_check,omitempty"`
}

// BriefTask is the task the brief is about.
type BriefTask struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Status    string          `json:"status"`
	Priority  string          `json:"priority,omitempty"`
	Why       string          `json:"why,omitempty"`
	Source    string          `json:"source,omitempty"`
	Check     *planning.Check `json:"check,omitempty"`
	DependsOn []BriefRef      `json:"depends_on,omitempty"`
	Unblocks  []string        `json:"unblocks,omitempty"`
}

// BriefRef names another task in the brief.
type BriefRef struct {
	ID     string `json:"id"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

// briefWhyLimit bounds the description so a brief stays within a small token
// budget however long the requirement text is.
const briefWhyLimit = 400

// Brief builds the brief for owner: their in-progress task if they have one,
// otherwise the ready task they should start next.
func (s *TaskService) Brief(ctx context.Context, owner string) (*TaskBrief, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	plan, err := s.repo.LoadPlan()
	if err != nil || plan == nil {
		return &TaskBrief{Mode: "none", Owner: owner}, nil
	}
	state, _ := s.repo.LoadState()
	summaries, err := s.coordinator.GetTaskSummaries(ctx)
	if err != nil {
		return nil, err
	}

	brief := &TaskBrief{Owner: owner, PlanStatus: string(plan.ApprovalStatus)}
	var mine, ready []project.TaskSummary
	for _, sum := range summaries {
		switch {
		case sum.Status == planning.StatusInProgress && sameOwner(sum.Owner, owner):
			mine = append(mine, sum)
		case sum.IsUnlocked && sum.Status.IsPending():
			ready = append(ready, sum)
		}
		if sum.Status == planning.StatusBlocked {
			brief.Blocked++
		}
		if sum.Status != planning.StatusDone && sum.Status != planning.StatusVerified {
			brief.Remaining++
		}
	}
	brief.Ready = len(ready)
	sortByPriority(mine)
	sortByPriority(ready)

	var focus *project.TaskSummary
	switch {
	case len(mine) > 0:
		brief.Mode = "active"
		focus = &mine[0]
		for _, other := range mine[1:] {
			brief.AlsoActive = append(brief.AlsoActive, BriefRef{ID: other.ID, Title: other.Title})
		}
		if len(ready) > 0 {
			brief.Next = &BriefRef{ID: ready[0].ID, Title: ready[0].Title}
		}
	case len(ready) > 0:
		brief.Mode = "suggested"
		focus = &ready[0]
		if len(ready) > 1 {
			brief.Next = &BriefRef{ID: ready[1].ID, Title: ready[1].Title}
		}
	default:
		brief.Mode = "none"
	}

	if focus != nil {
		task, _ := findTask(plan, focus.ID)
		bt := &BriefTask{
			ID:       focus.ID,
			Title:    focus.Title,
			Status:   string(focus.Status),
			Priority: string(focus.Priority),
			Why:      truncateRunes(strings.TrimSpace(task.Description), briefWhyLimit),
			Check:    task.Check,
		}
		if !task.Source.IsZero() {
			bt.Source = fmt.Sprintf("%s:%d", task.Source.Doc, task.Source.Line)
		}
		status := map[string]string{}
		titles := map[string]string{}
		for _, sum := range summaries {
			status[sum.ID] = string(sum.Status)
			titles[sum.ID] = sum.Title
		}
		for _, dep := range task.DependsOn {
			bt.DependsOn = append(bt.DependsOn, BriefRef{ID: dep, Status: status[dep]})
		}
		for _, other := range plan.Tasks {
			for _, dep := range other.DependsOn {
				if dep == focus.ID {
					bt.Unblocks = append(bt.Unblocks, other.ID)
				}
			}
		}
		brief.Task = bt
		if state != nil {
			if last, ok := state.TaskStates[focus.ID].LastCheck(); ok {
				brief.LastCheck = &last
			}
		}
		brief.Rules = []string{
			fmt.Sprintf("commit with [roady:%s] in the message, then `roady git sync`", focus.ID),
		}
		if task.Check != nil {
			brief.Rules = append(brief.Rules, fmt.Sprintf("prove it with `roady task check %s` before claiming done", focus.ID))
		}
	}
	brief.Rules = append(brief.Rules, "record new plans or tasks with `roady capture`, not in markdown files")
	return brief, nil
}

// Render formats the brief for injection into an agent's context.
func (b *TaskBrief) Render() string {
	var w strings.Builder
	switch b.Mode {
	case "none":
		w.WriteString("Roady: no task in progress and none ready.")
		if b.Remaining > 0 {
			fmt.Fprintf(&w, " %d task(s) remain, %d blocked; see `roady status`.", b.Remaining, b.Blocked)
		}
		if b.PlanStatus != "" && b.PlanStatus != string(planning.ApprovalApproved) {
			fmt.Fprintf(&w, " The plan is %s: it must be approved before work starts.", b.PlanStatus)
		}
		w.WriteString("\n")
		for _, r := range b.Rules {
			fmt.Fprintf(&w, "- %s\n", r)
		}
		return w.String()
	case "active":
		w.WriteString("Roady — current task\n")
	default:
		w.WriteString("Roady — no task in progress; start this next (`roady task start " + b.Task.ID + "`)\n")
	}
	t := b.Task
	fmt.Fprintf(&w, "%s [%s", t.ID, t.Status)
	if t.Priority != "" {
		fmt.Fprintf(&w, ", %s", t.Priority)
	}
	fmt.Fprintf(&w, "] %s\n", t.Title)
	if t.Why != "" {
		fmt.Fprintf(&w, "Why: %s\n", t.Why)
	}
	if t.Source != "" {
		fmt.Fprintf(&w, "Source: %s\n", t.Source)
	}
	switch {
	case t.Check != nil && t.Check.Run != "":
		fmt.Fprintf(&w, "Done when: `%s` passes", t.Check.Run)
	case t.Check != nil:
		fmt.Fprintf(&w, "Done when: a person confirms — %s", t.Check.Manual)
	default:
		w.WriteString("Done when: no acceptance check defined")
	}
	if b.LastCheck != nil {
		verdict := "failed"
		if b.LastCheck.Passed {
			verdict = "passed"
		}
		at := b.LastCheck.Commit
		if len(at) > 8 {
			at = at[:8]
		}
		fmt.Fprintf(&w, " (last run %s at %s)", verdict, at)
	}
	w.WriteString("\n")
	if len(t.DependsOn) > 0 {
		parts := make([]string, 0, len(t.DependsOn))
		for _, d := range t.DependsOn {
			parts = append(parts, fmt.Sprintf("%s (%s)", d.ID, d.Status))
		}
		fmt.Fprintf(&w, "Depends on: %s\n", strings.Join(parts, ", "))
	}
	if len(t.Unblocks) > 0 {
		fmt.Fprintf(&w, "Unblocks: %s\n", strings.Join(t.Unblocks, ", "))
	}
	if len(b.AlsoActive) > 0 {
		ids := make([]string, 0, len(b.AlsoActive))
		for _, a := range b.AlsoActive {
			ids = append(ids, a.ID)
		}
		fmt.Fprintf(&w, "Also in progress for you: %s\n", strings.Join(ids, ", "))
	}
	if b.Next != nil {
		fmt.Fprintf(&w, "Then: %s — %s\n", b.Next.ID, b.Next.Title)
	}
	for _, r := range b.Rules {
		fmt.Fprintf(&w, "- %s\n", r)
	}
	return w.String()
}

func sameOwner(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

var priorityRank = map[planning.TaskPriority]int{"high": 0, "medium": 1, "": 2, "low": 3}

// sortByPriority orders by priority, keeping plan order within a priority.
func sortByPriority(tasks []project.TaskSummary) {
	sort.SliceStable(tasks, func(i, j int) bool {
		return priorityRank[tasks[i].Priority] < priorityRank[tasks[j].Priority]
	})
}

func truncateRunes(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}
