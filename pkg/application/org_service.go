package application

import (
	"context"
	"os"
	"path/filepath"

	"github.com/felixgeelhaar/roady/pkg/domain/org"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/policy"
	"github.com/felixgeelhaar/roady/pkg/storage"
	"gopkg.in/yaml.v3"
)

// OrgService provides organizational multi-project operations.
type OrgService struct {
	root string
}

// NewOrgService creates a new OrgService rooted at the given directory.
func NewOrgService(root string) *OrgService {
	return &OrgService{root: root}
}

// DiscoveredProject identifies one project found during a walk.
// SubProject is empty for the root project of a repo. For sub-projects
// stored under <Path>/.roady/projects/<name>/, SubProject is set to <name>.
type DiscoveredProject struct {
	Path       string
	SubProject string
}

// DiscoverProjects walks the root directory tree and returns paths containing
// .roady directories. Backward-compatible shape — only root projects are
// returned. For full sub-project discovery use DiscoverProjectsWithSub.
func (s *OrgService) DiscoverProjects() ([]string, error) {
	found, err := s.DiscoverProjectsWithSub()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, p := range found {
		if p.SubProject == "" {
			paths = append(paths, p.Path)
		}
	}
	return paths, nil
}

// DiscoverProjectsWithSub walks the root directory tree and returns every
// project found — both the root project of each repo (where a .roady/ lives)
// and every named sub-project under <repo>/.roady/projects/<name>/.
func (s *OrgService) DiscoverProjectsWithSub() ([]DiscoveredProject, error) {
	var projects []DiscoveredProject
	err := filepath.Walk(s.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() && info.Name() == ".roady" {
			repoRoot := filepath.Dir(path)
			projects = append(projects, DiscoveredProject{Path: repoRoot})

			projects = append(projects, s.subProjectsOf(repoRoot)...)
			return filepath.SkipDir
		}
		return nil
	})
	return projects, err
}

// AggregateMetrics collects metrics across the workspace's member
// repositories, including sub-projects under each repo's
// .roady/projects/<name>/.
//
// Membership comes from org.yaml when it declares repos, so the aggregate
// covers repositories outside this directory and excludes checkouts inside it
// that nobody claimed. Without a declaration it walks the tree as before.
func (s *OrgService) AggregateMetrics() (*org.OrgMetrics, error) {
	projects, members, err := s.memberProjects()
	if err != nil {
		return nil, err
	}

	config, _ := s.LoadOrgConfig()
	orgName := ""
	if config != nil {
		orgName = config.Name
	}

	metrics := &org.OrgMetrics{
		OrgName:  orgName,
		Warnings: members.Problems(),
	}

	for _, p := range projects {
		pm := s.projectMetricsFor(p)
		metrics.Projects = append(metrics.Projects, pm)
		metrics.TotalTasks += pm.Total
		metrics.TotalVerified += pm.Verified
		metrics.TotalWIP += pm.WIP
	}

	metrics.TotalProjects = len(metrics.Projects)
	if metrics.TotalProjects > 0 {
		var sum float64
		for _, pm := range metrics.Projects {
			sum += pm.Progress
		}
		metrics.AvgProgress = sum / float64(metrics.TotalProjects)
	}

	return metrics, nil
}

// projectMetricsFor reports metrics for a discovered project entry, supporting
// both root projects and sub-projects under <Path>/.roady/projects/<SubProject>.
func (s *OrgService) projectMetricsFor(p DiscoveredProject) org.ProjectMetrics {
	repo, repoErr := storage.NewFilesystemRepositoryForProject(p.Path, p.SubProject)
	if repoErr != nil {
		// Invalid sub-project name; fall back to legacy root repo so we don't
		// silently drop the entry.
		repo = storage.NewFilesystemRepository(p.Path)
	}
	spec, _ := repo.LoadSpec()
	plan, _ := repo.LoadPlan()
	state, _ := repo.LoadState()

	displayName := filepath.Base(p.Path)
	if p.SubProject != "" {
		displayName = filepath.Base(p.Path) + "/" + p.SubProject
	}
	pm := org.ProjectMetrics{
		Name: displayName,
		Path: p.Path,
	}
	if p.SubProject != "" {
		// Surface the sub-project path so callers see the actual on-disk location.
		pm.Path = repo.ProjectBase()
	}

	if absPath, err := filepath.Abs(pm.Path); err == nil {
		pm.Path = absPath
	}

	if spec != nil {
		pm.Name = spec.Title
		if p.SubProject != "" && spec.Title != "" {
			pm.Name = spec.Title + " (" + p.SubProject + ")"
		}
	}

	if plan != nil {
		pm.Total = len(plan.Tasks)
		for _, t := range plan.Tasks {
			if state != nil {
				if res, ok := state.TaskStates[t.ID]; ok {
					switch res.Status {
					case planning.StatusVerified:
						pm.Verified++
					case planning.StatusDone:
						pm.Done++
					case planning.StatusInProgress:
						pm.WIP++
					case planning.StatusBlocked:
						pm.Blocked++
					default:
						pm.Pending++
					}
					continue
				}
			}
			pm.Pending++
		}
		if pm.Total > 0 {
			pm.Progress = float64(pm.Verified) / float64(pm.Total) * 100
		}
	}

	// Check drift
	auditSvc := NewAuditService(repo)
	policySvc := NewPolicyService(repo)
	inspector := storage.NewCodebaseInspector()
	driftSvc := NewDriftService(repo, auditSvc, inspector, policySvc)
	if driftReport, err := driftSvc.DetectDrift(context.Background()); err == nil && driftReport != nil {
		if len(driftReport.Issues) > 0 {
			pm.HasDrift = true
			pm.DriftCount = len(driftReport.Issues)
		}
	}

	return pm
}

