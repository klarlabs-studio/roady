package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/spf13/cobra"
)

// Agent hooks. `roady setup <agent>` registers these in the agent's hook
// config; each reads the hook's JSON on stdin. --agent names whose payload
// and answer format to use, since agents agree on the events but not on the
// wire: Claude Code and Codex answer with hookSpecificOutput, Gemini CLI
// denies with a top-level decision, Cursor and Copilot have their own keys,
// and Kiro and the OpenCode plugin read plain text and exit codes.
//
// A hook must never get in the agent's way by failing: outside a roady
// project, or when roady itself errors, each exits 0 and says nothing (or
// says what went wrong as context), so a broken roady degrades to no roady.

// Agents whose hook dialects roady speaks.
const (
	agentClaude   = "claude"
	agentCodex    = "codex"
	agentGemini   = "gemini"
	agentCursor   = "cursor"
	agentCopilot  = "copilot"
	agentKiro     = "kiro"
	agentOpenCode = "opencode"
)

var hookAgent string

// hookInput is the part of a hook payload roady reads, normalised across
// agents.
type hookInput struct {
	Cwd           string         `json:"cwd"`
	HookEventName string         `json:"hook_event_name"`
	Source        string         `json:"source"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	ToolResponse  any            `json:"tool_response"`
}

// hookExit ends the process with a code; a variable so tests can observe it.
var hookExit = os.Exit

var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Agent hook handlers (registered by `roady setup <agent>`)",
	Hidden: true,
}

var hookSessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "SessionStart: put the current task brief into the agent's context",
	Long: `Prints the brief from 'roady next'. Registered for every SessionStart source,
including "compact", so the brief is back in context after compaction too.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		in := readHookInput(cmd.InOrStdin())
		root, ok := hookProjectRoot(in)
		if !ok {
			return nil
		}
		brief := hookBrief(cmd, root)
		if brief != "" {
			writeHookContext(cmd.OutOrStdout(), hookAgent, "SessionStart", brief)
		}
		return nil
	},
}

var hookPlanApprovedCmd = &cobra.Command{
	Use:   "plan-approved",
	Short: "After the agent's plan is approved (ExitPlanMode, exit_plan_mode): import it into roady",
	RunE: func(cmd *cobra.Command, args []string) error {
		in := readHookInput(cmd.InOrStdin())
		root, ok := hookProjectRoot(in)
		if !ok {
			return nil
		}
		if planRejected(in) {
			return nil
		}
		msg := importApprovedPlan(root, in)
		if brief := hookBrief(cmd, root); brief != "" {
			msg += "\n\n" + brief
		}
		event := "PostToolUse"
		if hookAgent == agentGemini {
			event = "AfterTool"
		}
		writeHookContext(cmd.OutOrStdout(), hookAgent, event, msg)
		return nil
	},
}

var hookGuardWriteCmd = &cobra.Command{
	Use:   "guard-write",
	Short: "Before a file write: redirect roadmap, TODO and plan markdown files to roady",
	RunE: func(cmd *cobra.Command, args []string) error {
		in := readHookInput(cmd.InOrStdin())
		root, ok := hookProjectRoot(in)
		if !ok {
			return nil
		}
		if !isWriteTool(in.ToolName) {
			return nil
		}
		var allow []string
		if pol, err := wiring.NewWorkspace(root).Repo.LoadPolicy(); err == nil && pol != nil {
			allow = pol.PlanFilesAllow
		}
		for _, target := range hookFilePaths(in) {
			rel, blocked := isPlanFile(root, in.Cwd, target, allow)
			if !blocked {
				continue
			}
			reason := fmt.Sprintf("Plans live in roady, not in %s. Record new work with `roady capture` "+
				"(or the roady_capture tool); to bring in a plan that is already written, `roady plan import <file>`. "+
				"If this file is kept on purpose, add %q to plan_files_allow in .roady/policy.yaml.", rel, rel)
			writeHookDeny(cmd.OutOrStdout(), cmd.ErrOrStderr(), hookAgent, reason)
			return nil
		}
		// An agent writing files is at work: keep its claims alive. This is
		// the heartbeat between briefs, and silent.
		hookHeartbeat(root)
		return nil
	},
}

