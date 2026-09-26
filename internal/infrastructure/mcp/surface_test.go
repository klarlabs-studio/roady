package mcp

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"go.klarlabs.de/mcp/protocol"
)

func TestResolveSurface(t *testing.T) {
	cases := []struct {
		profile  string
		listed   []string
		unlisted []string
		allReg   bool
		wantErr  bool
	}{
		{profile: "", listed: []string{"roady_next", "roady_capture", "roady_task_transition"}, unlisted: []string{"roady_plan_get", "roady_debt_report"}, allReg: true},
		{profile: "essential", listed: []string{"roady_status"}, unlisted: []string{"roady_spec_get"}, allReg: true},
		{profile: "essential,debt", listed: []string{"roady_next", "roady_debt_report"}, unlisted: []string{"roady_cost_report", "roady_plan_get"}, allReg: true},
		{profile: "essential,core", listed: []string{"roady_plan_get"}, unlisted: []string{"roady_debt_report"}, allReg: true},
		{profile: "essential,dept", wantErr: true},
		{profile: "all", allReg: true},
	}
	for _, c := range cases {
		got, err := resolveSurface(c.profile)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: want an error", c.profile)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", c.profile, err)
		}
		if c.allReg && len(got.groups) != len(allGroups) {
			t.Errorf("%q: every group stays registered (callable), got %v", c.profile, got.groups)
		}
		if c.profile == "all" {
			if got.listed != nil {
				t.Error("all lists everything")
			}
			continue
		}
		for _, n := range c.listed {
			if !got.listed(n) {
				t.Errorf("%q: %s should be listed", c.profile, n)
			}
		}
		for _, n := range c.unlisted {
			if got.listed(n) {
				t.Errorf("%q: %s should not be listed", c.profile, n)
			}
		}
	}

	// A group profile keeps its old meaning: only those groups exist.
	got, err := resolveSurface("core")
	if err != nil || got.listed != nil || got.groups[groupDebt] {
		t.Errorf("core: %+v %v", got, err)
	}
}

func TestEssentialToolsExist(t *testing.T) {
	registered := map[string]bool{}
	for _, n := range registeredToolNames(t) {
		registered[n] = true
	}
	for n := range essentialTools {
		if !registered[n] {
			t.Errorf("essential tool %s is not registered", n)
		}
	}
}

func TestListFilterOnlyTrimsToolsList(t *testing.T) {
	listResult := map[string]any{"tools": []any{
		map[string]any{"name": "roady_next"}, map[string]any{"name": "roady_plan_get"},
	}}
	next := func(_ context.Context, req *protocol.Request) (*protocol.Response, error) {
		return &protocol.Response{Result: listResult}, nil
	}
	h := listFilter(func(n string) bool { return n == "roady_next" })(next)

	resp, _ := h(context.Background(), &protocol.Request{Method: protocol.MethodToolsList})
	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), "roady_next") || strings.Contains(string(raw), "roady_plan_get") {
		t.Errorf("tools/list = %s", raw)
	}
	resp, _ = h(context.Background(), &protocol.Request{Method: "tools/call"})
	if raw, _ := json.Marshal(resp.Result); !strings.Contains(string(raw), "roady_plan_get") {
		t.Errorf("other methods must pass through: %s", raw)
	}
	if resp, _ := listFilter(nil)(next)(context.Background(), &protocol.Request{Method: protocol.MethodToolsList}); resp.Result == nil {
		t.Error("nil filter passes through")
	}
}

// The done-when of task-slim-mcp: the default tools/list stays under 3k
// tokens, and a tool that is not listed is still callable.
func TestDefaultSurfaceOnTheWire(t *testing.T) {
	t.Setenv("ROADY_MCP_TOOLS", "")
	dir := t.TempDir()
	s, err := NewServer(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.handleInit(context.Background(), InitArgs{Name: "surface"}); err != nil {
		t.Fatal(err)
	}
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.ServeHTTP(ctx, addr) }()
	c := &wireClient{url: "http://" + addr + "/mcp"}
	c.waitReady(t)

	var list struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	raw := c.call(t, "tools/list", map[string]any{})
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	var names []string
	size := 0
	for _, tool := range list.Result.Tools {
		var m struct{ Name string }
		_ = json.Unmarshal(tool, &m)
		names = append(names, m.Name)
		var compact map[string]any
		_ = json.Unmarshal(tool, &compact)
		b, _ := json.Marshal(compact)
		size += len(b)
	}
	sort.Strings(names)
	if len(names) != len(essentialTools) {
		t.Errorf("default tools/list = %v, want the %d essential tools", names, len(essentialTools))
	}
	// ~3.3 characters per token for JSON schemas: 9,900 characters ≈ 3k tokens.
	if size > 9900 {
		t.Errorf("default tools/list is %d characters (~%d tokens); the budget is 3k tokens", size, size*10/33)
	}

	raw = c.call(t, "tools/call", map[string]any{"name": "roady_plan_get", "arguments": map[string]any{}})
	var call struct {
		Result *struct {
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.Unmarshal(raw, &call); err != nil || call.Error != nil || call.Result == nil {
		t.Errorf("an unlisted tool must stay callable: %s", raw)
	}
}
