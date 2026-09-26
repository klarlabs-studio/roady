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

// Claude Code hooks. `roady setup claude-code` registers these in
// .claude/settings.json; each reads the hook's JSON on stdin.
//
// A hook must never get in the agent's way by failing: outside a roady
// project, or when roady itself errors, each exits 0 and says nothing (or
// says what went wrong as context), so a broken roady degrades to no roady.

// hookInput is the part of Claude Code's hook payload roady reads.
type hookInput struct {
	Cwd           string         `json:"cwd"`
	HookEventName string         `json:"hook_event_name"`
	Source        string         `json:"source"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
	ToolResponse  any            `json:"tool_response"`
}

var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Claude Code hook handlers (registered by `roady setup claude-code`)",
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
			_, _ = fmt.Fprint(cmd.OutOrStdout(), brief)
		}
		return nil
	},
}

var hookPlanApprovedCmd = &cobra.Command{
	Use:   "plan-approved",
	Short: "PostToolUse on ExitPlanMode: import the approved plan into roady",
	RunE: func(cmd *cobra.Command, args []string) error {
		in := readHookInput(cmd.InOrStdin())
		root, ok := hookProjectRoot(in)
		if !ok {
			return nil
		}
		msg := importApprovedPlan(root, in)
		if brief := hookBrief(cmd, root); brief != "" {
			msg += "\n\n" + brief
		}
		return writeHookContext(cmd.OutOrStdout(), "PostToolUse", msg)
	},
}

var hookGuardWriteCmd = &cobra.Command{
	Use:   "guard-write",
	Short: "PreToolUse on Write/Edit: redirect roadmap, TODO and plan markdown files to roady",
	RunE: func(cmd *cobra.Command, args []string) error {
		in := readHookInput(cmd.InOrStdin())
		root, ok := hookProjectRoot(in)
		if !ok {
			return nil
		}
		target := hookFilePath(in)
		if target == "" {
			return nil
		}
		var allow []string
		if pol, err := wiring.NewWorkspace(root).Repo.LoadPolicy(); err == nil && pol != nil {
			allow = pol.PlanFilesAllow
		}
		rel, blocked := isPlanFile(root, in.Cwd, target, allow)
		if !blocked {
			return nil
		}
		reason := fmt.Sprintf("Plans live in roady, not in %s. Record new work with `roady capture` "+
			"(or the roady_capture tool); to bring in a plan that is already written, `roady plan import <file>`. "+
			"If this file is kept on purpose, add %q to plan_files_allow in .roady/policy.yaml.", rel, rel)
		return writeHookJSON(cmd.OutOrStdout(), map[string]any{
			"hookEventName":            "PreToolUse",
			"permissionDecision":       "deny",
			"permissionDecisionReason": reason,
		})
	},
}

func readHookInput(r io.Reader) hookInput {
	var in hookInput
	raw, err := io.ReadAll(io.LimitReader(r, 8<<20))
	if err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &in)
	}
	return in
}

// hookProjectRoot finds the roady project the hook runs for: the session's
// cwd or CLAUDE_PROJECT_DIR (after --project), else the working directory. It
// walks up to the directory holding .roady, so a session started in a
// subdirectory still finds it.
func hookProjectRoot(in hookInput) (string, bool) {
	var starts []string
	if projectPath != "" {
		starts = append(starts, projectPath)
	}
	starts = append(starts, in.Cwd, os.Getenv("CLAUDE_PROJECT_DIR"))
	// The process's own directory only when Claude Code did not say where the
	// session is: a session in another project must not act on this one.
	if in.Cwd == "" && os.Getenv("CLAUDE_PROJECT_DIR") == "" {
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
		Capture(imp.Doc, application.CaptureOptions{Actor: "claude-code", Origin: planning.OriginAI})
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

// approvedPlanText finds the plan in an ExitPlanMode payload: the plan text
// in tool_input, or a plan file named in tool_input or tool_response.
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
		for _, key := range []string{"planFilePath", "plan_file_path", "filePath", "file_path"} {
			if p, ok := src[key].(string); ok && strings.HasSuffix(strings.ToLower(p), ".md") {
				if raw, err := os.ReadFile(filepath.Clean(p)); err == nil { // #nosec G304 -- the plan file Claude Code named
					return string(raw)
				}
			}
		}
	}
	return ""
}

func hookFilePath(in hookInput) string {
	for _, key := range []string{"file_path", "path"} {
		if p, ok := in.ToolInput[key].(string); ok && p != "" {
			return p
		}
	}
	return ""
}

var (
	planFileRoadmap = regexp.MustCompile(`(?i)^(roadmap|todo)([-_. ].*)?\.md$`)
	planFileName    = regexp.MustCompile(`(?i)(^|[-_. ])plans?([-_. ].*)?\.md$`)
)

// isPlanFile reports whether writing target would start a plan outside
// roady: a ROADMAP*/TODO*/*plan*.md file inside the project. Paths outside
// the project, under .roady or .claude (Claude Code keeps its own plans
// there), and paths matching allow are let through.
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
	if strings.HasPrefix(rel, ".roady/") || strings.HasPrefix(rel, ".claude/") {
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

// writeHookContext emits additional context for the agent in the JSON shape
// Claude Code reads from PostToolUse hooks.
func writeHookContext(w io.Writer, event, text string) error {
	return writeHookJSON(w, map[string]any{"hookEventName": event, "additionalContext": text})
}

// writeHookJSON writes a hookSpecificOutput object. Errors are swallowed: a
// hook that cannot answer lets the agent carry on.
func writeHookJSON(w io.Writer, specific map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]any{"hookSpecificOutput": specific})
	return nil
}

func init() {
	hookCmd.AddCommand(hookSessionStartCmd, hookPlanApprovedCmd, hookGuardWriteCmd)
	RootCmd.AddCommand(hookCmd)
}
