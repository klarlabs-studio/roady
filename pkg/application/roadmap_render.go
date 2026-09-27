package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/drift"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// ROADMAP.md is rendered from the goals, not written by hand. The first line
// is a marker carrying the hash of the rest, so an edit made to the file
// instead of to the goals is noticed — and reported as drift — rather than
// silently overwritten by the next render or silently diverging from roady.

// RoadmapFile is the conventional file name, at the repository root.
const RoadmapFile = "ROADMAP.md"

var roadmapMarker = regexp.MustCompile(`^<!-- roady:roadmap sha256=([0-9a-f]{64})\b.*-->$`)

// RenderRoadmapMarkdown renders sp's goals as a ROADMAP.md, marker included.
// It carries no task progress, so the file changes only when the roadmap
// does, not every time a task finishes.
func RenderRoadmapMarkdown(sp *spec.ProductSpec) string {
	body := renderRoadmapBody(sp)
	return roadmapHeader(body) + "\n" + body
}

func roadmapHeader(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "<!-- roady:roadmap sha256=" + hex.EncodeToString(sum[:]) +
		" — generated from .roady/spec.yaml by `roady goal render`. Change goals with `roady goal`, not this file. -->"
}

func renderRoadmapBody(sp *spec.ProductSpec) string {
	var b strings.Builder
	b.WriteString("# Roadmap\n")
	if sp == nil || len(sp.Goals) == 0 {
		b.WriteString("\nNo goals yet.\n")
		return b.String()
	}
	featureTitle := map[string]string{}
	for _, f := range sp.Features {
		featureTitle[f.ID] = f.Title
	}
	rm := BuildRoadmap(sp, nil, nil)
	for _, s := range rm.Sections {
		fmt.Fprintf(&b, "\n## %s\n", s.Name)
		if s.Name == SectionOutOfScope {
			b.WriteString("\n")
			for _, g := range s.Goals {
				line := "- **" + g.Title + "**"
				if d := strings.TrimSpace(g.Description); d != "" {
					line += " — " + strings.ReplaceAll(d, "\n", "\n  ")
				}
				b.WriteString(line + "\n")
			}
			continue
		}
		for _, g := range s.Goals {
			heading := g.Title
			if g.Milestone != "" {
				heading += " (" + g.Milestone + ")"
			}
			fmt.Fprintf(&b, "\n### %s\n", heading)
			if g.Status == spec.GoalIdea && s.Name != SectionIdeas {
				b.WriteString("\n_Idea — not yet agreed._\n")
			}
			if d := strings.TrimSpace(g.Description); d != "" {
				b.WriteString("\n" + d + "\n")
			}
			if len(g.Features) > 0 {
				names := make([]string, 0, len(g.Features))
				for _, id := range g.Features {
					if t := featureTitle[id]; t != "" {
						names = append(names, t+" (`"+id+"`)")
					} else {
						names = append(names, "`"+id+"`")
					}
				}
				b.WriteString("\nFeatures: " + strings.Join(names, ", ") + "\n")
			}
		}
	}
	return b.String()
}

// RoadmapState is how a rendered roadmap file relates to the goals.
type RoadmapState string

const (
	RoadmapInSync     RoadmapState = "in_sync"
	RoadmapHandEdited RoadmapState = "hand_edited"
	RoadmapOutOfDate  RoadmapState = "out_of_date"
	// RoadmapNotRendered is a file roady did not write (no marker).
	RoadmapNotRendered RoadmapState = "not_rendered"
)

// CompareRoadmap reports how content, a rendered roadmap file, relates to
// the roadmap sp would render now.
func CompareRoadmap(content string, sp *spec.ProductSpec) RoadmapState {
	// A checkout with CRLF line endings is the same file, not an edit.
	content = strings.ReplaceAll(content, "\r\n", "\n")
	first, body, _ := strings.Cut(content, "\n")
	m := roadmapMarker.FindStringSubmatch(first)
	if m == nil {
		return RoadmapNotRendered
	}
	sum := sha256.Sum256([]byte(body))
	if hex.EncodeToString(sum[:]) != m[1] {
		return RoadmapHandEdited
	}
	if body != renderRoadmapBody(sp) {
		return RoadmapOutOfDate
	}
	return RoadmapInSync
}

