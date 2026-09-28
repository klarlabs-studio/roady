package application

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// A roadmap kept by hand usually has the shape roady renders: a heading per
// horizon (Now, Next, Later, Done…) and under each either ### headings or
// top-level bullets, one per goal. ImportRoadmapMarkdown reads that shape
// into goals, so a project that planned in ROADMAP.md (or memory/roadmap.md)
// can move it into roady in one step instead of retyping it. It is a parser,
// not a reader of prose: anything outside a recognised section is skipped
// and reported.

// ViaRoadmapImport is recorded on the capture event of a roadmap import.
const ViaRoadmapImport = "roadmap-import"

// RoadmapImport is what a roadmap file yielded.
type RoadmapImport struct {
	Doc CaptureDoc `json:"doc"`
	// Skipped names the headings whose content was not read.
	Skipped []string `json:"skipped,omitempty"`
}

type importSection struct {
	horizon string
	status  string
	// whole makes the section itself one goal, titled by its heading,
	// instead of reading goals from its ### headings or bullets.
	whole bool
}

// SectionMapping tells the importer where a ## section's goals go, for a
// roadmap organised by something other than horizons (phases, quarters).
// Prefix matches the start of a heading, ignoring case; Target is a horizon
// or status word as a section would be named (now, next, later, ideas, done,
// shipped, out of scope). Whole makes each matching section one goal.
type SectionMapping struct {
	Prefix string
	Target string
	Whole  bool
}

// ParseSectionMapping reads "Prefix=target", as the CLI and MCP take it.
func ParseSectionMapping(s string, whole bool) (SectionMapping, error) {
	prefix, target, ok := strings.Cut(s, "=")
	prefix, target = strings.TrimSpace(prefix), strings.TrimSpace(target)
	if !ok || prefix == "" || target == "" {
		return SectionMapping{}, fmt.Errorf("section mapping %q: want \"Heading prefix=horizon\", e.g. \"Phase 4=now\"", s)
	}
	if _, known := sectionFor(target); !known {
		return SectionMapping{}, fmt.Errorf("section mapping %q: %q is not a horizon or status; use now, next, later, ideas, done (shipped) or out of scope", s, target)
	}
	return SectionMapping{Prefix: prefix, Target: target, Whole: whole}, nil
}

// RoadmapImportOptionsFrom builds options from "Prefix=target" strings:
// sections read their goals as a horizon section would, sectionGoals make
// each matching section one goal.
func RoadmapImportOptionsFrom(sections, sectionGoals []string) (RoadmapImportOptions, error) {
	var opts RoadmapImportOptions
	for _, group := range []struct {
		values []string
		whole  bool
	}{{sections, false}, {sectionGoals, true}} {
		for _, v := range group.values {
			m, err := ParseSectionMapping(v, group.whole)
			if err != nil {
				return opts, err
			}
			opts.Sections = append(opts.Sections, m)
		}
	}
	return opts, nil
}

// NoGoalsError explains an import that found no goal. A roadmap organised
// some other way has sections, just not horizons: naming them shows what to
// map.
func NoGoalsError(file string, skipped []string) error {
	const want = "goals go under ## Now, Next, Later, Ideas, Done (or Shipped) or Out of scope, a ### heading or bullet each"
	if len(skipped) == 0 {
		return fmt.Errorf("no goals found in %s: %s", file, want)
	}
	named := skipped
	more := ""
	if len(named) > 5 {
		named, more = named[:5], fmt.Sprintf(", … %d more", len(skipped)-5)
	}
	return fmt.Errorf("no goals found in %s: its sections (%s%s) are not horizons; %s. Map them with --section \"<heading prefix>=now\", make each one goal with --section-goal \"<prefix>=shipped\", or add goals with `roady goal add`",
		file, strings.Join(named, ", "), more, want)
}

// RoadmapImportOptions adjust how a roadmap's sections are read.
type RoadmapImportOptions struct {
	Sections []SectionMapping
}

// sectionOf resolves a heading: the longest matching mapping first, then the
// built-in horizon names. used records which mappings matched.
func (o RoadmapImportOptions) sectionOf(heading string, used map[int]bool) (importSection, bool) {
	best := -1
	h := strings.ToLower(strings.TrimSpace(heading))
	for i, m := range o.Sections {
		if strings.HasPrefix(h, strings.ToLower(m.Prefix)) && (best < 0 || len(m.Prefix) > len(o.Sections[best].Prefix)) {
			best = i
		}
	}
	if best >= 0 {
		used[best] = true
		s, _ := sectionFor(o.Sections[best].Target)
		s.whole = o.Sections[best].Whole
		return s, true
	}
	return sectionFor(heading)
}

