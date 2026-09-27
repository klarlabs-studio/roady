package application

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// Markdown import turns an ordinary PRD into features and requirements.
//
// The previous importer took every "## " heading as a feature and everything
// under it as description, and never produced a requirement. A normal PRD —
// "## Overview", then "## Features" with "### " sections of bullet points —
// became two features called Overview and Features, and plan generation
// turned those into "Implement Overview" and "Implement Features". The first
// thing roady did with a real document was lose all of its structure.
//
// The rules, all deterministic:
//
//   - Headings form a tree. A section is a feature when it has no sub-sections
//     of its own, or when it has bullets of its own. Otherwise it is a
//     container ("## Features") and only its children count.
//   - A feature's top-level list items are its requirements. Nested items and
//     continuation lines belong to the item above them, as its description.
//   - Sub-sections named like "Requirements", "Acceptance criteria" or "User
//     stories" give their bullets to the enclosing feature.
//   - Overview-like sections (Overview, Goals, Non-goals, Background, …) are
//     not features; their first paragraph can become the spec description.
//   - "Constraints" and "Non-functional requirements" become spec constraints.
//   - A requirement's priority follows its wording: must/shall/required →
//     high, should → medium, could/may/optional → low.
//   - IDs are slugs, unique across the whole spec. Every feature, requirement
//     and constraint keeps its doc:line.

var (
	mdHeading  = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	mdListItem = regexp.MustCompile(`^(\s*)(?:[-*+]|\d+[.)])\s+(.*)$`)
	mdFence    = regexp.MustCompile("^\\s*(```|~~~)")
	mdCheckbox = regexp.MustCompile(`^\[[ xX]\]\s*`)
	mdLink     = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	// Emphasis is stripped only where it delimits: underscores inside a word
	// (ROADY_MCP_TOOLS, snake_case) are part of the word.
	mdCode     = regexp.MustCompile("`([^`]*)`")
	mdStrong   = regexp.MustCompile(`\*\*(.+?)\*\*|(^|\W)__(.+?)__(\W|$)`)
	mdEmStar   = regexp.MustCompile(`\*([^*\s][^*]*?)\*`)
	mdEmUnder  = regexp.MustCompile(`(^|\W)_([^_\s][^_]*?)_(\W|$)`)
	mdLeadName = regexp.MustCompile(`^\*\*([^*]{1,60})\*\*\s*[:—–-]?\s*(.*)$`)
	mdColon    = regexp.MustCompile(`^([^:.]{2,60}):\s+(.+)$`)

	priorityHigh   = regexp.MustCompile(`(?i)\b(must|shall|required|critical|mandatory)\b`)
	priorityMedium = regexp.MustCompile(`(?i)\bshould\b`)
	// Lower-case "may" in prose grants permission ("users may trigger it"),
	// so only the RFC 2119 capital MAY reads as optional.
	priorityLow = regexp.MustCompile(`(?i:\b(could|optional|nice to have|nice-to-have)\b)|\bMAY\b`)
	mdNumbering = regexp.MustCompile(`^\d+(\.\d+)*\.?\s+`)
)

// Section titles that describe the document rather than something to build.
var mdNarrativeSections = map[string]bool{
	"overview": true, "introduction": true, "intro": true, "background": true, "context": true,
	"summary": true, "executive summary": true, "goals": true, "non-goals": true, "non goals": true,
	"nongoals": true, "out of scope": true, "motivation": true, "problem": true, "problem statement": true,
	"appendix": true, "glossary": true, "references": true, "open questions": true, "questions": true,
	"table of contents": true, "toc": true, "changelog": true, "revision history": true, "faq": true,
	"notes": true, "assumptions": true, "risks": true, "success metrics": true, "metrics": true,
	"vision": true, "purpose": true, "about": true,
}

// Sub-sections whose bullets are the enclosing feature's requirements.
var mdRequirementSections = map[string]bool{
	"requirements": true, "functional requirements": true, "acceptance criteria": true,
	"user stories": true, "stories": true, "tasks": true, "scope": true, "in scope": true,
	"deliverables": true, "capabilities": true, "criteria": true,
}

var mdConstraintSections = map[string]bool{
	"constraints": true, "non-functional requirements": true, "nonfunctional requirements": true,
	"non functional requirements": true, "nfr": true, "nfrs": true, "technical constraints": true,
}

type mdItem struct {
	text string // first line
	more []string
	line int
}

type mdSection struct {
	level    int
	title    string
	line     int
	paras    []string
	items    []mdItem
	children []*mdSection
	parent   *mdSection
}

