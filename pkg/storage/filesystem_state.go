package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// SaveState writes the execution state if nobody else wrote it since it was
// loaded. The version check and the write happen under a lock, so of two
// processes saving from the same version exactly one succeeds and the other
// gets a ConflictError to reload and retry. In a git repository the state is
// the shared file every worktree reads; the checkout's state.json is
// rewritten after it as a mirror for commits.
func (r *FilesystemRepository) SaveState(s *planning.ExecutionState) error {
	local, err := r.ResolvePath(StateFile)
	if err != nil {
		return err
	}
	path, shared := r.StateLocation()
	if shared {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create shared state dir: %w", err)
		}
	}
	var data []byte
	err = withFileLock(path, func() error {
		// A shared file that does not exist yet was seeded from the checkout
		// (LoadState), so the checkout's version is the one to match.
		current := path
		if _, statErr := os.Stat(path); shared && os.IsNotExist(statErr) {
			current = local
		}
		// #nosec G304 -- Paths are resolved by the repository
		if existing, err := os.ReadFile(current); err == nil {
			var disk planning.ExecutionState
			if jsonErr := json.Unmarshal(existing, &disk); jsonErr == nil && disk.Version != s.Version {
				return &planning.ConflictError{Expected: s.Version, Actual: disk.Version}
			}
		}
		s.Version++
		var mErr error
		data, mErr = json.MarshalIndent(s, "", "  ")
		if mErr != nil {
			s.Version--
			return fmt.Errorf("failed to marshal state: %w", mErr)
		}
		if err := writeFileAtomic(path, data, 0o600); err != nil {
			s.Version--
			return err
		}
		return nil
	})
	if err != nil || !shared {
		return err
	}
	// The mirror is for review; the shared file already holds the truth, so
	// a failure here must not fail the save.
	_ = writeFileAtomic(local, data, 0o600)
	return nil
}

// LoadState reads the execution state: the shared file when there is one,
// otherwise the checkout's state.json (which also seeds the shared file on
// the first save).
func (r *FilesystemRepository) LoadState() (*planning.ExecutionState, error) {
	local, err := r.ResolvePath(StateFile)
	if err != nil {
		return nil, err
	}
	path, shared := r.StateLocation()
	// #nosec G304 -- Paths are resolved by the repository
	data, err := os.ReadFile(path)
	if shared && os.IsNotExist(err) {
		data, err = os.ReadFile(local) // #nosec G304
	}
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty state if not found
			return planning.NewExecutionState("unknown"), nil
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var s planning.ExecutionState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &s, nil
}
