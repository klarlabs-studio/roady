package application

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// Plan import turns the plan an agent harness already wrote into roady tasks.
//
// Every major harness now plans in markdown: Claude Code's plan mode
// (~/.claude/plans or plansDirectory), Cursor's plans, Gemini CLI, Devin, a
// Codex ExecPlan (PLANS.md), Kiro's .kiro/specs/<name>/tasks.md. Roady does
// not ask agents to plan differently; it takes what they produce. The result
// is a CaptureDoc, so an import is validated, all-or-nothing, idempotent and
// subject to the same approval rules as any other capture.

// Plan formats recognised by DetectPlanFormat.
const (
	PlanFormatKiro     = "kiro"
	PlanFormatExecPlan = "execplan"
	PlanFormatMarkdown = "markdown"
)

// PlanImportOptions controls how a plan becomes tasks.
type PlanImportOptions struct {
	Format string // empty or "auto" detects
	// FeatureID attaches the tasks to an existing feature — work within
	// approved intent. Empty creates a feature named after the plan, which is
	// new intent and needs approval like any other.
	FeatureID string
	// Parallel leaves steps independent. By default each step depends on the
	// one before it, since a plan is written in the order it is meant to run.
	Parallel bool
	// IncludeDone imports steps already checked off. They are skipped by
	// default: a finished step is history, and roady does not mark work done
	// without its own evidence.
	IncludeDone bool
	// Root, when set, makes the cited source path relative to the project, so
	// a task's doc:line reads the same on every machine.
	Root string
}

// PlanImport is what a plan file became.
type PlanImport struct {
	Format  string     `json:"format"`
	Title   string     `json:"title"`
	Doc     CaptureDoc `json:"doc"`
	Skipped int        `json:"skipped_done"`
	// SkippedBullets counts the plain list items not imported because the
	// plan marks its steps as a checklist (see markdownPlanSteps).
	SkippedBullets int `json:"skipped_bullets,omitempty"`
}

type planStep struct {
	title string
	desc  []string
	line  int
	done  bool
	// checkbox marks a "- [ ]" item, as opposed to a plain bullet.
	checkbox bool
	// parent is the index of the enclosing step (Kiro sub-tasks), or -1.
	parent int
}

var (
	planItem      = regexp.MustCompile(`^(\s*)(?:[-*+]|\d+[.)])\s+(?:\[([ xX\-~])\]\s*)?(.*)$`)
	planNumbering = regexp.MustCompile(`^(\d+(?:\.\d+)*)\.?\s+`)
	planTimestamp = regexp.MustCompile(`^\(\d{4}-\d{2}-\d{2}[^)]*\)\s*`)
	planPrefix    = regexp.MustCompile(`(?i)^(implementation\s+)?plan\s*[:—–-]\s*`)
	planStepLabel = regexp.MustCompile(`(?i)^step\s*\d+\s*[:.—–-]?\s*`)
)

// Sections of a generic plan that describe rather than prescribe.
var planNarrative = map[string]bool{
	"context": true, "background": true, "summary": true, "overview": true, "goal": true, "goals": true,
	"risks": true, "notes": true, "open questions": true, "questions": true, "assumptions": true,
	"verification": true, "testing": true, "test plan": true, "validation": true, "rollback": true,
	"out of scope": true, "non-goals": true, "references": true, "files": true, "files to change": true,
	"critical files": true, "purpose": true, "surprises & discoveries": true, "decision log": true,
	"outcomes & retrospective": true, "idempotence and recovery": true, "interfaces and dependencies": true,
	"artifacts and notes": true,
}

// Sections that hold the steps, preferred over list items elsewhere.
var planStepSections = map[string]bool{
	"steps": true, "implementation": true, "implementation steps": true, "plan": true, "tasks": true,
	"todo": true, "to do": true, "approach": true, "concrete steps": true, "plan of work": true,
	"implementation plan": true, "changes": true, "work": true, "milestones": true,
}

// DetectPlanFormat names the format of a plan file from its path and content.
func DetectPlanFormat(path string, lines []string) string {
	slashed := filepath.ToSlash(path)
	if strings.Contains(slashed, ".kiro/specs/") && strings.EqualFold(filepath.Base(path), "tasks.md") {
		return PlanFormatKiro
	}
	var hasProgress, hasExecSection bool
	numberedChecks := 0
	for _, l := range lines {
		t := strings.ToLower(strings.TrimSpace(strings.TrimLeft(l, "#")))
		if strings.HasPrefix(l, "## ") {
			switch t {
			case "progress":
				hasProgress = true
			case "decision log", "surprises & discoveries", "outcomes & retrospective":
				hasExecSection = true
			}
		}
		if m := planItem.FindStringSubmatch(l); m != nil && m[2] != "" && planNumbering.MatchString(m[3]) {
			numberedChecks++
		}
	}
	switch {
	case hasProgress && hasExecSection:
		return PlanFormatExecPlan
	case numberedChecks >= 2:
		return PlanFormatKiro
	}
	return PlanFormatMarkdown
}

