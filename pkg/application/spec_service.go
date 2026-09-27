package application

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
)

type SpecService struct {
	repo domain.WorkspaceRepository
}

func NewSpecService(repo domain.WorkspaceRepository) *SpecService {
	return &SpecService{repo: repo}
}

// ImportFromMarkdown reads a markdown file and converts it into a ProductSpec.
func (s *SpecService) ImportFromMarkdown(path string) (*spec.ProductSpec, error) {
	productSpec, err := s.parseMarkdownFile(path)
	if err != nil {
		return nil, err
	}

	if err := s.repo.SaveSpec(productSpec); err != nil {
		return nil, fmt.Errorf("failed to save spec: %w", err)
	}

	return productSpec, nil
}

// AnalyzeDirectory crawls a directory for markdown files and merges them into a single Spec.
func (s *SpecService) AnalyzeDirectory(root string) (*spec.ProductSpec, error) {
	mergedSpec := &spec.ProductSpec{
		ID:          "analyzed-spec",
		Version:     "0.1.0",
		Constraints: []spec.Constraint{},
		Features:    []spec.Feature{},
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".md") || strings.HasSuffix(info.Name(), ".markdown")) {
			// Skip roady internal docs or hidden files
			if strings.Contains(path, ".roady") || strings.HasPrefix(info.Name(), ".") {
				return nil
			}

			fileSpec, err := s.parseMarkdownFile(path)
			if err != nil {
				return nil // Skip files that fail to parse
			}

			// 1. Merge Title/Description
			if mergedSpec.Title == "" {
				mergedSpec.Title = fileSpec.Title
			}
			if mergedSpec.Description == "" {
				mergedSpec.Description = fileSpec.Description
			}

			// 2. Intelligent Feature Merge
			for _, newFeat := range fileSpec.Features {
				found := false
				for i, existingFeat := range mergedSpec.Features {
					if existingFeat.ID == newFeat.ID {
						// Merge Angle: Append descriptions with a separator
						if !strings.Contains(existingFeat.Description, newFeat.Description) {
							mergedSpec.Features[i].Description += "\n\n---\n\n" + newFeat.Description
						}
						// Merge requirements by id: the same requirement in
						// two documents is one requirement, not two.
						for _, r := range newFeat.Requirements {
							if requirementIndex(mergedSpec.Features[i], r.ID) >= 0 {
								continue
							}
							r.ID = uniqueRequirementID(mergedSpec, r.ID)
							mergedSpec.Features[i].Requirements = append(mergedSpec.Features[i].Requirements, r)
						}
						found = true
						break
					}
				}
				if !found {
					// Requirement ids are unique across the spec; one file
					// cannot see another's, so collisions are settled here.
					for k := range newFeat.Requirements {
						newFeat.Requirements[k].ID = uniqueRequirementID(mergedSpec, newFeat.Requirements[k].ID)
					}
					mergedSpec.Features = append(mergedSpec.Features, newFeat)
				}
			}

			// 3. Merge Constraints
			mergedSpec.Constraints = append(mergedSpec.Constraints, fileSpec.Constraints...)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	if len(mergedSpec.Features) == 0 {
		return nil, fmt.Errorf("no features found in directory: %s", root)
	}

	s.preserveExistingFeatureIDs(mergedSpec)

	if err := s.repo.SaveSpec(mergedSpec); err != nil {
		return nil, fmt.Errorf("failed to save merged spec: %w", err)
	}

	return mergedSpec, nil
}

// preserveExistingFeatureIDs keeps the id a feature already had, matching on
// title.
//
// Ids are derived from titles, so any change to how they are derived — a
// better slugifier, say — would otherwise rename every feature in an existing
// spec on the next analysis. Tasks reference features by id, so that rename
// would orphan the entire plan and, worse, do it silently. A feature Roady has
// already recorded therefore keeps its id even if today's slugifier would
// spell it differently; only genuinely new features get the current form.
func (s *SpecService) preserveExistingFeatureIDs(merged *spec.ProductSpec) {
	existing, err := s.repo.LoadSpec()
	if err != nil || existing == nil {
		return
	}

	idByTitle := make(map[string]string, len(existing.Features))
	for _, f := range existing.Features {
		if f.Title != "" && f.ID != "" {
			idByTitle[f.Title] = f.ID
		}
	}
	if len(idByTitle) == 0 {
		return
	}

	// Ids stay unique: a preserved id must not collide with one already
	// claimed in this analysis.
	taken := make(map[string]bool, len(merged.Features))
	for _, f := range merged.Features {
		taken[f.ID] = true
	}

	for i := range merged.Features {
		previous, ok := idByTitle[merged.Features[i].Title]
		if !ok || previous == merged.Features[i].ID {
			continue
		}
		if taken[previous] {
			continue
		}
		delete(taken, merged.Features[i].ID)
		taken[previous] = true
		merged.Features[i].ID = previous
	}
}

func (s *SpecService) GetSpec() (*spec.ProductSpec, error) {

	return s.repo.LoadSpec()

}

// AddFeatureResult reports what adding a feature actually changed.
//
// The documentation sync can fail — or be skipped — while the spec write
// succeeds, so the outcome is more than a spec. Reporting a flat success in
// that case tells the caller its docs were updated when they were not.
type AddFeatureResult struct {
	// Spec is the specification including the new feature.
	Spec *spec.ProductSpec

	// Warnings names what did not happen, for a caller to pass on rather
	// than announce success over.
	Warnings []string
}

// rootedRepository is implemented by repositories that know where on disk the
// project lives. It is deliberately narrow: the spec service needs the root
// only to place documentation beside the spec it belongs to.
type rootedRepository interface {
	Root() string
}

// AddFeature adds a new functional unit to the spec. It writes no markdown:
// intent lives in roady, and ROADMAP.md is rendered from goals.
func (s *SpecService) AddFeature(title, description string) (*AddFeatureResult, error) {
	current, err := s.repo.LoadSpec()
	if err != nil {
		return nil, fmt.Errorf("failed to load spec: %w", err)
	}

	newFeat := spec.Feature{
		ID:          spec.Slugify(title),
		Title:       title,
		Description: description,
	}

	current.Features = append(current.Features, newFeat)

	// Adding a feature re-locks the whole spec, which would also bless any
	// check loosened by hand in spec.yaml since the last lock.
	guard := NewCheckGuard(s.repo, nil)
	if err := guard.Authorize(guard.Inspect(current, nil), false); err != nil {
		return nil, err
	}

	if err := s.repo.SaveSpec(current); err != nil {
		return nil, err
	}
	if err := s.repo.SaveSpecLock(current); err != nil {
		return nil, err
	}

	result := &AddFeatureResult{Spec: current}
	return result, nil
}

// uniqueRequirementID returns id, suffixed if another feature in sp already
// uses it.
func uniqueRequirementID(sp *spec.ProductSpec, id string) string {
	taken := func(candidate string) bool {
		for _, f := range sp.Features {
			if requirementIndex(f, candidate) >= 0 {
				return true
			}
		}
		return false
	}
	out := id
	for n := 2; taken(out); n++ {
		out = fmt.Sprintf("%s-%d", id, n)
	}
	return out
}