// LoadOrgConfig loads the org config from .roady/org.yaml in the root directory.
func (s *OrgService) LoadOrgConfig() (*org.OrgConfig, error) {
	path := filepath.Join(s.root, ".roady", "org.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var config org.OrgConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

// SaveOrgConfig saves the org config to .roady/org.yaml in the root directory.
func (s *OrgService) SaveOrgConfig(config *org.OrgConfig) error {
	dir := filepath.Join(s.root, ".roady")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "org.yaml"), data, 0600)
}

// LoadMergedPolicy loads org-level SharedPolicy and overlays project-level policy.yaml values.
func (s *OrgService) LoadMergedPolicy(projectPath string) (*policy.PolicyConfig, error) {
	merged := &policy.PolicyConfig{}

	// Load org-level shared policy
	config, err := s.LoadOrgConfig()
	if err == nil && config != nil && config.SharedPolicy != nil {
		merged.MaxWIP = config.SharedPolicy.MaxWIP
		merged.AllowAI = config.SharedPolicy.AllowAI
		merged.TokenLimit = config.SharedPolicy.TokenLimit
	}

	// Overlay project-level policy
	repo := storage.NewFilesystemRepository(projectPath)
	projectPolicy, err := repo.LoadPolicy()
	if err == nil && projectPolicy != nil {
		if projectPolicy.MaxWIP > 0 {
			merged.MaxWIP = projectPolicy.MaxWIP
		}
		if projectPolicy.AllowAI {
			merged.AllowAI = true
		}
		if projectPolicy.TokenLimit > 0 {
			merged.TokenLimit = projectPolicy.TokenLimit
		}
	}

	return merged, nil
}

// DetectCrossDrift aggregates drift reports across the workspace's member
// repositories, including their sub-projects.
func (s *OrgService) DetectCrossDrift() (*org.CrossDriftReport, error) {
	projects, members, err := s.memberProjects()
	if err != nil {
		return nil, err
	}

	report := &org.CrossDriftReport{Warnings: members.Problems()}

	for _, p := range projects {
		repo, repoErr := storage.NewFilesystemRepositoryForProject(p.Path, p.SubProject)
		if repoErr != nil {
			continue
		}
		auditSvc := NewAuditService(repo)
		policySvc := NewPolicyService(repo)
		inspector := storage.NewCodebaseInspector()
		driftSvc := NewDriftService(repo, auditSvc, inspector, policySvc)

		driftReport, err := driftSvc.DetectDrift(context.Background())
		displayPath := p.Path
		if p.SubProject != "" {
			displayPath = repo.ProjectBase()
		}
		displayName := filepath.Base(p.Path)
		if p.SubProject != "" {
			displayName = filepath.Base(p.Path) + "/" + p.SubProject
		}
		summary := org.ProjectDriftSummary{
			Name: displayName,
			Path: displayPath,
		}

		if absPath, absErr := filepath.Abs(displayPath); absErr == nil {
			summary.Path = absPath
		}

		// Try to get project name from spec
		if spec, specErr := repo.LoadSpec(); specErr == nil && spec != nil {
			summary.Name = spec.Title
			if p.SubProject != "" && spec.Title != "" {
				summary.Name = spec.Title + " (" + p.SubProject + ")"
			}
		}

		if err == nil && driftReport != nil && len(driftReport.Issues) > 0 {
			summary.IssueCount = len(driftReport.Issues)
			summary.HasDrift = true
			report.TotalIssues += summary.IssueCount
		}

		report.Projects = append(report.Projects, summary)
	}

	return report, nil
}