// sectionFor maps a ## heading to where its goals go; ok is false for a
// heading that is not a roadmap section.
func sectionFor(heading string) (importSection, bool) {
	h := strings.ToLower(strings.TrimSpace(heading))
	// "Now (Q3)", "Next — after launch", "Later: someday"
	if i := strings.IndexAny(h, "(:—–"); i > 0 {
		h = strings.TrimSpace(h[:i])
	}
	h = strings.Trim(h, " *_")
	switch h {
	case "now", "current", "in progress", "doing":
		return importSection{horizon: "now"}, true
	case "next", "up next", "soon":
		return importSection{horizon: "next"}, true
	case "later", "future", "someday":
		return importSection{horizon: "later"}, true
	case "ideas", "idea", "icebox", "maybe", "backlog":
		return importSection{status: string(spec.GoalIdea)}, true
	case "done", "shipped", "released", "completed", "complete":
		return importSection{status: string(spec.GoalShipped)}, true
	case "out of scope", "not doing", "won't do", "wont do", "non-goals", "non goals", "rejected":
		return importSection{status: string(spec.GoalOutOfScope)}, true
	}
	return importSection{}, false
}

var (
	wikiLink       = regexp.MustCompile(`\[\[([^\]|]+)(\|[^\]]*)?\]\]`)
	milestoneParen = regexp.MustCompile(`\s*\((v?\d[\w.\-]*(?:\.x)?)\)\s*$`)
	leadingBold    = regexp.MustCompile(`^\*\*(.+?)\*\*[\s.:]*(?:[—–-]\s*)?(.*)$`)
	bulletLine     = regexp.MustCompile(`^([-*+]|\d+[.)])\s+(.*)$`)
)

// roadmapInline drops wiki links and leading decoration (★) from a line of
// a roadmap, keeping emphasis so a bold lead can still be found.
func roadmapInline(s string) string {
	s = wikiLink.ReplaceAllString(s, "$1")
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "★☆✓✔✅⭐🚀 "))
}

// splitTitle takes a goal's title from the start of a bullet: its bold lead
// if it has one, else the text before a dash or colon, else the first
// sentence. The rest is the description.
func splitTitle(text string) (string, string) {
	text = roadmapInline(text)
	if m := leadingBold.FindStringSubmatch(text); m != nil {
		return strings.Trim(cleanInline(roadmapInline(m[1])), " .:"), cleanInline(m[2])
	}
	text = cleanInline(text)
	for _, sep := range []string{" — ", " – ", " - ", ": "} {
		i := strings.Index(text, sep)
		// "Phase 4: Nexa Finance beyond…" — a head of a word or two is a
		// label, not a title; keep reading.
		if i > 0 && (sep != ": " || len(strings.Fields(text[:i])) >= 3) {
			return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+len(sep):])
		}
	}
	if i := strings.Index(text, ". "); i > 0 {
		return strings.TrimSpace(text[:i]), strings.TrimSpace(text[i+2:])
	}
	return strings.TrimRight(text, "."), ""
}

// splitMilestone takes a trailing "(v1.2)" off a title.
func splitMilestone(title string) (string, string) {
	if m := milestoneParen.FindStringSubmatchIndex(title); m != nil {
		return strings.TrimSpace(title[:m[0]]), title[m[2]:m[3]]
	}
	return title, ""
}

type importedGoal struct {
	title, milestone, horizon, status string
	desc                              []string
	idea                              bool
	// bullet: read from a top-level bullet, whose indented lines continue
	// it, rather than from a ### heading. raw is the bullet's text while it
	// is still open: a wrapped line belongs to the title's sentence, so the
	// title is split off only once the whole bullet is read.
	bullet bool
	raw    string
	open   bool
	// whole: the goal is a ## section; its first paragraph is read as the
	// description and the rest of the section is not.
	whole     bool
	wholeDone bool
}

// addWholeLine reads a whole-section goal's description: the first
// paragraph of prose under its heading, not lists, tables or later text.
func (g *importedGoal) addWholeLine(trimmed string) {
	if g.wholeDone {
		return
	}
	switch {
	case trimmed == "":
		if len(g.desc) > 0 {
			g.wholeDone = true
		}
	case bulletLine.MatchString(trimmed) || strings.HasPrefix(trimmed, "|"):
		if len(g.desc) > 0 {
			g.wholeDone = true
		}
	default:
		g.desc = append(g.desc, cleanInline(roadmapInline(trimmed)))
	}
}

// finish splits a bullet's title from its text once the bullet is read.
func (g *importedGoal) finish() {
	if !g.bullet || !g.open {
		return
	}
	g.open = false
	title, rest := splitTitle(g.raw)
	g.title, g.milestone = splitMilestone(title)
	g.desc = append(nonEmpty(rest), g.desc...)
}

