package mcp

import (
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

// Limits for roady_tasks.
//
// A tool result has to fit inside the caller's context, and roady_tasks is
// the tool most likely to be called at the start of a session — the moment a
// blown budget costs the most. A 96-task plan previously serialised to 84,000
// characters and had to be spilled to a file before it could be read, which
// is a poor showing for a system whose whole claim is surviving context
// resets.
const (
	// defaultTaskLimit is what a caller gets without asking. Large enough to
	// cover a normal working set, small enough that the answer is always
	// readable.
	defaultTaskLimit = 50

	// maxTaskLimit caps an explicit request, so asking for everything cannot
	// reproduce the original failure.
	maxTaskLimit = 200
)

// taskView is the projection roady_tasks returns.
//
// Descriptions are omitted unless asked for: they are the bulk of the payload
// and a caller listing tasks almost always wants to identify one, then read
// that one in full.
type taskView struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Priority   string   `json:"priority,omitempty"`
	Owner      string   `json:"owner,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
	IsBlocked  bool     `json:"is_blocked,omitempty"`
	IsUnlocked bool     `json:"is_unlocked,omitempty"`

	// Description is present only when the caller passed detail=true.
	Description string `json:"description,omitempty"`
}

// taskPage is one page of a task listing.
//
// The counts are not decoration. Returning a truncated list as a bare array
// tells the caller it has seen the whole plan, which is a worse failure than
// the oversized response it replaced.
type taskPage struct {
	// Status echoes the filter that produced this page.
	Status string `json:"status"`

	Tasks []taskView `json:"tasks"`

	// Total is how many tasks matched, before the page was cut.
	Total int `json:"total"`
	// Returned is how many are in this page.
	Returned int `json:"returned"`

	Offset int `json:"offset"`
	Limit  int `json:"limit"`

	// HasMore says another page exists.
	HasMore bool `json:"has_more"`

	// Hint tells the caller what to do about it, in words, because the
	// caller is usually a model.
	Hint string `json:"hint,omitempty"`
}

// paginateTasks projects and pages a task listing.
func paginateTasks(status string, tasks []project.TaskSummary, offset, limit int, detail bool) taskPage {
	if offset < 0 {
		offset = 0
	}
	switch {
	case limit <= 0:
		limit = defaultTaskLimit
	case limit > maxTaskLimit:
		limit = maxTaskLimit
	}

	total := len(tasks)
	page := taskPage{
		Status: status,
		Tasks:  []taskView{},
		Total:  total,
		Offset: offset,
		Limit:  limit,
	}

	if offset >= total {
		page.Returned = 0
		if total > 0 {
			page.Hint = fmt.Sprintf(
				"Offset %d is past the end of %d matching task(s). This is an empty page, not an empty plan.",
				offset, total)
		}
		return page
	}

	end := offset + limit
	if end > total {
		end = total
	}

	window := tasks[offset:end]
	page.Tasks = make([]taskView, 0, len(window))
	for _, t := range window {
		page.Tasks = append(page.Tasks, projectTask(t, detail))
	}
	page.Returned = len(page.Tasks)
	page.HasMore = end < total

	if page.HasMore {
		page.Hint = fmt.Sprintf(
			"Showing %d-%d of %d. Pass offset=%d for the next page, or narrow the listing with status or assignee.",
			offset+1, end, total, end)
	}
	if !detail && page.Returned > 0 {
		page.Hint = appendHint(page.Hint,
			"Descriptions are omitted from listings; pass detail=true, or read a single task, to get the full text.")
	}

	return page
}

func appendHint(existing, addition string) string {
	if existing == "" {
		return addition
	}
	return existing + " " + addition
}

func projectTask(t project.TaskSummary, detail bool) taskView {
	view := taskView{
		ID:         t.ID,
		Title:      t.Title,
		Status:     string(t.Status),
		Priority:   string(t.Priority),
		Owner:      t.Owner,
		DependsOn:  t.DependsOn,
		IsBlocked:  t.IsBlocked,
		IsUnlocked: t.IsUnlocked,
	}
	if detail {
		view.Description = t.Description
	}
	return view
}