func normalisedTitle(t string) string {
	t = strings.ToLower(strings.TrimSpace(cleanInline(t)))
	t = strings.TrimRight(t, ":")
	return strings.TrimSpace(mdNumbering.ReplaceAllString(t, ""))
}

// headingTitle drops section numbering ("3.1 Event scheduling"), which
// belongs to the document's layout, not the feature's name or id.
func headingTitle(t string) string {
	return strings.TrimSpace(mdNumbering.ReplaceAllString(cleanInline(t), ""))
}

func cleanInline(s string) string {
	s = mdLink.ReplaceAllString(s, "$1")
	s = mdStrong.ReplaceAllString(s, "$1$2$3$4")
	s = mdEmStar.ReplaceAllString(s, "$1")
	s = mdEmUnder.ReplaceAllString(s, "$1$2$3")
	s = mdCode.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// parseMarkdownFile reads one markdown document into a spec.
func (s *SpecService) parseMarkdownFile(path string) (*spec.ProductSpec, error) {
	cleanPath := filepath.Clean(path)
	file, err := os.Open(cleanPath) // #nosec G304 -- a document the caller named
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close() //nolint:errcheck // best-effort close on read path

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}
	return parseMarkdownSpec(cleanPath, lines), nil
}

func parseMarkdownSpec(doc string, lines []string) *spec.ProductSpec {
	out := &spec.ProductSpec{
		ID:          "imported-spec",
		Version:     "0.1.0",
		Constraints: []spec.Constraint{},
		Features:    []spec.Feature{},
	}

	root := &mdSection{level: 0}
	cur := root
	var preamble []string // text before any heading
	sawH1 := false
	inFence := false
	var para []string
	var openItem *mdItem
	itemIndent := -1

	flushPara := func() {
		if len(para) > 0 {
			text := strings.TrimSpace(strings.Join(para, " "))
			if text != "" {
				if cur == root {
					preamble = append(preamble, text)
				} else {
					cur.paras = append(cur.paras, text)
				}
			}
			para = nil
		}
	}
	closeItem := func() {
		openItem = nil
		itemIndent = -1
	}

	for i, raw := range lines {
		lineNo := i + 1
		if mdFence.MatchString(raw) {
			inFence = !inFence
			flushPara()
			closeItem()
			continue
		}
		if inFence {
			continue
		}
		trimmed := strings.TrimSpace(raw)

		if m := mdHeading.FindStringSubmatch(raw); m != nil && !strings.HasPrefix(raw, " ") {
			flushPara()
			closeItem()
			level := len(m[1])
			title := strings.TrimSpace(m[2])
			if level == 1 && !sawH1 {
				sawH1 = true
				out.Title = cleanInline(title)
				cur = root
				continue
			}
			sec := &mdSection{level: level, title: title, line: lineNo}
			parent := cur
			for parent != root && parent.level >= level {
				parent = parent.parent
			}
			sec.parent = parent
			parent.children = append(parent.children, sec)
			cur = sec
			continue
		}

		if trimmed == "" {
			flushPara()
			continue
		}
		if strings.HasPrefix(trimmed, "|") || strings.HasPrefix(trimmed, ">") && openItem == nil {
			// Tables and block quotes are context, not requirements.
			flushPara()
			continue
		}

		if m := mdListItem.FindStringSubmatch(raw); m != nil && cur != root {
			flushPara()
			indent := len(strings.ReplaceAll(m[1], "\t", "    "))
			if openItem != nil && indent > itemIndent {
				openItem.more = append(openItem.more, cleanInline(mdCheckbox.ReplaceAllString(m[2], "")))
				continue
			}
			cur.items = append(cur.items, mdItem{text: mdCheckbox.ReplaceAllString(m[2], ""), line: lineNo})
			openItem = &cur.items[len(cur.items)-1]
			itemIndent = indent
			continue
		}

		if openItem != nil && (strings.HasPrefix(raw, " ") || strings.HasPrefix(raw, "\t")) {
			openItem.more = append(openItem.more, cleanInline(trimmed))
			continue
		}
		closeItem()
		para = append(para, trimmed)
	}
	flushPara()

	if len(preamble) > 0 {
		out.Description = preamble[0]
	}

	ids := map[string]bool{}
	uniqueID := func(base string) string {
		if base == "" {
			base = "item"
		}
		id := base
		for n := 2; ids[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		ids[id] = true
		return id
	}

	var walk func(sec *mdSection)
	walk = func(sec *mdSection) {
		name := normalisedTitle(sec.title)
		switch {
		case mdNarrativeSections[name]:
			if out.Description == "" && len(sec.paras) > 0 {
				out.Description = sec.paras[0]
			}
			return
		case mdConstraintSections[name]:
			for _, it := range sec.items {
				title, desc := splitItem(it)
				text := title
				if desc != "" && desc != title {
					text = desc
				}
				out.Constraints = append(out.Constraints, spec.Constraint{ID: uniqueID(shortSlug(title)), Description: text})
			}
			return
		}

		var own []*mdSection // children that are sections in their own right
		var absorbed []*mdSection
		for _, c := range sec.children {
			if mdRequirementSections[normalisedTitle(c.title)] && len(c.children) == 0 {
				absorbed = append(absorbed, c)
			} else {
				own = append(own, c)
			}
		}

		items := append([]mdItem{}, sec.items...)
		paras := append([]string{}, sec.paras...)
		for _, a := range absorbed {
			items = append(items, a.items...)
			paras = append(paras, a.paras...)
		}

		isFeature := len(own) == 0 || len(sec.items) > 0
		if isFeature && sec != nil && (len(items) > 0 || len(paras) > 0 || len(own) == 0) {
			// A leaf "Features" section listing bullets lists features, not
			// requirements of a feature called Features.
			if name == "features" && len(own) == 0 {
				for _, it := range items {
					title, desc := splitItem(it)
					out.Features = append(out.Features, spec.Feature{
						ID: uniqueID(shortSlug(title)), Title: title, Description: desc,
						Requirements: []spec.Requirement{},
						Source:       spec.Source{Doc: doc, Line: it.line},
					})
				}
			} else if len(items) > 0 || len(paras) > 0 {
				f := spec.Feature{
					ID:           uniqueID(shortSlug(headingTitle(sec.title))),
					Title:        headingTitle(sec.title),
					Description:  strings.Join(paras, "\n\n"),
					Requirements: []spec.Requirement{},
					Source:       spec.Source{Doc: doc, Line: sec.line},
				}
				for _, it := range items {
					title, desc := splitItem(it)
					f.Requirements = append(f.Requirements, spec.Requirement{
						ID:          uniqueID(shortSlug(title)),
						Title:       title,
						Description: desc,
						Priority:    priorityOf(it),
						DependsOn:   []string{},
						Source:      spec.Source{Doc: doc, Line: it.line},
					})
				}
				out.Features = append(out.Features, f)
			}
		}
		for _, c := range own {
			walk(c)
		}
	}
	for _, c := range root.children {
		walk(c)
	}
	return out
}

// splitItem derives a requirement's title and description from a list item.
// "**Export**: CSV for the tax advisor" and "Export: CSV …" name themselves;
// otherwise a short item is its own title and a long one is titled by its
// first sentence.
func splitItem(it mdItem) (title, desc string) {
	raw := strings.TrimSpace(it.text)
	full := cleanInline(raw)
	if len(it.more) > 0 {
		full = full + "\n" + strings.Join(it.more, "\n")
	}
	if m := mdLeadName.FindStringSubmatch(raw); m != nil {
		return strings.TrimRight(cleanInline(m[1]), ". "), full
	}
	clean := cleanInline(raw)
	if m := mdColon.FindStringSubmatch(clean); m != nil && !strings.Contains(m[1], " must") {
		return strings.TrimRight(strings.TrimSpace(m[1]), ". "), full
	}
	title = clean
	if len(title) > 100 {
		if i := strings.Index(title, ". "); i > 0 && i < 100 {
			title = title[:i]
		} else {
			cut := strings.LastIndex(title[:97], " ")
			if cut < 40 {
				cut = 97
			}
			title = title[:cut] + "…"
		}
	}
	title = strings.TrimRight(title, ".")
	if full == title || full == title+"." {
		full = ""
	}
	return title, full
}

func priorityOf(it mdItem) string {
	text := it.text
	switch {
	case priorityHigh.MatchString(text):
		return "high"
	case priorityMedium.MatchString(text):
		return "medium"
	case priorityLow.MatchString(text):
		return "low"
	}
	return ""
}

// shortSlug is spec.Slugify bounded to a readable length on a word boundary.
func shortSlug(title string) string {
	s := spec.Slugify(cleanInline(title))
	if len(s) <= 48 {
		return s
	}
	cut := strings.LastIndex(s[:48], "-")
	if cut < 20 {
		cut = 48
	}
	return strings.Trim(s[:cut], "-")
}
