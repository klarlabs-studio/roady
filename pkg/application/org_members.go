package application

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/felixgeelhaar/roady/pkg/domain/org"
)

// ResolveMembers returns the repositories belonging to this workspace.
//
// org.yaml has carried a repos: list since the type was introduced and
// nothing ever read it. A workspace could therefore declare its members and
// Roady would quietly walk the tree instead — missing any repository outside
// the root, silently including any scratch checkout inside it, and reporting
// aggregate progress that answered a different question from the one asked.
//
// A declared list is now authoritative. Discovery remains the behaviour when
// nothing is declared, because declaring members is an option rather than a
// requirement, and a workspace that has never needed the distinction should
// not have to start.
func (s *OrgService) ResolveMembers() (*org.MemberSet, error) {
	config, err := s.LoadOrgConfig()
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load org config: %w", err)
	}

	if config == nil || len(config.Repos) == 0 {
		return s.discoverMembers()
	}

	set := &org.MemberSet{Declared: true, Members: make([]org.Member, 0, len(config.Repos))}
	for _, declared := range config.Repos {
		set.Members = append(set.Members, s.resolveMember(declared))
	}
	return set, nil
}

// resolveMember turns one declared path into a member, describing why it
// cannot be used rather than dropping it.
func (s *OrgService) resolveMember(declared string) org.Member {
	member := org.Member{Declared: declared}

	path := declared
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.root, path)
	}
	path = filepath.Clean(path)

	info, err := os.Stat(path)
	switch {
	case os.IsNotExist(err):
		member.Problem = fmt.Sprintf("no such directory: %s", path)
		return member
	case err != nil:
		member.Problem = fmt.Sprintf("cannot read %s: %v", path, err)
		return member
	case !info.IsDir():
		member.Problem = fmt.Sprintf("%s is not a directory", path)
		return member
	}

	roadyDir := filepath.Join(path, ".roady")
	if info, err := os.Stat(roadyDir); err != nil || !info.IsDir() {
		member.Path = path
		member.Problem = fmt.Sprintf("no Roady project at %s; run 'roady init' there or remove it from repos", path)
		return member
	}

	member.Path = path
	member.RoadyDir = roadyDir
	return member
}

// discoverMembers falls back to walking the tree.
func (s *OrgService) discoverMembers() (*org.MemberSet, error) {
	paths, err := s.DiscoverProjects()
	if err != nil {
		return nil, err
	}

	set := &org.MemberSet{Members: make([]org.Member, 0, len(paths))}
	for _, path := range paths {
		// Show the operator the path relative to the workspace where that
		// is meaningful, since an absolute temp path tells them nothing.
		declared := path
		if rel, err := filepath.Rel(s.root, path); err == nil {
			declared = rel
		}
		set.Members = append(set.Members, org.Member{
			Declared: declared,
			Path:     path,
			RoadyDir: filepath.Join(path, ".roady"),
		})
	}
	return set, nil
}
