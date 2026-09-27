package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

func (r *FilesystemRepository) SavePlan(p *planning.Plan) error {
	// Stamp the write here, at the one funnel every plan mutation passes
	// through, rather than leaving each caller to remember. Approving a plan
	// rewrote plan.json without touching UpdatedAt, so a plan could be modified
	// while its timestamp stayed months old — and drift, which reads exactly
	// this field to decide whether the repository has left the plan behind,
	// then reported it stale at critical severity and pointed the operator at a
	// regeneration they did not need. A field that only some writers maintain
	// is a field that lies. See issue #76.
	p.UpdatedAt = time.Now()
	return r.writePlan(p)
}

// RewritePlan saves a correction that leaves what the plan describes as it
// was — repaired feature links — without stamping it as updated. SavePlan's
// stamp says the plan was touched now, and drift reads it as "the plan still
// describes the work"; a repair says nothing of the kind, and stamping it hid
// 79 days of staleness on mcp-go.
func (r *FilesystemRepository) RewritePlan(p *planning.Plan) error {
	return r.writePlan(p)
}

func (r *FilesystemRepository) writePlan(p *planning.Plan) error {
	path, err := r.ResolvePath(PlanFile)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

func (r *FilesystemRepository) LoadPlan() (*planning.Plan, error) {
	if _, err := os.Stat(r.root); err != nil {
		return nil, fmt.Errorf("root directory does not exist: %w", err)
	}

	path, err := r.ResolvePath(PlanFile)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- Path is resolved and validated via resolvePath
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return nil, nil to indicate no plan exists yet
		}
		return nil, fmt.Errorf("failed to read plan file: %w", err)
	}

	var p planning.Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan: %w", err)
	}

	return &p, nil
}