// ImportPlanFile reads a plan file and returns the capture it amounts to.
func ImportPlanFile(path string, current *spec.ProductSpec, opts PlanImportOptions) (*PlanImport, error) {
	raw, err := os.ReadFile(filepath.Clean(path)) // #nosec G304 -- a plan file the caller named
	if err != nil {
		return nil, fmt.Errorf("read plan: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	format := opts.Format
	if format == "" || format == "auto" {
		format = DetectPlanFormat(path, lines)
	}

	title := PlanTitle(lines, path)
	var steps []planStep
	bullets := 0
	switch format {
	case PlanFormatKiro:
		steps = kiroSteps(lines)
	case PlanFormatExecPlan:
		steps = execPlanSteps(lines)
	case PlanFormatMarkdown:
		steps, bullets = markdownPlanSteps(lines)
	default:
		return nil, fmt.Errorf("unknown plan format %q (kiro, execplan or markdown)", format)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps found in %s (format %s)", path, format)
	}
	imp, err := buildPlanImport(format, title, citePath(path, opts.Root), steps, current, opts)
	if imp != nil {
		imp.SkippedBullets = bullets
	}
	return imp, err
}

// citePath is the path a task's source and the feature's description cite.
// Both are committed (plan.json, spec.yaml), so the citation must not name the
// machine it was imported on (#139): relative to root when the plan lives
// inside the project; "~/..." when it lives under the home directory (a plan
// in ~/.claude/plans reads the same to its author, without their user name);
// otherwise its file name alone, since a temp directory or another checkout
// points at nothing for anyone else. Roady only displays the citation.
func citePath(path, root string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return filepath.Base(path)
	}
	if root != "" {
		if rel, ok := within(root, absPath); ok {
			return rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, ok := within(home, absPath); ok {
			return "~/" + rel
		}
	}
	return filepath.Base(absPath)
}

// within returns absPath relative to dir, slash-separated, when it lies inside
// it.
func within(dir, absPath string) (string, bool) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// PlanTitle names a plan: its first "# " heading without a "Plan:" prefix, or
// the file name (the spec directory's, for Kiro's tasks.md).
func PlanTitle(lines []string, path string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, "# ") {
			t := cleanInline(strings.TrimPrefix(l, "# "))
			t = strings.TrimSpace(planPrefix.ReplaceAllString(t, ""))
			if t != "" && !strings.EqualFold(t, "plan") && !strings.EqualFold(t, "implementation plan") {
				return t
			}
		}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.EqualFold(base, "tasks") || strings.EqualFold(base, "plans") {
		// .kiro/specs/<name>/tasks.md: the spec's directory names it.
		base = filepath.Base(filepath.Dir(path))
	}
	return strings.ReplaceAll(base, "-", " ")
}

// collectItems reads list items under the given predicate on the section,
// folding nested lines into the item above. The predicate sees the heading
// path, outermost first: "### Docs" under "## Tasks" is ["tasks", "docs"], so
// a sub-heading belongs to the section it sits in. Reading the nearest heading
// alone dropped every checklist grouped under sub-headings of "## Tasks"
// (#138).
func collectItems(lines []string, inSection func(path []string) bool, nested bool) []planStep {
	var steps []planStep
	type heading struct {
		level int
		title string
	}
	var stack []heading
	path := func() []string {
		out := make([]string, len(stack))
		for i, h := range stack {
			out[i] = h.title
		}
		return out
	}
	section := []string{}
	inFence := false
	baseIndent := -1
	for i, l := range lines {
		if mdFence.MatchString(l) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := mdHeading.FindStringSubmatch(l); m != nil {
			level := len(m[1])
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{level, normalisedTitle(m[2])})
			section = path()
			baseIndent = -1
			continue
		}
		if !inSection(section) {
			continue
		}
		m := planItem.FindStringSubmatch(l)
		if m == nil {
			if len(steps) > 0 && strings.TrimSpace(l) != "" && (strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) {
				steps[len(steps)-1].desc = append(steps[len(steps)-1].desc, cleanInline(strings.TrimSpace(l)))
			}
			continue
		}
		indent := len(strings.ReplaceAll(m[1], "\t", "    "))
		if baseIndent < 0 || indent < baseIndent {
			baseIndent = indent
		}
		text := strings.TrimSpace(m[3])
		if indent > baseIndent && len(steps) > 0 && (!nested || m[2] == "") {
			steps[len(steps)-1].desc = append(steps[len(steps)-1].desc, cleanInline(text))
			continue
		}
		parent := -1
		if indent > baseIndent && nested {
			for p := len(steps) - 1; p >= 0; p-- {
				if steps[p].parent == -1 {
					parent = p
					break
				}
			}
		}
		steps = append(steps, planStep{
			title:    text,
			line:     i + 1,
			done:     m[2] == "x" || m[2] == "X",
			checkbox: m[2] != "",
			parent:   parent,
		})
	}
	return steps
}

