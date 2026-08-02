package application

import (
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/project"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// SubProjectResolver answers cross-project dependency lookups by reading the
// named sub-project's execution state under .roady/projects/<name>/.
//
// It exists so the domain can ask "is @auth:task-signup done?" without
// knowing anything about directories.
type SubProjectResolver struct {
	root string
	// cache avoids re-reading a sibling project's state once per dependency
	// on a plan that references it many times.
	cache map[string]*planning.ExecutionState
}

func NewSubProjectResolver(root string) *SubProjectResolver {
	return &SubProjectResolver{root: root, cache: map[string]*planning.ExecutionState{}}
}

var _ project.ExternalStatusResolver = (*SubProjectResolver)(nil)

// ExternalTaskStatus reports a task's status in another sub-project.
//
// The second return distinguishes "not done" from "cannot be found", which
// callers need: an unresolvable reference is a broken plan, while an
// incomplete one is ordinary work in progress. Both block, for different
// reasons.
func (r *SubProjectResolver) ExternalTaskStatus(projectName, taskID string) (planning.TaskStatus, bool) {
	state, ok := r.cache[projectName]
	if !ok {
		repo, err := storage.NewFilesystemRepositoryForProject(r.root, projectName)
		if err != nil {
			r.cache[projectName] = nil
			return "", false
		}
		loaded, err := repo.LoadState()
		if err != nil {
			loaded = nil
		}
		r.cache[projectName] = loaded
		state = loaded
	}
	if state == nil {
		return "", false
	}

	result, found := state.GetTaskResult(taskID)
	if !found {
		return "", false
	}
	return result.Status, true
}