func readHookInput(r io.Reader) hookInput {
	var in hookInput
	raw, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err != nil || len(raw) == 0 {
		return in
	}
	_ = json.Unmarshal(raw, &in)

	// Other agents' spellings of the same fields.
	var alt struct {
		ToolName       string   `json:"toolName"`
		ToolArgs       any      `json:"toolArgs"`
		WorkspaceRoots []string `json:"workspace_roots"`
		ToolResult     any      `json:"tool_result"`
		Response       any      `json:"toolResult"`
	}
	_ = json.Unmarshal(raw, &alt)
	if in.ToolName == "" {
		in.ToolName = alt.ToolName
	}
	if in.ToolInput == nil {
		switch args := alt.ToolArgs.(type) {
		case map[string]any:
			in.ToolInput = args
		case string: // Copilot sends the arguments as a JSON string.
			var m map[string]any
			if json.Unmarshal([]byte(args), &m) == nil {
				in.ToolInput = m
			}
		}
	}
	if in.Cwd == "" && len(alt.WorkspaceRoots) > 0 {
		in.Cwd = alt.WorkspaceRoots[0]
	}
	if in.ToolResponse == nil {
		in.ToolResponse = alt.ToolResult
	}
	if in.ToolResponse == nil {
		in.ToolResponse = alt.Response
	}
	return in
}

