package application

import (
	"fmt"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// GuardedCheckChange is a change to the acceptance check of work already
// started: the one edit that can make "done" easier to reach after the fact.
type GuardedCheckChange struct {
	TaskID string `json:"task_id"`
	Kind   string `json:"kind"`             // removed | modified
	Via    string `json:"via"`              // requirement | task
	Status string `json:"status"`           // the task's status when changed
	Old    string `json:"old,omitempty"`    // the check before, rendered
	New    string `json:"new,omitempty"`    // the check after, rendered
	Reopen bool   `json:"reopen,omitempty"` // done or verified work reopened by the change
}

// CheckChangeError refuses an unapproved weakening of a started task's check.
type CheckChangeError struct {
	Changes []GuardedCheckChange
}

func (e *CheckChangeError) Error() string {
	parts := make([]string, 0, len(e.Changes))
	for _, c := range e.Changes {
		parts = append(parts, fmt.Sprintf("%s (%s, %s check %s)", c.TaskID, c.Status, c.Via, c.Kind))
	}
	return "refused: this would change the acceptance check of work already started — " +
		strings.Join(parts, "; ") +
		". Loosening a check after the work began can make done easier to claim than to reach, so it needs a person: " +
		"re-run with --change-checks from the CLI (not available over MCP). Done or verified tasks are reopened and the change is recorded in the audit log."
}

// CheckGuard is the one gate for check changes. Every writer of the spec lock
// or the plan goes through it, because each is a way to make a loosened check
// stick: edit the task, edit the requirement, or re-lock the spec so a direct
// edit to spec.yaml stops showing as drift.
type CheckGuard struct {
	repo  domain.WorkspaceRepository
	audit domain.AuditLogger
}

func NewCheckGuard(repo domain.WorkspaceRepository, audit domain.AuditLogger) *CheckGuard {
	return &CheckGuard{repo: repo, audit: audit}
}

// Inspect lists the guarded changes between what is locked and planned now
// and nextSpec / nextPlan. Either may be nil to skip that side. Adding a check
// is never guarded, nor is any change to a task that has not started.
func (g *CheckGuard) Inspect(nextSpec *spec.ProductSpec, nextPlan *planning.Plan) []GuardedCheckChange {
	state, _ := g.repo.LoadState()
	status := func(taskID string) planning.TaskStatus {
		if state == nil {
			return planning.StatusPending
		}
		return state.GetTaskStatus(taskID)
	}
	seen := map[string]bool{}
	var out []GuardedCheckChange
	add := func(taskID, kind, via string, oldC, newC string) {
		st := status(taskID)
		if !st.Started() || seen[taskID] {
			return
		}
		seen[taskID] = true
		out = append(out, GuardedCheckChange{
			TaskID: taskID, Kind: kind, Via: via, Status: string(st), Old: oldC, New: newC,
			Reopen: st == planning.StatusDone || st == planning.StatusVerified,
		})
	}

	if nextSpec != nil {
		if lock, err := g.repo.LoadSpecLock(); err == nil && lock != nil {
			for _, c := range spec.DiffChecks(lock, nextSpec) {
				if c.Kind.Weakens() {
					add("task-"+c.RequirementID, string(c.Kind), "requirement", renderSpecCheck(c.Old), renderSpecCheck(c.New))
				}
			}
		}
	}
	if nextPlan != nil {
		if current, err := g.repo.LoadPlan(); err == nil && current != nil {
			for _, c := range planning.DiffTaskChecks(current, nextPlan) {
				if c.Weakens() {
					add(c.TaskID, c.Kind, "task", renderTaskCheck(c.Old), renderTaskCheck(c.New))
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskID < out[j].TaskID })
	return out
}

// Authorize refuses guarded changes unless allow is set.
func (g *CheckGuard) Authorize(changes []GuardedCheckChange, allow bool) error {
	if len(changes) > 0 && !allow {
		return &CheckChangeError{Changes: changes}
	}
	return nil
}

// Record reopens done or verified tasks whose check changed and logs every
// allowed change as an override, so the trail shows the goalposts moved and
// who moved them. Call it after the change is written.
func (g *CheckGuard) Record(changes []GuardedCheckChange, actor string) error {
	if len(changes) == 0 {
		return nil
	}
	state, err := g.repo.LoadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	reopened := false
	for _, c := range changes {
		if c.Reopen && state != nil {
			state.SetTaskStatus(c.TaskID, planning.StatusPending)
			reopened = true
		}
		if g.audit != nil {
			if err := g.audit.Log("task.check_changed", actor, map[string]any{
				"task_id":  c.TaskID,
				"kind":     c.Kind,
				"via":      c.Via,
				"status":   c.Status,
				"old":      c.Old,
				"new":      c.New,
				"reopened": c.Reopen,
				"override": true,
			}); err != nil {
				return fmt.Errorf("write audit log: %w", err)
			}
		}
	}
	if reopened {
		if err := g.repo.SaveState(state); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
	}
	return nil
}

func renderSpecCheck(c *spec.Check) string {
	if c == nil {
		return ""
	}
	return renderTaskCheck(&planning.Check{Run: c.Run, Manual: c.Manual})
}

func renderTaskCheck(c *planning.Check) string {
	switch {
	case c.IsZero():
		return ""
	case c.Run != "":
		return "run: " + c.Run
	default:
		return "manual: " + c.Manual
	}
}
