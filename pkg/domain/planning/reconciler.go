package planning

import (
	"fmt"
	"time"
)

// PlanReconciler is a domain service that handles merging proposed tasks
// with existing plan state.
type PlanReconciler struct{}

// NewPlanReconciler creates a new PlanReconciler instance.
func NewPlanReconciler() *PlanReconciler {
	return &PlanReconciler{}
}

// ReconcileOptions contains options for the reconciliation process.
type ReconcileOptions struct {
	SpecID     string
	ExistingID string    // Existing plan ID to preserve
	CreatedAt  time.Time // Original creation time to preserve
}

// Reconcile merges proposed tasks with the existing plan state.
// It preserves orphan tasks (tasks that exist in the old plan but not in proposed)
// and validates the resulting dependency graph.
func (r *PlanReconciler) Reconcile(existing *Plan, proposedTasks []Task, opts ReconcileOptions) (*Plan, error) {
	planID := fmt.Sprintf("plan-%s-%d", opts.SpecID, time.Now().Unix())
	createdAt := time.Now()

	currentTaskState := make(map[string]Task)

	if existing != nil {
		planID = existing.ID
		createdAt = existing.CreatedAt
		for _, t := range existing.Tasks {
			currentTaskState[t.ID] = t
		}
	}

	// Override with explicit options if provided
	if opts.ExistingID != "" {
		planID = opts.ExistingID
	}
	if !opts.CreatedAt.IsZero() {
		createdAt = opts.CreatedAt
	}

	newPlan := &Plan{
		ID:             planID,
		SpecID:         opts.SpecID,
		ApprovalStatus: ApprovalPending,
		CreatedAt:      createdAt,
		UpdatedAt:      time.Now(),
		Tasks:          make([]Task, 0),
	}

	// Process proposed tasks
	for _, proposed := range proposedTasks {
		if proposed.ID == "" || proposed.Title == "" {
			continue // Skip malformed proposed tasks
		}
		// Task already exists - use the proposed structure.
		// Execution state (Status/Path) is persisted separately in state.json.
		delete(currentTaskState, proposed.ID)
		newPlan.Tasks = append(newPlan.Tasks, proposed)
	}

	// Keep Orphans (tasks that were manual or already exist but weren't in proposed)
	for _, orphan := range currentTaskState {
		if orphan.ID == "" || orphan.Title == "" {
			continue // Auto-clean hallucinations from history
		}
		newPlan.Tasks = append(newPlan.Tasks, orphan)
	}

	// Validate the dependency graph
	if err := newPlan.ValidateDAG(); err != nil {
		return nil, fmt.Errorf("invalid plan dependency graph: %w", err)
	}

	return newPlan, nil
}

// FilterValidTasks returns only tasks that are valid according to the given
// task and feature ID sets.
func (r *PlanReconciler) FilterValidTasks(tasks []Task, validTaskIDs, validFeatureIDs map[string]bool) []Task {
	result := make([]Task, 0, len(tasks))
	for _, t := range tasks {
		// Task is valid if it matches a requirement ID OR its feature ID
		// exists. A task with no feature is unplanned work, kept on purpose.
		if validTaskIDs[t.ID] || validFeatureIDs[t.FeatureID] || t.FeatureID == "" {
			result = append(result, t)
		}
	}
	return result
}