// RoadmapDrift reads the rendered roadmap at path and reports it when it was
// edited by hand or no longer matches the goals. A missing file, or one roady
// did not render, is not drift: projects without a rendered roadmap are fine.
func RoadmapDrift(path string, sp *spec.ProductSpec) []drift.Issue {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	switch CompareRoadmap(string(raw), sp) {
	case RoadmapHandEdited:
		return []drift.Issue{{
			ID: "roadmap-hand-edited", Type: drift.DriftTypeDoc, Category: drift.CategoryMismatch, Severity: drift.SeverityMedium,
			ComponentID: filepath.Base(path), Path: path,
			Message: filepath.Base(path) + " was edited by hand; it is rendered from the goals, so the next render would drop the edit.",
			Hint:    "Make the change with `roady goal add|edit` (or a capture), then `roady goal render`.",
		}}
	case RoadmapOutOfDate:
		return []drift.Issue{{
			ID: "roadmap-out-of-date", Type: drift.DriftTypeDoc, Category: drift.CategoryStale, Severity: drift.SeverityLow,
			ComponentID: filepath.Base(path), Path: path,
			Message: filepath.Base(path) + " no longer matches the goals.",
			Hint:    "Run `roady goal render`.",
		}}
	}
	return nil
}

// RoadmapWrite reports what WriteRoadmap found and did.
type RoadmapWrite struct {
	Path    string       `json:"path"`
	Before  RoadmapState `json:"before"` // not_rendered also covers a missing file
	Missing bool         `json:"missing,omitempty"`
	Written bool         `json:"written"`
}

// ErrRoadmapHandEdited is returned when rendering would replace a file that
// was edited by hand or not written by roady, and force was not given.
var ErrRoadmapHandEdited = errors.New("the roadmap file has content the goals do not")

// WriteRoadmap renders sp to path unless the file is already in sync. A file
// edited by hand, or one roady did not write, is replaced only with force:
// its content would be lost, so it should be moved into goals first.
func WriteRoadmap(path string, sp *spec.ProductSpec, force bool) (RoadmapWrite, error) {
	res := RoadmapWrite{Path: path, Before: RoadmapNotRendered}
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		res.Before = CompareRoadmap(string(existing), sp)
	case os.IsNotExist(err):
		res.Missing = true
	default:
		return res, err
	}
	if res.Before == RoadmapInSync {
		return res, nil
	}
	if !res.Missing && (res.Before == RoadmapHandEdited || res.Before == RoadmapNotRendered) && !force {
		how := "was edited by hand"
		if res.Before == RoadmapNotRendered {
			how = "was not written by roady"
		}
		return res, fmt.Errorf("%s %s: %w; move what it says into goals (roady goal add|edit), then render with force",
			path, how, ErrRoadmapHandEdited)
	}
	if err := os.WriteFile(path, []byte(RenderRoadmapMarkdown(sp)), 0o644); err != nil {
		return res, err
	}
	res.Written = true
	return res, nil
}

// RoadmapPath is where the project's roadmap is rendered: the policy's
// roadmap setting, relative to root, or ROADMAP.md at root. ok is false for
// a sub-project with no setting — it has no default file.
func RoadmapPath(root string, pol *domain.PolicyConfig, subProject bool) (path string, ok bool) {
	if pol != nil && strings.TrimSpace(pol.Roadmap) != "" {
		p := strings.TrimSpace(pol.Roadmap)
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		return p, true
	}
	if subProject {
		return "", false
	}
	return filepath.Join(root, RoadmapFile), true
}
