package application

import (
	"fmt"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

// LockState reports whether the files derived from the spec still agree with
// it.
//
// `roady init --template x` writes spec.yaml, spec.lock.json and state.json
// together, so they agree by construction. The realistic adoption path then
// replaces spec.yaml with one describing the actual project — and the two
// derived files keep the template's identity. Nothing reconciled them and
// nothing reported it, while `spec validate` answered "valid", which reads as
// "everything here is fine" at exactly the moment it is not.
type LockState struct {
	// SpecID is the id the specification declares.
	SpecID string
	// LockID is the id the drift baseline was captured under.
	LockID string
	// StateID is the project id execution state was created under.
	StateID string

	// LockPresent is false when no baseline has been captured at all.
	LockPresent bool
	// StatePresent is false when execution state does not exist yet.
	StatePresent bool
}

// InSync reports whether every derived file agrees with the spec.
func (s LockState) InSync() bool { return len(s.Problems()) == 0 }

// Problems describes each disagreement and how to resolve it.
func (s LockState) Problems() []string {
	var out []string
	if s.LockPresent && s.LockID != s.SpecID {
		out = append(out, fmt.Sprintf(
			"spec.lock.json was captured for %q but spec.yaml declares %q. The lock is the drift baseline, so drift is being measured against a spec this project never had. Run 'roady spec lock' to re-capture it.",
			s.LockID, s.SpecID))
	}
	if s.StatePresent && s.StateID != s.SpecID {
		out = append(out, fmt.Sprintf(
			"state.json records project_id %q but spec.yaml declares %q. Run 'roady spec lock' to reconcile it.",
			s.StateID, s.SpecID))
	}
	return out
}

// LockResult reports what WriteLock changed, so a no-op says so and the
// command is safe to re-run in a script.
type LockResult struct {
	SpecID       string
	LockUpdated  bool
	StateUpdated bool
}

// Changed reports whether anything was rewritten.
func (r LockResult) Changed() bool { return r.LockUpdated || r.StateUpdated }

// LockStatus compares the files derived from the spec against it.
func (s *SpecService) LockStatus() (*LockState, error) {
	productSpec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if productSpec == nil {
		return nil, fmt.Errorf("no spec found")
	}

	state := &LockState{SpecID: productSpec.ID}

	if locked, lErr := s.repo.LoadSpecLock(); lErr == nil && locked != nil {
		state.LockPresent = true
		state.LockID = locked.ID
	}
	if execState, sErr := s.repo.LoadState(); sErr == nil && execState != nil {
		state.StatePresent = true
		state.StateID = execState.ProjectID
	}

	return state, nil
}

// WriteLock re-captures the drift baseline from the current spec and
// reconciles the execution state's project id with it.
//
// It exists because there was no supported way to do either. An adopter who
// replaced the generated spec had to hand-write spec.lock.json — which works
// until it silently does not, since the lock is what every later drift check
// compares against.
func (s *SpecService) WriteLock() (*LockResult, error) {
	productSpec, err := s.repo.LoadSpec()
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	if productSpec == nil {
		return nil, fmt.Errorf("no spec found; nothing to lock")
	}

	result := &LockResult{SpecID: productSpec.ID}

	// The lock is rewritten only when it actually differs, so re-running is a
	// no-op rather than a spurious change in the diff.
	locked, lErr := s.repo.LoadSpecLock()
	if lErr != nil || locked == nil || !specsMatch(locked, productSpec) {
		if err := s.repo.SaveSpecLock(productSpec); err != nil {
			return nil, fmt.Errorf("write spec lock: %w", err)
		}
		result.LockUpdated = true
	}

	execState, sErr := s.repo.LoadState()
	if sErr == nil && execState != nil && execState.ProjectID != productSpec.ID {
		execState.ProjectID = productSpec.ID
		if err := s.repo.SaveState(execState); err != nil {
			return nil, fmt.Errorf("reconcile state project id: %w", err)
		}
		result.StateUpdated = true
	}

	return result, nil
}

// specsMatch reports whether the lock already captures this spec. Identity and
// shape are compared rather than the whole document, since the lock exists to
// answer "has the spec moved", not to be byte-identical.
func specsMatch(locked, current *spec.ProductSpec) bool {
	if locked.ID != current.ID || locked.Title != current.Title || locked.Version != current.Version {
		return false
	}
	if len(locked.Features) != len(current.Features) {
		return false
	}
	for i := range locked.Features {
		if locked.Features[i].ID != current.Features[i].ID {
			return false
		}
		if len(locked.Features[i].Requirements) != len(current.Features[i].Requirements) {
			return false
		}
	}
	return true
}
