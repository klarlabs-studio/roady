package application

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain/prompt"
)

// A project that planned in markdown before (or instead of) roady keeps its
// intent in notes: a decisions log, open threads with what each waits on, a
// status page, session logs. Only a model can read prose like that. Roady
// runs no inference, so it hands the caller's model the notes, what the
// project already holds, and the capture document to answer with.

// NoteFile is one planning note to convert.
type NoteFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	// Truncated reports that Content is the start of a longer file.
	Truncated bool `json:"truncated,omitempty"`
}

// Limits on what goes into one prompt: enough for a decisions log and an
// open-threads page, not a year of session logs.
const (
	noteFileLimit  = 40 << 10
	noteTotalLimit = 120 << 10
)

// ReadNotes reads the named files, relative to root unless absolute, capped
// per file and in total. A directory contributes its .md files.
func ReadNotes(root string, paths []string) ([]NoteFile, error) {
	var files []string
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("read notes: %w", err)
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, fmt.Errorf("read notes: %w", err)
		}
		var mds []string
		for _, e := range entries {
			if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
				mds = append(mds, filepath.Join(p, e.Name()))
			}
		}
		sort.Strings(mds)
		files = append(files, mds...)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no notes to read: name markdown files or a directory of them")
	}
	var (
		notes []NoteFile
		total int
	)
	for _, f := range files {
		raw, err := os.ReadFile(f) // #nosec G304 -- files the user named
		if err != nil {
			return nil, fmt.Errorf("read notes: %w", err)
		}
		n := NoteFile{Path: displayPath(root, f), Content: string(raw)}
		limit := noteFileLimit
		if left := noteTotalLimit - total; left < limit {
			limit = left
		}
		if limit <= 0 {
			n.Content, n.Truncated = "", true
		} else if len(n.Content) > limit {
			n.Content, n.Truncated = n.Content[:limit], true
		}
		total += len(n.Content)
		notes = append(notes, n)
	}
	return notes, nil
}

func displayPath(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

// NotesToCapture builds the request that turns planning notes into one
// capture document.
func (s *PromptService) NotesToCapture(_ context.Context, notes []NoteFile) (*prompt.Request, error) {
	if err := s.checkPolicy(); err != nil {
		return nil, err
	}
	if len(notes) == 0 {
		return nil, fmt.Errorf("no notes given")
	}
	sp, _ := s.repo.LoadSpec()

	var b strings.Builder
	b.WriteString("Convert these planning notes into one roady capture document.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Only what the notes say. Do not invent goals, dates or reasons.\n")
	b.WriteString("- Roadmap items become goals: horizon now, next or later; status shipped for what is done, out_of_scope for what was ruled out, idea for what is only floated.\n")
	b.WriteString("- Each dated decision becomes a decision: title (what it was about), choice (what was chosen), context (why), date (YYYY-MM-DD). One that a later entry replaces: record both, the later one with supersedes.\n")
	b.WriteString("- Open work becomes a task with a title that says what the work is — never a status word like \"done\". Link it to a goal with goal when the notes place it on the roadmap.\n")
	b.WriteString("- Work that is finished is history: a shipped goal, not a task.\n")
	b.WriteString("- Session logs and running commentary are evidence, not items; mine them only for decisions and open work.\n")
	b.WriteString("- Ids: goal-<short-slug>, dec-<short-slug>, task-<short-slug>, a few words each. Reuse an id below when the note is about the same thing, so it updates rather than duplicates.\n\n")

	if sp != nil && (len(sp.Goals) > 0 || len(sp.Decisions) > 0 || len(sp.Features) > 0) {
		b.WriteString("Already recorded:\n")
		for _, g := range sp.Goals {
			fmt.Fprintf(&b, "- goal %s: %s (%s)\n", g.ID, g.Title, strings.TrimSpace(string(g.Horizon)+" "+string(g.EffectiveStatus())))
		}
		for _, d := range sp.Decisions {
			fmt.Fprintf(&b, "- decision %s: %s\n", d.ID, d.Title)
		}
		for i, f := range sp.Features {
			if i == 40 {
				fmt.Fprintf(&b, "- … %d more features\n", len(sp.Features)-40)
				break
			}
			fmt.Fprintf(&b, "- feature %s: %s\n", f.ID, f.Title)
		}
		b.WriteString("\n")
	}

	for _, n := range notes {
		fmt.Fprintf(&b, "=== %s", n.Path)
		if n.Truncated {
			b.WriteString(" (truncated)")
		}
		b.WriteString(" ===\n")
		b.WriteString(strings.TrimRight(n.Content, "\n"))
		b.WriteString("\n\n")
	}

	return &prompt.Request{
		Operation: prompt.OpNotesToCapture,
		System:    "You move a project's planning notes into a structured plan, faithfully and without embellishment.",
		Prompt:    b.String(),
		ExpectedFormat: `{"goals": [{"id": "goal-...", "title": "...", "description": "...", "horizon": "now|next|later", ` +
			`"status": "idea|planned|shipped|out_of_scope", "milestone": "..."}], ` +
			`"decisions": [{"id": "dec-...", "title": "...", "choice": "...", "context": "...", "date": "YYYY-MM-DD", "supersedes": "dec-..."}], ` +
			`"tasks": [{"id": "task-...", "title": "...", "description": "...", "priority": "low|medium|high", "goal": "goal-..."}]}`,
		WriteBack: "roady_capture",
		Guidance: "Write the document yourself, show it to the user, then record it with roady_capture (or `roady capture -f`); try dry_run first. " +
			"For a task the notes say is waiting on something outside the project, block it afterwards with `roady task block <id>` and say what it waits on.",
	}, nil
}
