package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A gated decision (plan approve) asks the user through MCP elicitation. On a
// stateless 2026-07-28 request that is an input_required result, and the spec
// requires its inputRequests to be an OBJECT keyed by request id, each value a
// {method, params} server request. roady 0.28.0 shipped with go.klarlabs.de/mcp
// v1.28.2, which wrote an ARRAY of {id, kind, payload}; Claude Code read that
// as no inputRequests at all and rejected the result ("input_required carries
// neither inputRequests nor requestState"), so approval over MCP never reached
// the user (issue #137). The unit test stubs the confirmation and never sees
// the wire, so this drives the built binary.
func TestMCPApproveAsksTheUserOnTheWire(t *testing.T) {
	bin := roadyBinary(t)
	for _, c := range []struct {
		answer string
		want   string
	}{
		{"accept", "approved"},
		{"decline", "pending"},
	} {
		dir := pendingPlanProject(t, bin)

		srv := startMCP(t, bin, dir)
		first := srv.call(t, 1, nil)
		var paused struct {
			ResultType    string `json:"resultType"`
			InputRequests map[string]struct {
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			} `json:"inputRequests"`
		}
		if err := json.Unmarshal(first, &paused); err != nil {
			t.Fatalf("approve result is not the spec shape (inputRequests must be an object keyed by id): %v\n%s", err, first)
		}
		if paused.ResultType != "input_required" || len(paused.InputRequests) != 1 {
			t.Fatalf("approve did not ask the user: %s", first)
		}
		var id string
		for k, r := range paused.InputRequests {
			id = k
			if r.Method != "elicitation/create" || !strings.Contains(string(r.Params), "roady plan approve") {
				t.Fatalf("input request %q = %s %s, want an elicitation naming the decision", k, r.Method, r.Params)
			}
		}

		// The client retries with the user's answer.
		srv.call(t, 2, map[string]any{id: map[string]any{
			"action":  c.answer,
			"content": map[string]any{"approve": c.answer == "accept"},
		}})
		srv.stop()

		out, err := exec.Command(bin, "status", "-C", dir).CombinedOutput()
		if err != nil {
			t.Fatalf("roady status: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "Plan status: "+c.want) {
			t.Errorf("after the user answered %q the plan is not %s:\n%s", c.answer, c.want, out)
		}
	}
}

func roadyBinary(t *testing.T) string {
	t.Helper()
	bin, _ := filepath.Abs("../../dist/roady")
	if _, err := os.Stat(bin); err != nil {
		t.Skip("dist/roady binary not found - run 'go build -o dist/roady ./cmd/roady' first")
	}
	return bin
}

// pendingPlanProject initializes a project whose plan awaits approval.
func pendingPlanProject(t *testing.T, bin string) string {
	t.Helper()
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.md")
	if err := os.WriteFile(plan, []byte("# Release\n\n## Tasks\n\n- [ ] Write the changelog\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "e2e"}, {"plan", "import", "plan.md"}} {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("roady %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

type mcpPeer struct {
	cmd *exec.Cmd
	in  *json.Encoder
	out *bufio.Scanner
}

func startMCP(t *testing.T, bin, dir string) *mcpPeer {
	t.Helper()
	cmd := exec.Command(bin, "mcp")
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	p := &mcpPeer{cmd: cmd, in: json.NewEncoder(stdin), out: sc}
	t.Cleanup(p.stop)
	return p
}

// call sends a stateless (2026-07-28) tools/call for plan approve, with the
// client's answers when given, and returns the raw result.
func (p *mcpPeer) call(t *testing.T, id int, inputResponses map[string]any) json.RawMessage {
	t.Helper()
	params := map[string]any{
		"name":      "roady_plan",
		"arguments": map[string]any{"action": "approve"},
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "e2e", "version": "0"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{"elicitation": map[string]any{}},
		},
	}
	if inputResponses != nil {
		params["inputResponses"] = inputResponses
	}
	if err := p.in.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": params}); err != nil {
		t.Fatal(err)
	}
	done := make(chan bool, 1)
	go func() { done <- p.out.Scan() }()
	select {
	case ok := <-done:
		if !ok {
			t.Fatalf("roady mcp closed its output: %v", p.out.Err())
		}
	case <-time.After(20 * time.Second):
		t.Fatal("roady mcp did not answer")
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(p.out.Bytes(), &resp); err != nil || resp.Error != nil {
		t.Fatalf("response %s: %v", p.out.Bytes(), err)
	}
	return resp.Result
}

func (p *mcpPeer) stop() {
	if p.cmd.Process != nil && p.cmd.ProcessState == nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
}
