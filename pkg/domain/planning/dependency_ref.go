package planning

import "strings"

// DependencyRef is one entry in a task's DependsOn list.
//
// Most dependencies point at another task in the same plan. An org-level plan
// also needs to say "this waits on work in another sub-project", which is what
// Project carries: `@feature-auth:task-signup`.
type DependencyRef struct {
	// Project names the sub-project under .roady/projects/<name>/, or is
	// empty for a dependency inside this plan.
	Project string
	// TaskID is the task depended upon.
	TaskID string
}

// IsExternal reports whether the dependency lives in another sub-project.
func (r DependencyRef) IsExternal() bool { return r.Project != "" }

// String renders the canonical form. ParseDependencyRef round-trips it, so a
// plan rewritten by the reconciler keeps the reference the author wrote.
func (r DependencyRef) String() string {
	if r.Project == "" {
		return r.TaskID
	}
	return "@" + r.Project + ":" + r.TaskID
}

// ParseDependencyRef reads one DependsOn entry.
//
// The canonical form is `@project:task`. The sigil-less `project:task` is also
// accepted because it predates the sigil and is already stored in real plans —
// rejecting it would silently reinterpret a cross-project dependency as a
// dangling local one, which reads as a broken plan rather than a format
// change.
//
// Anything that does not yield both a project and a task is treated as a local
// task ID verbatim, so an unfamiliar shape degrades to "task not found" rather
// than being silently split apart.
func ParseDependencyRef(raw string) DependencyRef {
	trimmed := strings.TrimSpace(raw)

	body := strings.TrimPrefix(trimmed, "@")
	project, task, found := strings.Cut(body, ":")
	if !found || project == "" || task == "" {
		return DependencyRef{TaskID: trimmed}
	}

	return DependencyRef{Project: project, TaskID: task}
}

// ExternalDependencies returns the task's cross-project dependencies.
func (t Task) ExternalDependencies() []DependencyRef {
	var refs []DependencyRef
	for _, dep := range t.DependsOn {
		if ref := ParseDependencyRef(dep); ref.IsExternal() {
			refs = append(refs, ref)
		}
	}
	return refs
}

// LocalDependencies returns the task's dependencies within this plan.
func (t Task) LocalDependencies() []string {
	var ids []string
	for _, dep := range t.DependsOn {
		if ref := ParseDependencyRef(dep); !ref.IsExternal() {
			ids = append(ids, ref.TaskID)
		}
	}
	return ids
}
