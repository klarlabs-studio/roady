package org

import (
	"fmt"
	"path/filepath"
)

// Member is one repository belonging to a workspace.
//
// A member is either declared in org.yaml or found by walking the tree. The
// two are not equivalent: a declared member that cannot be resolved is a
// problem to report, whereas a directory the walk did not reach was never
// claimed to exist in the first place.
type Member struct {
	// Declared is the path exactly as written in org.yaml, or the path the
	// walk found it at. It is what an operator recognises.
	Declared string `json:"declared" yaml:"declared"`

	// Path is the resolved absolute directory of the repository.
	Path string `json:"path,omitempty" yaml:"path,omitempty"`

	// RoadyDir is the .roady directory inside Path.
	RoadyDir string `json:"roady_dir,omitempty" yaml:"roady_dir,omitempty"`

	// Problem says why this member is unusable, and is empty when it is
	// fine. A member is reported rather than dropped, because a workspace
	// that silently omits a repository reports healthy progress across
	// repositories nobody is looking at.
	Problem string `json:"problem,omitempty" yaml:"problem,omitempty"`
}

// OK reports whether the member resolved to a usable Roady project.
func (m Member) OK() bool {
	return m.Problem == "" && m.Path != "" && m.RoadyDir != ""
}

// Name is the short identifier for the member in a report.
func (m Member) Name() string {
	if m.Path != "" {
		return filepath.Base(m.Path)
	}
	return filepath.Base(m.Declared)
}

// MemberSet is the membership of one workspace.
type MemberSet struct {
	// Declared is true when the membership came from org.yaml rather than
	// from walking the filesystem. A declared set is authoritative: it can
	// name repositories outside the walk root, and a member missing from it
	// is deliberate.
	Declared bool `json:"declared" yaml:"declared"`

	Members []Member `json:"members" yaml:"members"`
}

// IsDeclared reports whether membership was stated rather than discovered.
func (s MemberSet) IsDeclared() bool { return s.Declared }

// Usable returns the members that resolved to a Roady project.
func (s MemberSet) Usable() []Member {
	out := make([]Member, 0, len(s.Members))
	for _, m := range s.Members {
		if m.OK() {
			out = append(out, m)
		}
	}
	return out
}

// Problems describes each member that could not be used.
func (s MemberSet) Problems() []string {
	var out []string
	for _, m := range s.Members {
		if m.OK() {
			continue
		}
		reason := m.Problem
		if reason == "" {
			reason = "could not be resolved"
		}
		out = append(out, fmt.Sprintf("member %q: %s", m.Declared, reason))
	}
	return out
}

// HasProblems reports whether any declared member is unusable.
func (s MemberSet) HasProblems() bool { return len(s.Problems()) > 0 }

// Names lists the short name of every usable member.
func (s MemberSet) Names() []string {
	usable := s.Usable()
	out := make([]string, 0, len(usable))
	for _, m := range usable {
		out = append(out, m.Name())
	}
	return out
}