// kiroSteps reads Kiro's tasks.md: numbered checkbox tasks, sub-tasks
// ("2.1") nested under their parent, and "_Requirements: 1.1_" lines kept as
// description.
func kiroSteps(lines []string) []planStep {
	steps := collectItems(lines, func([]string) bool { return true }, true)
	// A sub-task numbered "2.1" belongs to "2" even when Kiro writes it at the
	// same indent as its parent.
	byNumber := map[string]int{}
	for i := range steps {
		if n := planNumbering.FindStringSubmatch(steps[i].title); n != nil {
			byNumber[n[1]] = i
			if dot := strings.LastIndex(n[1], "."); dot > 0 {
				if p, ok := byNumber[n[1][:dot]]; ok && steps[i].parent == -1 {
					steps[i].parent = p
				}
			}
			steps[i].title = strings.TrimSpace(planNumbering.ReplaceAllString(steps[i].title, ""))
		}
	}
	return steps
}

// execPlanSteps reads a Codex ExecPlan: the Progress checklist, whose items
// carry timestamps, or — before any progress is recorded — the numbered steps
// of Concrete Steps / Plan of Work.
func execPlanSteps(lines []string) []planStep {
	steps := collectItems(lines, func(p []string) bool { return innermost(p) == "progress" }, false)
	if len(steps) == 0 {
		steps = collectItems(lines, func(p []string) bool { s := innermost(p); return s == "concrete steps" || s == "plan of work" }, false)
	}
	for i := range steps {
		steps[i].title = strings.TrimSpace(planTimestamp.ReplaceAllString(steps[i].title, ""))
	}
	return steps
}

// markdownPlanSteps reads a free-form plan (Claude Code, Cursor, Gemini CLI,
// Devin). A checklist is the strongest signal of what the steps are: a plan
// that writes "- [ ]" items means those, and plain bullets beside them (a
// milestones list, rules, notes) are commentary. So, in order: checklist items
// under a steps-like section; checklist items anywhere outside narrative
// sections; list items under a steps-like section; list items outside
// narrative sections; failing all of that, each sub-heading as a step with its
// text as description. When a checklist wins, it also returns how many plain
// list items outside narrative sections it left out, so the import can say so.
func markdownPlanSteps(lines []string) ([]planStep, int) {
	outsideNarrative := func(p []string) bool { return !planNarrative[innermost(p)] }
	for _, pass := range []struct {
		in        func([]string) bool
		checklist bool
	}{{inStepSection, true}, {outsideNarrative, true}, {inStepSection, false}, {outsideNarrative, false}} {
		steps := collectItems(lines, pass.in, false)
		if pass.checklist {
			steps = checklistOnly(steps)
		}
		if len(steps) > 0 {
			if !pass.checklist {
				return steps, 0
			}
			left := 0
			for _, st := range collectItems(lines, outsideNarrative, false) {
				if !st.checkbox {
					left++
				}
			}
			return steps, left
		}
	}
	var steps []planStep
	inFence := false
	for i, l := range lines {
		if mdFence.MatchString(l) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := mdHeading.FindStringSubmatch(l); m != nil && len(m[1]) >= 2 {
			title := headingTitle(m[2])
			if planNarrative[normalisedTitle(title)] {
				continue
			}
			title = strings.TrimSpace(planStepLabel.ReplaceAllString(title, ""))
			steps = append(steps, planStep{title: title, line: i + 1, parent: -1})
			continue
		}
		if len(steps) > 0 && strings.TrimSpace(l) != "" {
			steps[len(steps)-1].desc = append(steps[len(steps)-1].desc, cleanInline(strings.TrimSpace(l)))
		}
	}
	return steps, 0
}

// innermost is the nearest heading of a section path, "" before any heading.
func innermost(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return path[len(path)-1]
}

// inStepSection reports a section that holds steps: a steps-like heading, or a
// sub-heading of one that is not itself narrative ("### Notes" under "##
// Tasks" stays notes).
func inStepSection(path []string) bool {
	if planNarrative[innermost(path)] {
		return false
	}
	for _, t := range path {
		if planStepSections[t] {
			return true
		}
	}
	return false
}