// hookProjectRoot finds the roady project the hook runs for: the session's
// cwd or the agent's project-dir variable (after --project), else the
// working directory. It
// walks up to the directory holding .roady, so a session started in a
// subdirectory still finds it.
func hookProjectRoot(in hookInput) (string, bool) {
	var starts []string
	if projectPath != "" {
		starts = append(starts, projectPath)
	}
	envDir := firstNonEmpty(os.Getenv("CLAUDE_PROJECT_DIR"), os.Getenv("GEMINI_PROJECT_DIR"), os.Getenv("CURSOR_PROJECT_DIR"))
	starts = append(starts, in.Cwd, envDir)
	// The process's own directory only when the agent did not say where the
	// session is: a session in another project must not act on this one.
	if in.Cwd == "" && envDir == "" {
		if wd, err := os.Getwd(); err == nil {
			starts = append(starts, wd)
		}
	}
	for _, s := range starts {
		if s == "" {
			continue
		}
		dir, err := filepath.Abs(s)
		if err != nil {
			continue
		}
		for {
			if info, err := os.Stat(filepath.Join(dir, ".roady")); err == nil && info.IsDir() {
				return dir, true
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", false
}

// hookHeartbeat renews the claims of whoever is working — the git identity,
// and ai-agent for work started over MCP.
func hookHeartbeat(root string) {
	ws := wiring.NewWorkspace(root)
	svc := application.NewTaskService(ws.Repo, ws.Audit, application.NewPolicyService(ws.Repo))
	owner := resolveCurrentOwner(gitConfigUserName)
	svc.Heartbeat(owner)
	if !sameIdentity(owner, "ai-agent") {
		svc.Heartbeat("ai-agent")
	}
}

// hookBrief is the brief for whoever is working: the git identity the CLI
// records, or ai-agent (what MCP transitions record) when that identity has
// nothing in progress but the agent does.
func hookBrief(cmd *cobra.Command, root string) string {
	ws := wiring.NewWorkspace(root)
	svc := application.NewTaskService(ws.Repo, ws.Audit, application.NewPolicyService(ws.Repo))
	owner := resolveCurrentOwner(gitConfigUserName)
	brief, err := svc.Brief(cmd.Context(), owner)
	if err != nil {
		return ""
	}
	if brief.Mode != "active" && !sameIdentity(owner, "ai-agent") {
		if agent, err := svc.Brief(cmd.Context(), "ai-agent"); err == nil && agent.Mode == "active" {
			brief = agent
		}
	}
	return brief.Render()
}

func sameIdentity(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// importApprovedPlan saves the plan ExitPlanMode carried and imports it, and
// returns what the agent should be told.
func importApprovedPlan(root string, in hookInput) string {
	text := approvedPlanText(in)
	if strings.TrimSpace(text) == "" {
		return "Roady: the approved plan was not in the hook payload, so it was not imported. " +
			"Record it with `roady capture` or `roady plan import <plan file>`."
	}
	saved, err := application.SavePlanText(root, text)
	if err != nil {
		return fmt.Sprintf("Roady could not save the approved plan (%v). Record it with `roady capture`.", err)
	}
	rel, _ := filepath.Rel(root, saved)
	rel = filepath.ToSlash(rel)

	ws := wiring.NewWorkspace(root)
	current, _ := ws.Repo.LoadSpec()
	imp, err := application.ImportPlanFile(saved, current, application.PlanImportOptions{Root: root})
	if err != nil {
		return fmt.Sprintf("Roady saved the approved plan to %s but found no steps to import (%v). "+
			"Record its tasks with `roady capture`.", rel, err)
	}
	result, err := application.NewCaptureService(ws.Repo, ws.Audit).
		Capture(imp.Doc, application.CaptureOptions{Actor: hookActor(), Origin: planning.OriginAI,
			Via: application.ViaPlanImportAuto, Note: "Imported approved plan " + rel})
	if err != nil {
		return fmt.Sprintf("Roady could not import the approved plan from %s: %v", rel, err)
	}
	if len(result.Rejected) > 0 {
		var reasons []string
		for _, r := range result.Rejected {
			reasons = append(reasons, r.Item+": "+r.Reason)
		}
		return fmt.Sprintf("Roady did not import the approved plan (%s); nothing was written: %s. "+
			"Fix it with `roady capture`.", rel, strings.Join(reasons, "; "))
	}

	var w strings.Builder
	switch {
	case !result.Changed():
		fmt.Fprintf(&w, "Roady: the approved plan %q is already recorded (%s); nothing changed.", imp.Title, rel)
	default:
		fmt.Fprintf(&w, "Roady imported the approved plan %q from %s: %d task(s) created, %d updated.",
			imp.Title, rel, countPrefix(result.Created, "task:"), countPrefix(result.Updated, "task:"))
	}
	if imp.Skipped > 0 {
		fmt.Fprintf(&w, " %d step(s) already checked off were skipped.", imp.Skipped)
	}
	if result.PlanApproval != "" && result.PlanApproval != string(planning.ApprovalApproved) {
		fmt.Fprintf(&w, " The roady plan is %s: ask the user to run `roady plan approve` before starting tasks.", result.PlanApproval)
	}
	w.WriteString(" Track this work in roady from here on — do not keep a separate plan or TODO file.")
	return w.String()
}

func countPrefix(items []string, prefix string) int {
	n := 0
	for _, it := range items {
		if strings.HasPrefix(it, prefix) {
			n++
		}
	}
	return n
}

// approvedPlanText finds the plan in a plan-approval payload: the plan text
// in tool_input (Claude Code's ExitPlanMode), or a plan file named in
// tool_input or tool_response (Gemini CLI's exit_plan_mode plan_path).
func approvedPlanText(in hookInput) string {
	if s, ok := in.ToolInput["plan"].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	sources := []map[string]any{in.ToolInput}
	if m, ok := in.ToolResponse.(map[string]any); ok {
		if s, ok := m["plan"].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
		sources = append(sources, m)
	}
	for _, src := range sources {
		for _, key := range []string{"planFilePath", "plan_file_path", "plan_path", "filePath", "file_path"} {
			if p, ok := src[key].(string); ok && strings.HasSuffix(strings.ToLower(p), ".md") {
				if raw, err := os.ReadFile(filepath.Clean(p)); err == nil { // #nosec G304 -- the plan file the agent named
					return string(raw)
				}
			}
		}
	}
	return ""
}

// hookFilePaths lists the files a tool call would write. Agents name the
// path differently (file_path, path, filePath, target_file, absolute_path),
// and Codex's apply_patch carries it inside the patch text.
func hookFilePaths(in hookInput) []string {
	var out []string
	for _, key := range []string{"file_path", "path", "filePath", "target_file", "absolute_path", "file"} {
		if p, ok := in.ToolInput[key].(string); ok && p != "" {
			out = append(out, p)
		}
	}
	for _, key := range []string{"command", "patch", "input"} {
		if s, ok := in.ToolInput[key].(string); ok && strings.Contains(s, "*** Begin Patch") {
			for _, m := range patchFile.FindAllStringSubmatch(s, -1) {
				out = append(out, strings.TrimSpace(m[1]))
			}
		}
	}
	return out
}

var patchFile = regexp.MustCompile(`(?m)^\*\*\* (?:Add File|Update File|Move to): (.+)$`)

// writeToolName matches the tools that write files across agents: Write,
// Edit, MultiEdit (Claude Code), apply_patch (Codex), write_file, replace
// (Gemini CLI), fs_write (Kiro), edit, create, write (Copilot, OpenCode).
var writeToolName = regexp.MustCompile(`(?i)^(write|edit|multiedit|str_?replace|apply_patch|write_file|replace|fs_write|create|create_file|edit_file|notebookedit)$`)

// isWriteTool reports whether the guard applies. An empty name means the
// agent's matcher already chose the tool.
func isWriteTool(name string) bool {
	return name == "" || writeToolName.MatchString(strings.TrimSpace(name))
}

// planRejected reports whether a plan-exit tool result says the user did
// not approve the plan (Gemini CLI runs AfterTool either way, with the
// user's feedback in the result).
func planRejected(in hookInput) bool {
	if in.ToolResponse == nil {
		return false
	}
	raw, _ := json.Marshal(in.ToolResponse)
	s := strings.ToLower(string(raw))
	return strings.Contains(s, "not approved") || strings.Contains(s, "rejected") || strings.Contains(s, "did not approve")
}

func hookActor() string {
	switch hookAgent {
	case "", agentClaude:
		return "claude-code"
	case agentGemini:
		return "gemini-cli"
	}
	return hookAgent
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// agentDirs are directories agents and roady keep their own files in.
var agentDirs = map[string]bool{
	".roady": true, ".claude": true, ".codex": true, ".gemini": true, ".cursor": true,
	".opencode": true, ".kiro": true, ".agents": true, ".github": true,
}

var (
	planFileRoadmap = regexp.MustCompile(`(?i)^(roadmap|todo)([-_. ].*)?\.md$`)
	planFileName    = regexp.MustCompile(`(?i)(^|[-_. ])plans?([-_. ].*)?\.md$`)
)

// isPlanFile reports whether writing target would start a plan outside
// roady: a ROADMAP*/TODO*/*plan*.md file inside the project. Paths outside
// the project, under .roady or an agent's own directory (where agents keep
// their plan-mode files), and paths matching allow are let through.
func isPlanFile(root, cwd, target string, allow []string) (string, bool) {
	if !filepath.IsAbs(target) {
		base := cwd
		if base == "" {
			base = root
		}
		target = filepath.Join(base, target)
	}
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if top, _, _ := strings.Cut(rel, "/"); agentDirs[top] && strings.Contains(rel, "/") {
		return rel, false
	}
	name := path.Base(rel)
	if !planFileRoadmap.MatchString(name) && !planFileName.MatchString(name) {
		return rel, false
	}
	for _, pattern := range allow {
		pattern = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(pattern)), "./")
		if dir, ok := strings.CutSuffix(pattern, "/**"); ok && strings.HasPrefix(rel, dir+"/") {
			return rel, false
		}
		if m, _ := path.Match(pattern, rel); m {
			return rel, false
		}
		if !strings.Contains(pattern, "/") {
			if m, _ := path.Match(pattern, name); m {
				return rel, false
			}
		}
	}
	return rel, true
}

// writeHookContext hands text to the agent as context, in its dialect.
func writeHookContext(w io.Writer, agent, event, text string) {
	switch agent {
	case agentCursor:
		writeJSONLine(w, map[string]any{"additional_context": text})
	case agentCopilot:
		writeJSONLine(w, map[string]any{"additionalContext": text})
	case agentKiro, agentOpenCode:
		_, _ = fmt.Fprint(w, text)
	case "", agentClaude:
		if event == "SessionStart" {
			_, _ = fmt.Fprint(w, text) // plain stdout is context on SessionStart
			return
		}
		writeJSONLine(w, map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": text}})
	default: // codex, gemini
		writeJSONLine(w, map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event, "additionalContext": text}})
	}
}

// writeHookDeny refuses the tool call with a reason the agent reads.
func writeHookDeny(out, errw io.Writer, agent, reason string) {
	switch agent {
	case agentGemini:
		writeJSONLine(out, map[string]any{"decision": "deny", "reason": reason})
	case agentCursor:
		writeJSONLine(out, map[string]any{"permission": "deny", "user_message": reason, "agent_message": reason})
	case agentCopilot:
		writeJSONLine(out, map[string]any{"permissionDecision": "deny", "permissionDecisionReason": reason})
	case agentKiro, agentOpenCode:
		// Exit 2 blocks the call; stderr is what the agent is told.
		_, _ = fmt.Fprintln(errw, reason)
		hookExit(2)
	default: // claude, codex
		writeJSONLine(out, map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		}})
	}
}

// writeJSONLine writes one JSON object. Errors are swallowed: a hook that
// cannot answer lets the agent carry on.
func writeJSONLine(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func init() {
	hookCmd.PersistentFlags().StringVar(&hookAgent, "agent", agentClaude, "Whose hook payload and answer format: claude, codex, gemini, cursor, copilot, kiro or opencode")
	hookCmd.AddCommand(hookSessionStartCmd, hookPlanApprovedCmd, hookGuardWriteCmd)
	RootCmd.AddCommand(hookCmd)
}
