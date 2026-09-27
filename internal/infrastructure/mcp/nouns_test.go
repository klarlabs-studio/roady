package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/storage"
	mcpserver "go.klarlabs.de/mcp/server"
)

// The surface is one tool per CLI noun, plus the single-verb commands.
var surface = []string{
	"roady_audit", "roady_capture", "roady_drift", "roady_git", "roady_goal", "roady_init", "roady_next",
	"roady_plan", "roady_policy", "roady_query", "roady_spec", "roady_state", "roady_status", "roady_task",
}

func TestServer_ToolPerCLINoun(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	var got []string
	for _, tool := range server.mcpServer.Tools() {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(surface, ",") {
		t.Errorf("tools = %v\nwant %v", got, surface)
	}
	declared := append([]string(nil), SingleTools...)
	for tool := range NounActions {
		declared = append(declared, tool)
	}
	sort.Strings(declared)
	if strings.Join(declared, ",") != strings.Join(got, ",") {
		t.Errorf("NounActions + SingleTools = %v, registered %v", declared, got)
	}
}

// Every declared action reaches a handler: none falls through to "has no
// action". Arguments are empty, so many fail — but on their own terms.
func TestEveryNounActionDispatches(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	server.confirm = func(context.Context, string) error { return errors.New("declined in test") }
	ctx := context.Background()
	call := map[string]func(action string) (any, error){
		"roady_task": func(a string) (any, error) {
			return server.handleTask(ctx, TaskArgs{Action: a, TaskID: "t1", DryRun: true})
		},
		"roady_plan":   func(a string) (any, error) { return server.handlePlan(ctx, PlanArgs{Action: a}) },
		"roady_spec":   func(a string) (any, error) { return server.handleSpec(ctx, SpecArgs{Action: a}) },
		"roady_drift":  func(a string) (any, error) { return server.handleDrift(ctx, DriftArgs{Action: a}) },
		"roady_audit":  func(a string) (any, error) { return server.handleAudit(ctx, AuditArgs{Action: a, TaskID: "t1"}) },
		"roady_state":  func(a string) (any, error) { return server.handleState(ctx, StateArgs{Action: a}) },
		"roady_policy": func(a string) (any, error) { return server.handlePolicy(ctx, VerbArgs{Action: a}) },
		"roady_git":    func(a string) (any, error) { return server.handleGit(ctx, VerbArgs{Action: a}) },
		"roady_goal": func(a string) (any, error) {
			return server.handleGoal(ctx, GoalArgs{Action: a, GoalID: "goal-x", DryRun: true})
		},
	}
	for tool, actions := range NounActions {
		for _, action := range append(actions, "no-such-action") {
			res, err := call[tool](action)
			if err != nil {
				t.Errorf("%s %s: protocol error %v", tool, action, err)
				continue
			}
			text := resultText(res)
			if action == "no-such-action" {
				if !strings.Contains(text, "has no action") {
					t.Errorf("%s accepted an unknown action: %s", tool, text)
				}
				continue
			}
			if strings.Contains(text, "has no action") {
				t.Errorf("%s %s is declared but not handled", tool, action)
			}
		}
	}
}

// A gated operation changes nothing unless the user confirms, and says what
// to do instead.
func TestGatedActionsNeedTheUser(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	repo := storage.NewFilesystemRepository(server.root)
	plan, _ := repo.LoadPlan()
	plan.ApprovalStatus = planning.ApprovalPending
	_ = repo.SavePlan(plan)
	ctx := context.Background()

	// No client support for asking: the real elicitation path refuses.
	res, _ := server.handlePlan(ctx, PlanArgs{Action: "approve"})
	text := resultText(res)
	if !isToolError(res) || !strings.Contains(text, "roady plan approve") || !strings.Contains(text, "Nothing was changed") {
		t.Errorf("approve without a way to ask: %s", text)
	}
	if p, _ := repo.LoadPlan(); p.ApprovalStatus != planning.ApprovalPending {
		t.Fatal("the plan was approved without the user")
	}

	// The user declines.
	server.confirm = func(context.Context, string) error { return errors.New("you declined the request") }
	res, _ = server.handlePlan(ctx, PlanArgs{Action: "approve"})
	if !isToolError(res) || !strings.Contains(resultText(res), "declined") {
		t.Errorf("declined approve: %s", resultText(res))
	}
	if p, _ := repo.LoadPlan(); p.ApprovalStatus != planning.ApprovalPending {
		t.Fatal("a declined approval approved the plan")
	}

	// The user confirms: the plan is approved and the confirmation recorded.
	var asked string
	server.confirm = func(_ context.Context, msg string) error { asked = msg; return nil }
	res, _ = server.handlePlan(ctx, PlanArgs{Action: "approve"})
	if isToolError(res) {
		t.Fatalf("confirmed approve failed: %s", resultText(res))
	}
	if !strings.Contains(asked, "roady plan approve") {
		t.Errorf("the user was asked %q", asked)
	}
	if p, _ := repo.LoadPlan(); p.ApprovalStatus != planning.ApprovalApproved {
		t.Error("a confirmed approval did not approve the plan")
	}
	events, _ := repo.LoadEvents()
	found := false
	for _, e := range events {
		if e.Action == "approval.confirmed" && e.Metadata["operation"] == "plan approve" {
			found = true
		}
	}
	if !found {
		t.Error("the user's confirmation is not in the audit log")
	}

	// Every gated action asks; none runs silently.
	for tool, actions := range GatedActions {
		for _, action := range actions {
			asked = ""
			server.confirm = func(_ context.Context, msg string) error { asked = msg; return errors.New("no") }
			switch tool {
			case "roady_plan":
				_, _ = server.handlePlan(ctx, PlanArgs{Action: action})
			case "roady_spec":
				_, _ = server.handleSpec(ctx, SpecArgs{Action: action})
			case "roady_drift":
				_, _ = server.handleDrift(ctx, DriftArgs{Action: action})
			case "roady_state":
				_, _ = server.handleState(ctx, StateArgs{Action: action})
			}
			if asked == "" {
				t.Errorf("%s %s ran without asking the user", tool, action)
			}
		}
	}
}

func isToolError(res any) bool {
	r, ok := res.(*mcpserver.StructuredResult)
	return ok && r.IsError
}

// resultText renders any handler result as text for assertions.
func resultText(res any) string {
	if r, ok := res.(*mcpserver.StructuredResult); ok {
		var b strings.Builder
		for _, c := range r.Content {
			b.WriteString(c.Text)
		}
		return b.String()
	}
	raw, _ := json.Marshal(res)
	return string(raw)
}

// Goals go through capture: an agent can put work on the roadmap, move it
// between horizons and link features, and list the result.
func TestGoalToolRecordsTheRoadmap(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }

	res, _ := server.handleGoal(ctx, GoalArgs{Action: "add", Title: str("Offline mode"), Horizon: str("next")})
	if isToolError(res) {
		t.Fatalf("add: %s", resultText(res))
	}
	res, _ = server.handleGoal(ctx, GoalArgs{Action: "edit", GoalID: "goal-offline-mode", Horizon: str("sideways")})
	if !strings.Contains(resultText(res), "unknown horizon") {
		t.Errorf("a bad horizon should be rejected: %s", resultText(res))
	}
	res, _ = server.handleGoal(ctx, GoalArgs{Action: "edit", GoalID: "goal-offline-mode", Horizon: str("now")})
	if isToolError(res) {
		t.Fatalf("edit: %s", resultText(res))
	}
	res, _ = server.handleGoal(ctx, GoalArgs{Action: "edit", GoalID: "goal-nope", Horizon: str("now")})
	if !isToolError(res) {
		t.Errorf("editing a goal that does not exist should fail: %s", resultText(res))
	}
	res, _ = server.handleGoal(ctx, GoalArgs{Action: "list"})
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"name":"Now"`) || !strings.Contains(string(b), "goal-offline-mode") {
		t.Errorf("list = %s", b)
	}
}

// Rendering the roadmap over MCP writes ROADMAP.md; replacing a hand edit
// is a decision for the user.
func TestGoalRenderOverMCP(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()
	str := func(s string) *string { return &s }
	if res, _ := server.handleGoal(ctx, GoalArgs{Action: "add", Title: str("Offline mode"), Horizon: str("now")}); isToolError(res) {
		t.Fatal(resultText(res))
	}
	if res, _ := server.handleGoal(ctx, GoalArgs{Action: "render", DryRun: true}); !strings.Contains(resultText(res), "### Offline mode") {
		t.Errorf("dry run: %s", resultText(res))
	}
	if res, _ := server.handleGoal(ctx, GoalArgs{Action: "render"}); isToolError(res) {
		t.Fatalf("render: %s", resultText(res))
	}
	path := filepath.Join(server.root, "ROADMAP.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(path, append(raw, []byte("\nhand edit\n")...), 0o644)
	if res, _ := server.handleGoal(ctx, GoalArgs{Action: "render"}); !isToolError(res) {
		t.Errorf("a hand edit must not be replaced without force: %s", resultText(res))
	}
	server.confirm = func(context.Context, string) error { return errors.New("declined") }
	if res, _ := server.handleGoal(ctx, GoalArgs{Action: "render", Force: true}); !strings.Contains(resultText(res), "roady goal render --force") {
		t.Errorf("force without the user: %s", resultText(res))
	}
	if b, _ := os.ReadFile(path); !strings.Contains(string(b), "hand edit") {
		t.Error("a declined force still replaced the file")
	}
	server.confirm = func(context.Context, string) error { return nil }
	if res, _ := server.handleGoal(ctx, GoalArgs{Action: "render", Force: true}); isToolError(res) {
		t.Errorf("confirmed force: %s", resultText(res))
	}
	if b, _ := os.ReadFile(path); strings.Contains(string(b), "hand edit") {
		t.Error("a confirmed force left the hand edit in place")
	}
}

// An agent keeps its claim over MCP; another agent cannot renew it.
func TestRenewClaimOverMCP(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()
	repo := storage.NewFilesystemRepository(server.root)
	plan, _ := repo.LoadPlan()
	id := plan.Tasks[0].ID
	if res, _ := server.handleTask(ctx, TaskArgs{Action: "start", TaskID: id, Agent: "claude", SessionID: "s1"}); isToolError(res) {
		t.Fatalf("start: %s", resultText(res))
	}
	if res, _ := server.handleTask(ctx, TaskArgs{Action: "renew", TaskID: id, Agent: "claude", SessionID: "s1"}); isToolError(res) {
		t.Errorf("renew: %s", resultText(res))
	}
	if res, _ := server.handleTask(ctx, TaskArgs{Action: "renew", TaskID: id, Agent: "claude", SessionID: "s2"}); !isToolError(res) {
		t.Errorf("a different session renewed the claim: %s", resultText(res))
	}
	if res, _ := server.handleTask(ctx, TaskArgs{Action: "start", TaskID: id, Agent: "codex", SessionID: "s3"}); !strings.Contains(resultText(res), "claimed by claude") {
		t.Errorf("a second agent took the task: %s", resultText(res))
	}
}

func TestDriftDetectWithChecksOverMCP(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	res, _ := server.handleDrift(context.Background(), DriftArgs{Action: "detect", Checks: true})
	if isToolError(res) {
		t.Fatalf("detect with checks: %s", resultText(res))
	}
}

func TestBlockWithReasonOverMCP(t *testing.T) {
	server := setupCoordinatorTestServer(t)
	ctx := context.Background()
	repo := storage.NewFilesystemRepository(server.root)
	plan, _ := repo.LoadPlan()
	id := plan.Tasks[0].ID
	if res, _ := server.handleTask(ctx, TaskArgs{Action: "start", TaskID: id, Agent: "codex"}); isToolError(res) {
		t.Fatal(resultText(res))
	}
	if res, _ := server.handleTask(ctx, TaskArgs{Action: "block", TaskID: id, Reason: "spec-conflict"}); !isToolError(res) {
		t.Errorf("a conflict without a detail must be refused: %s", resultText(res))
	}
	res, _ := server.handleTask(ctx, TaskArgs{Action: "block", TaskID: id, Reason: "cannot-complete", Evidence: "no credentials", Agent: "codex"})
	if isToolError(res) || !strings.Contains(resultText(res), "A person will resolve it") {
		t.Fatalf("block: %s", resultText(res))
	}
	st, _ := server.handleStatus(ctx, StatusArgs{})
	if !strings.Contains(resultText(st), "Needs a decision (1)") {
		t.Errorf("status: %s", resultText(st))
	}
}
