package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/domain/policy/rules"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

type PolicyService struct {
	repo domain.WorkspaceRepository
}

func NewPolicyService(repo domain.WorkspaceRepository) *PolicyService {
	return &PolicyService{repo: repo}
}

// CheckCompliance validates the current plan against active policies.
func (s *PolicyService) CheckCompliance() ([]policy.Violation, error) {
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return nil, fmt.Errorf("failed to load plan: %w", err)
	}

	state, err := s.repo.LoadState()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	cfg, err := s.repo.LoadPolicy()
	if err != nil {
		return nil, fmt.Errorf("failed to load policy: %w", err)
	}

	var activeRules []policy.Rule
	if cfg != nil {
		activeRules = append(activeRules, &rules.MaxWIPRule{Limit: cfg.MaxWIP})
		activeRules = append(activeRules, &rules.MaxWIPPerOwnerRule{Limit: cfg.MaxWIPPerOwner})
	}
	activeRules = append(activeRules, &rules.DependencyRule{})

	policySet := policy.PolicySet{
		Rules: activeRules,
	}

	return policySet.Validate(plan, state), nil
}

func (s *PolicyService) ValidateTransition(taskID string, event string) error {
	return s.ValidateTransitionForOwner(taskID, event, "")
}

// ValidateTransitionForOwner is ValidateTransition with the acting owner
// known, which is what per-owner WIP limits need. An empty owner skips the
// per-owner check.
func (s *PolicyService) ValidateTransitionForOwner(taskID, event, owner string) error {
	// Role enforcement covers every transition, not just start — a viewer
	// must not be able to complete or reopen work either.
	if err := s.ValidateActorCanTransition(owner); err != nil {
		return err
	}

	if event != "start" {
		return nil
	}

	// 1. Check WIP Limits
	cfg, err := s.repo.LoadPolicy()
	if err == nil && cfg != nil && (cfg.MaxWIP > 0 || cfg.MaxWIPPerOwner > 0) {
		state, err := s.repo.LoadState()
		if err == nil {
			inProgressCount := 0
			ownerCount := 0
			wantOwner := strings.ToLower(strings.TrimSpace(owner))

			for id, ts := range state.TaskStates {
				if id == taskID || ts.Status != "in_progress" {
					continue
				}
				inProgressCount++
				if wantOwner != "" && strings.ToLower(strings.TrimSpace(ts.Owner)) == wantOwner {
					ownerCount++
				}
			}

			if cfg.MaxWIP > 0 && inProgressCount >= cfg.MaxWIP {
				return fmt.Errorf("WIP limit reached (current limit: %d); please complete or stop an existing task before starting a new one", cfg.MaxWIP)
			}
			if cfg.MaxWIPPerOwner > 0 && wantOwner != "" && ownerCount >= cfg.MaxWIPPerOwner {
				return fmt.Errorf("per-owner WIP limit reached for %s (current limit: %d); complete or stop one of their tasks first", strings.TrimSpace(owner), cfg.MaxWIPPerOwner)
			}
		}
	}

	// 2. Check Dependencies
	plan, err := s.repo.LoadPlan()
	if err != nil {
		return err
	}
	if plan == nil {
		return nil // No plan, skip dependency validation
	}

	var targetTask *planning.Task
	for _, t := range plan.Tasks {
		if t.ID == taskID {
			targetTask = &t
			break
		}
	}

	if targetTask != nil && len(targetTask.DependsOn) > 0 {
		state, err := s.repo.LoadState()
		if err != nil {
			return err
		}
		for _, depID := range targetTask.DependsOn {
			// Handle Cross-repo Dependency (format: "project-name:task-id")
			if strings.Contains(depID, ":") {
				parts := strings.Split(depID, ":")
				extProject, extTask := parts[0], parts[1]

				// Discovery loop to find the project
				extRepoPath, found := s.findExternalProject(extProject)
				if !found {
					return fmt.Errorf("cannot start task '%s': depends on external project '%s' which cannot be found", taskID, extProject)
				}

				extRepo := storage.NewFilesystemRepository(extRepoPath)
				extState, err := extRepo.LoadState()
				if err != nil {
					return fmt.Errorf("cannot verify dependency '%s': failed to load external state", depID)
				}

				extStatus := planning.StatusPending
				if res, ok := extState.TaskStates[extTask]; ok {
					extStatus = res.Status
				}

				if extStatus != planning.StatusDone && extStatus != planning.StatusVerified {
					return fmt.Errorf("cannot start task '%s': it depends on '%s' in project '%s', which is currently '%s'", taskID, extTask, extProject, extStatus)
				}
				continue
			}

			// Local dependency
			if res, ok := state.TaskStates[depID]; !ok || (res.Status != planning.StatusDone && res.Status != planning.StatusVerified) {
				return fmt.Errorf("cannot start task '%s': it depends on '%s', which is not yet completed", taskID, depID)
			}
		}
	}

	return nil
}

func (s *PolicyService) findExternalProject(name string) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}

	// Start from parent of current repo to find siblings
	root := filepath.Dir(cwd)

	foundPath := ""
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}
		if info.IsDir() && info.Name() == ".roady" {
			projectDir := filepath.Dir(path)
			repo := storage.NewFilesystemRepository(projectDir)
			spec, loadErr := repo.LoadSpec()
			if loadErr == nil && spec != nil && (spec.ID == name || spec.Title == name) {
				foundPath = projectDir
				return filepath.SkipDir
			}
		}
		return nil
	})

	if walkErr != nil {
		return "", false
	}

	return foundPath, foundPath != ""
}
