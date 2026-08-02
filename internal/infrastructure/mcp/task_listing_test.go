package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
)

func summaries(n int) []project.TaskSummary {
	out := make([]project.TaskSummary, n)
	for i := range out {
		out[i] = project.TaskSummary{
			ID:          "task-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Title:       "Task title",
			Description: strings.Repeat("a long description that dominates the payload. ", 20),
			Status:      planning.StatusPending,
			Priority:    planning.TaskPriority("high"),
			Owner:       "alice",
		}
	}
	return out
}

// The reported failure: 96 tasks serialised to 84k characters and blew the
// client's tool-result budget. Descriptions were the bulk of it, and a caller
// listing tasks rarely wants them. See issue #75.
func TestPaginateTasksOmitsDescriptionsByDefault(t *testing.T) {
	page := paginateTasks("all", summaries(96), 0, 0, false)

	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "dominates the payload") {
		t.Error("descriptions were included in a default listing")
	}
	if len(body) > 20_000 {
		t.Errorf("default listing is %d bytes, want something an agent can hold", len(body))
	}
}

func TestPaginateTasksIncludesDescriptionsOnRequest(t *testing.T) {
	page := paginateTasks("ready", summaries(3), 0, 0, true)

	if len(page.Tasks) != 3 {
		t.Fatalf("got %d tasks, want 3", len(page.Tasks))
	}
	for _, task := range page.Tasks {
		if task.Description == "" {
			t.Errorf("task %s has no description with detail requested", task.ID)
		}
	}
}

func TestPaginateTasksAppliesDefaultLimit(t *testing.T) {
	page := paginateTasks("all", summaries(96), 0, 0, false)

	if page.Limit != defaultTaskLimit {
		t.Errorf("Limit = %d, want the default %d", page.Limit, defaultTaskLimit)
	}
	if len(page.Tasks) != defaultTaskLimit {
		t.Errorf("returned %d tasks, want %d", len(page.Tasks), defaultTaskLimit)
	}
	if page.Total != 96 {
		t.Errorf("Total = %d, want 96", page.Total)
	}
	if !page.HasMore {
		t.Error("HasMore = false with 96 tasks and a limit of 50")
	}
	// Truncating without saying so is worse than the original bug: the
	// caller believes it has seen the whole plan.
	if page.Hint == "" {
		t.Error("a truncated page carries no hint telling the caller how to get the rest")
	}
	if !strings.Contains(page.Hint, "offset") {
		t.Errorf("hint does not mention offset: %q", page.Hint)
	}
}

func TestPaginateTasksWalksPages(t *testing.T) {
	all := summaries(96)

	first := paginateTasks("all", all, 0, 40, false)
	second := paginateTasks("all", all, 40, 40, false)
	third := paginateTasks("all", all, 80, 40, false)

	if len(first.Tasks) != 40 || len(second.Tasks) != 40 || len(third.Tasks) != 16 {
		t.Fatalf("page sizes = %d, %d, %d; want 40, 40, 16",
			len(first.Tasks), len(second.Tasks), len(third.Tasks))
	}
	if third.HasMore {
		t.Error("the last page reports more to come")
	}
	if first.Tasks[0].ID == second.Tasks[0].ID {
		t.Error("the second page repeats the first")
	}
	if third.Offset != 80 {
		t.Errorf("Offset = %d, want 80", third.Offset)
	}
}

func TestPaginateTasksHandlesOutOfRangeInput(t *testing.T) {
	all := summaries(10)

	beyond := paginateTasks("ready", all, 500, 0, false)
	if len(beyond.Tasks) != 0 {
		t.Errorf("offset past the end returned %d tasks", len(beyond.Tasks))
	}
	if beyond.Total != 10 {
		t.Errorf("Total = %d, want 10 even when the page is empty", beyond.Total)
	}
	if beyond.Hint == "" {
		t.Error("an offset past the end should say so rather than look like an empty plan")
	}

	if got := paginateTasks("ready", all, -5, 0, false); got.Offset != 0 {
		t.Errorf("negative offset became %d, want 0", got.Offset)
	}
	if got := paginateTasks("ready", all, 0, -1, false); got.Limit != defaultTaskLimit {
		t.Errorf("negative limit became %d, want the default", got.Limit)
	}
	if got := paginateTasks("ready", all, 0, 10_000, false); got.Limit != maxTaskLimit {
		t.Errorf("oversized limit became %d, want the cap %d", got.Limit, maxTaskLimit)
	}
}

// A caller that asks for everything needs to know which bucket each task fell
// in, since the flat list no longer separates them.
func TestPaginateTasksCarriesStatus(t *testing.T) {
	tasks := summaries(2)
	tasks[0].Status = planning.StatusInProgress
	tasks[1].IsBlocked = true

	page := paginateTasks("all", tasks, 0, 0, false)

	if page.Tasks[0].Status != string(planning.StatusInProgress) {
		t.Errorf("status = %q, want in_progress", page.Tasks[0].Status)
	}
	if !page.Tasks[1].IsBlocked {
		t.Error("blocked flag was dropped")
	}
}

func TestPaginateTasksEmptyPlan(t *testing.T) {
	page := paginateTasks("ready", nil, 0, 0, false)

	if page.Total != 0 || len(page.Tasks) != 0 || page.HasMore {
		t.Errorf("empty plan produced %+v", page)
	}
	// Marshals as [] rather than null, so a client iterating the field does
	// not have to special-case it.
	body, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"tasks":[]`) {
		t.Errorf("empty task list did not marshal as []: %s", body)
	}
}
