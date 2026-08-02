package application

import (
	"bufio"
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
						// Merge Requirements
						mergedSpec.Features[i].Requirements = append(mergedSpec.Features[i].Requirements, newFeat.Requirements...)
						found = true
						break
					}
				}
				if !found {
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

func (s *SpecService) parseMarkdownFile(path string) (*spec.ProductSpec, error) {
	cleanPath := filepath.Clean(path)
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close() //nolint:errcheck // best-effort close on read path

	scanner := bufio.NewScanner(file)

	productSpec := &spec.ProductSpec{
		ID:          "imported-spec",
		Version:     "0.1.0",
		Constraints: []spec.Constraint{},
		Features:    []spec.Feature{},
	}

	var currentFeature *spec.Feature
	var descriptionBuilder strings.Builder
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		switch {
		case strings.HasPrefix(line, "# "):
			productSpec.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
		case strings.HasPrefix(line, "## "):
			if currentFeature != nil {
				currentFeature.Description = strings.TrimSpace(descriptionBuilder.String())
				productSpec.Features = append(productSpec.Features, *currentFeature)
				descriptionBuilder.Reset()
			}

			title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			id := spec.Slugify(title)
			currentFeature = &spec.Feature{
				ID:     id,
				Title:  title,
				Source: spec.Source{Doc: cleanPath, Line: lineNum},
			}
		default:
			if currentFeature != nil {
				descriptionBuilder.WriteString(line + "\n")
			} else if productSpec.Description == "" && line != "" {
				productSpec.Description = line
			}
		}
	}

	if currentFeature != nil {
		currentFeature.Description = strings.TrimSpace(descriptionBuilder.String())
		productSpec.Features = append(productSpec.Features, *currentFeature)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return productSpec, nil
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

	// BacklogPath is the documentation file the feature was appended to,
	// empty when no sync happened.
	BacklogPath string

	// Warnings names what did not happen, for a caller to pass on rather
	// than announce success over.
	Warnings []string
}

// Synced reports whether the feature reached the backlog document.
func (r *AddFeatureResult) Synced() bool { return r.BacklogPath != "" }

// rootedRepository is implemented by repositories that know where on disk the
// project lives. It is deliberately narrow: the spec service needs the root
// only to place documentation beside the spec it belongs to.
type rootedRepository interface {
	Root() string
}

// AddFeature adds a new functional unit and syncs it back to documentation.
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

	if err := s.repo.SaveSpec(current); err != nil {
		return nil, err
	}
	if err := s.repo.SaveSpecLock(current); err != nil {
		return nil, err
	}

	result := &AddFeatureResult{Spec: current}

	path, err := s.syncToMarkdown(newFeat)
	switch {
	case err != nil:
		// The spec is already written, so this is a partial success, not a
		// failure — but it must not be reported as a clean one.
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("the feature was added to the spec but not to %s: %v", path, err))
	default:
		result.BacklogPath = path
	}

	return result, nil
}

// backlogPath locates the backlog document for this project.
//
// It must be resolved against the project root rather than the process
// working directory: the MCP server runs from wherever it was started, which
// is routinely a different repository from the one named by project_path.
// Resolving relatively wrote one project's feature text into another's
// working tree.
func (s *SpecService) backlogPath() (string, error) {
	rooted, ok := s.repo.(rootedRepository)
	if !ok {
		return "", fmt.Errorf("this repository does not expose a project root, so the backlog document cannot be located")
	}
	root := rooted.Root()
	if root == "" {
		return "", fmt.Errorf("the project root is empty, so the backlog document cannot be located")
	}
	return filepath.Join(root, "docs", "backlog.md"), nil
}

// syncToMarkdown appends the feature to the project's backlog document,
// returning the path it wrote to.
func (s *SpecService) syncToMarkdown(f spec.Feature) (path string, err error) {
	path, err = s.backlogPath()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, fmt.Errorf("failed to create docs directory: %w", err)
	}

	content := fmt.Sprintf("\n## %s\n\n%s\n\n---\n", f.Title, f.Description)

	fWriter, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // path is derived from the project root
	if err != nil {
		return path, err
	}
	defer func() {
		if cerr := fWriter.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close file: %w", cerr)
		}
	}()

	if _, err = fWriter.WriteString(content); err != nil {
		return path, err
	}
	return path, nil
}