// ImportRoadmapMarkdown parses a roadmap file into goals. Goal ids are
// goal-<title>, so importing the same file again changes nothing.
func ImportRoadmapMarkdown(r io.Reader) (RoadmapImport, error) {
	return ImportRoadmapMarkdownWith(r, RoadmapImportOptions{})
}

// ImportRoadmapMarkdownWith parses a roadmap file with section mappings. A
// mapping that matches no heading is an error: a misspelt prefix would
// otherwise import less, silently.
func ImportRoadmapMarkdownWith(r io.Reader, opts RoadmapImportOptions) (RoadmapImport, error) {
	used := map[int]bool{}
	var (
		out     RoadmapImport
		goals   []*importedGoal
		sec     *importSection
		cur     *importedGoal
		inFront bool
		inCode  bool
		lineNo  int
	)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		lineNo++
		trimmed := strings.TrimSpace(line)
		if lineNo == 1 && trimmed == "---" {
			inFront = true
			continue
		}
		if inFront {
			if trimmed == "---" {
				inFront = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			continue
		}
		if inCode || strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, ">") {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "## "):
			heading := strings.TrimSpace(trimmed[3:])
			if cur != nil {
				cur.finish()
			}
			cur = nil
			if s, ok := opts.sectionOf(heading, used); ok && s.whole {
				// The section is the goal; its first paragraph describes it.
				title, milestone := splitMilestone(cleanInline(roadmapInline(heading)))
				cur = &importedGoal{title: title, milestone: milestone, horizon: s.horizon, status: s.status, whole: true}
				goals = append(goals, cur)
				sec = nil
			} else if ok {
				sec = &s
			} else {
				sec = nil
				out.Skipped = append(out.Skipped, heading)
			}
			continue
		case strings.HasPrefix(trimmed, "### "):
			if sec == nil {
				continue
			}
			if cur != nil {
				cur.finish()
			}
			title, milestone := splitMilestone(cleanInline(roadmapInline(trimmed[4:])))
			cur = &importedGoal{title: title, milestone: milestone, horizon: sec.horizon, status: sec.status}
			goals = append(goals, cur)
			continue
		case strings.HasPrefix(trimmed, "#"):
			continue
		}
		if cur != nil && cur.whole {
			cur.addWholeLine(trimmed)
			continue
		}
		if sec == nil {
			continue
		}
		indented := len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
		if m := bulletLine.FindStringSubmatch(trimmed); m != nil && !indented && (cur == nil || cur.bullet) {
			if cur != nil {
				cur.finish()
			}
			cur = &importedGoal{horizon: sec.horizon, status: sec.status, bullet: true, open: true, raw: m[2]}
			goals = append(goals, cur)
			continue
		}
		if cur != nil && cur.open {
			if trimmed != "" && indented && bulletLine.FindStringSubmatch(trimmed) == nil {
				cur.raw += " " + trimmed
				continue
			}
			cur.finish()
		}
		if cur == nil || trimmed == "" && len(cur.desc) == 0 {
			continue
		}
		switch {
		case trimmed == "_Idea — not yet agreed._":
			cur.idea = true
		case strings.HasPrefix(trimmed, "Features: ") && !cur.bullet:
			// Rendered by roady from the links; the links are the truth.
		default:
			cur.desc = append(cur.desc, cleanInline(roadmapInline(trimmed)))
		}
	}
	if err := sc.Err(); err != nil {
		return out, fmt.Errorf("read roadmap: %w", err)
	}
	var unmatched []string
	for i, m := range opts.Sections {
		if !used[i] {
			unmatched = append(unmatched, fmt.Sprintf("%q", m.Prefix))
		}
	}
	if len(unmatched) > 0 {
		return out, fmt.Errorf("no ## section starts with %s", strings.Join(unmatched, ", "))
	}

	if cur != nil {
		cur.finish()
	}
	seen := map[string]int{}
	for _, g := range goals {
		if g.title == "" {
			continue
		}
		id := "goal-" + shortSlug(g.title)
		if id == "goal-" {
			continue
		}
		seen[id]++
		if n := seen[id]; n > 1 {
			id = fmt.Sprintf("%s-%d", id, n)
		}
		title := g.title
		cg := CaptureGoal{ID: id, Title: &title}
		if d := g.description(); d != "" {
			cg.Description = &d
		}
		if g.horizon != "" {
			h := g.horizon
			cg.Horizon = &h
		}
		status := g.status
		if g.idea {
			status = string(spec.GoalIdea)
		}
		if status != "" {
			cg.Status = &status
		}
		if g.milestone != "" {
			m := g.milestone
			cg.Milestone = &m
		}
		out.Doc.Goals = append(out.Doc.Goals, cg)
	}
	return out, nil
}

func (g *importedGoal) description() string {
	var parts []string
	for _, d := range g.desc {
		if d != "" {
			parts = append(parts, d)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func nonEmpty(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []string{s}
}
