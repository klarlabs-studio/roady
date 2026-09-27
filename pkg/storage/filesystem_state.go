package storage

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// SaveState writes the execution state if nobody else wrote it since it was
// loaded. The version check and the write happen under a lock, so of two
// processes saving from the same version exactly one succeeds and the other
// gets a ConflictError to reload and retry.
func (r *FilesystemRepository) SaveState(s *planning.ExecutionState) error {
	path, err := r.ResolvePath(StateFile)
	if err != nil {
		return err
	}
	return withFileLock(path, func() error {
		// #nosec G304 -- Path is resolved and validated via ResolvePath
		if existing, err := os.ReadFile(path); err == nil {
			var disk planning.ExecutionState
			if jsonErr := json.Unmarshal(existing, &disk); jsonErr == nil && disk.Version != s.Version {
				return &planning.ConflictError{Expected: s.Version, Actual: disk.Version}
			}
		}
		// If the file doesn't exist, no conflict is possible.
		s.Version++
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			s.Version--
			return fmt.Errorf("failed to marshal state: %w", err)
		}
		if err := writeFileAtomic(path, data, 0o600); err != nil {
			s.Version--
			return err
		}
		return nil
	})
}

func (r *FilesystemRepository) LoadState() (*planning.ExecutionState, error) {
	path, err := r.ResolvePath(StateFile)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- Path is resolved and validated via ResolvePath
	data, err := os.ReadFile(path)
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