// checklistOnly keeps the checklist items of steps, or none when it has none.
func checklistOnly(steps []planStep) []planStep {
	var out []planStep
	for _, st := range steps {
		if st.checkbox {
			out = append(out, st)
		}
	}
	return out
}

// hasFeature reports whether the spec has a feature with id.
func hasFeature(current *spec.ProductSpec, id string) bool {
	if current == nil {
		return false
	}
	for _, f := range current.Features {
		if f.ID == id {
			return true
		}
	}
	return false
}

func buildPlanImport(format, title, path string, steps []planStep, current *spec.ProductSpec, opts PlanImportOptions) (*PlanImport, error) {
	out := &PlanImport{Format: format, Title: title}
	planSlug := shortSlug(title)
	if planSlug == "" {
		planSlug = "plan"
	}

	featureID := opts.FeatureID
	if featureID != "" {
		if !hasFeature(current, featureID) {
			return nil, fmt.Errorf("feature %q does not exist", featureID)
		}
	} else {
		featureID = planSlug
		// A re-import of a plan whose feature exists leaves the feature alone.
		// Its "Imported from ..." description is provenance, not intent:
		// rewriting it on a re-import from another path marked the intent
		// changed and reset plan approval for a path-only change (#139).
		if !hasFeature(current, featureID) {
			t := title
			desc := fmt.Sprintf("Imported from %s (%s).", path, format)
			out.Doc.Features = append(out.Doc.Features, CaptureFeature{ID: featureID, Title: &t, Description: &desc})
		}
	}

	ids := make([]string, len(steps))
	used := map[string]bool{}
	for i, st := range steps {
		base := "task-" + planSlug + "-" + shortSlug(stepTitle(st.title))
		id := base
		for n := 2; used[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		used[id] = true
		ids[i] = id
	}

	prev := ""
	for i, st := range steps {
		if st.done && !opts.IncludeDone {
			out.Skipped++
			continue
		}
		title := stepTitle(st.title)
		deps := []string{}
		// Sub-tasks come before their parent: the parent is done when they are.
		for j, other := range steps {
			if other.parent == i && (!other.done || opts.IncludeDone) {
				deps = append(deps, ids[j])
			}
		}
		if !opts.Parallel && st.parent == -1 && prev != "" {
			deps = append(deps, prev)
		}
		fid := featureID
		t := CaptureTask{
			ID:        ids[i],
			Title:     &title,
			FeatureID: &fid,
			DependsOn: &deps,
			Source:    &planning.TaskSource{Doc: path, Line: st.line},
		}
		// The step's own line is only worth repeating when the title had to
		// shorten it; nested lines always carry.
		parts := st.desc
		if full := cleanInline(st.title); strings.TrimRight(full, ". ") != title {
			parts = append([]string{full}, parts...)
		}
		if desc := strings.TrimSpace(strings.Join(parts, "\n")); desc != "" {
			t.Description = &desc
		}
		out.Doc.Tasks = append(out.Doc.Tasks, t)
		if st.parent == -1 {
			prev = ids[i]
		}
	}
	if len(out.Doc.Tasks) == 0 {
		return nil, fmt.Errorf("every step in %s is already checked off; nothing to import (use --include-done to import them anyway)", path)
	}
	// A dependency on a skipped (done) step is satisfied; drop it.
	kept := map[string]bool{}
	for _, t := range out.Doc.Tasks {
		kept[t.ID] = true
	}
	for i := range out.Doc.Tasks {
		deps := (*out.Doc.Tasks[i].DependsOn)[:0]
		for _, d := range *out.Doc.Tasks[i].DependsOn {
			if kept[d] {
				deps = append(deps, d)
			}
		}
		*out.Doc.Tasks[i].DependsOn = deps
	}
	return out, nil
}

// stepTitle cleans a step into a task title: no markup, no trailing period,
// the first sentence of a long step.
func stepTitle(raw string) string {
	title, _ := splitItem(mdItem{text: raw})
	return title
}

// PlansDir is where plans that arrive as text (an approved Claude Code plan)
// are kept, so the doc:line their tasks cite resolves in the repository.
const PlansDir = ".roady/plans"

// SavePlanText stores a plan that arrived as text under root/.roady/plans,
// named after its title, and returns the path. Saving the same plan again
// overwrites it, so a revised plan re-imports onto the same tasks.
func SavePlanText(root, text string) (string, error) {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return "", fmt.Errorf("the plan is empty")
	}
	slug := shortSlug(PlanTitle(strings.Split(text, "\n"), "plan.md"))
	if slug == "" {
		slug = "plan"
	}
	dir := filepath.Join(root, filepath.FromSlash(PlansDir))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", PlansDir, err)
	}
	path := filepath.Join(dir, slug+".md")
	if err := os.WriteFile(path, []byte(text+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("save plan: %w", err)
	}
	return path, nil
}
