package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"go.klarlabs.de/fortify/retry"
	"gopkg.in/yaml.v3"
)

const RoadyDir = ".roady"
const ProjectsDir = "projects"
const SpecFile = "spec.yaml"
const SpecLockFile = "spec.lock.json"
const PlanFile = "plan.json"
const PolicyFile = "policy.yaml"
const StateFile = "state.json"
const EventsFile = "events.jsonl"

// projectNamePattern restricts sub-project names to a safe, lowercase, slug-like form.
// Names must start with [a-z0-9] and may contain [a-z0-9._-]. Max 64 chars.
// The literal "projects" is reserved because it is the parent directory name.
var projectNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// ValidateProjectName returns nil if name is a valid sub-project identifier.
// Empty name is valid and refers to the root project.
func ValidateProjectName(name string) error {
	if name == "" {
		return nil
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid project name: %q", name)
	}
	if name == ProjectsDir {
		return fmt.Errorf("project name %q is reserved", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("project name must not contain path separators: %q", name)
	}
	if !projectNamePattern.MatchString(name) {
		return fmt.Errorf("project name must match %s: %q", projectNamePattern, name)
	}
	return nil
}

type FilesystemRepository struct {
	root        string
	subProject  string // empty = root project; otherwise stored under .roady/projects/<name>/
	retryConfig retry.Config
}

// NewFilesystemRepository returns a repository scoped to the root project
// (<root>/.roady/). Equivalent to NewFilesystemRepositoryForProject(root, "").
func NewFilesystemRepository(root string) *FilesystemRepository {
	return &FilesystemRepository{
		root: root,
		retryConfig: retry.Config{
			MaxAttempts:   3,
			InitialDelay:  10 * time.Millisecond,
			BackoffPolicy: retry.BackoffExponential,
		},
	}
}

// NewFilesystemRepositoryForProject returns a repository scoped to a named
// sub-project at <root>/.roady/projects/<project>/. When project is empty,
// behaves like NewFilesystemRepository. Returns an error if the project name
// is invalid.
func NewFilesystemRepositoryForProject(root, project string) (*FilesystemRepository, error) {
	if err := ValidateProjectName(project); err != nil {
		return nil, err
	}
	r := NewFilesystemRepository(root)
	r.subProject = project
	return r, nil
}

// Root returns the workspace root directory.
func (r *FilesystemRepository) Root() string {
	return r.root
}

// SubProject returns the sub-project name this repository is scoped to,
// or "" if it is scoped to the root project.
func (r *FilesystemRepository) SubProject() string {
	return r.subProject
}

// IsSubProject reports whether this repository is scoped to a named
// sub-project rather than the root project.
func (r *FilesystemRepository) IsSubProject() bool {
	return r.subProject != ""
}

// ProjectBase returns the directory that contains this project's files.
// For the root project that is <root>/.roady. For a sub-project it is
// <root>/.roady/projects/<name>.
func (r *FilesystemRepository) ProjectBase() string {
	base := filepath.Join(r.root, RoadyDir)
	if r.subProject == "" {
		return base
	}
	return filepath.Join(base, ProjectsDir, r.subProject)
}

// ResolvePath ensures the path is within this project's directory and prevents
// traversal. For root projects the base is <root>/.roady; for sub-projects it
// is <root>/.roady/projects/<name>. Only direct children are allowed.
func (r *FilesystemRepository) ResolvePath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("filename cannot be empty")
	}

	baseDir := r.ProjectBase()
	fullPath := filepath.Join(baseDir, filename)
	cleanPath := filepath.Clean(fullPath)

	// Ensure the resolved path is a direct child of baseDir.
	if !strings.HasPrefix(cleanPath, baseDir) || filepath.Dir(cleanPath) != baseDir {
		return "", fmt.Errorf("invalid file path: %s", filename)
	}

	return cleanPath, nil
}

func (r *FilesystemRepository) Initialize() error {
	path := r.ProjectBase()
	// G301: Use 0700 for directories
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}
	return nil
}

func (r *FilesystemRepository) IsInitialized() bool {
	_, err := os.Stat(r.ProjectBase())
	return err == nil
}

func (r *FilesystemRepository) SaveSpec(s *spec.ProductSpec) error {
	path, err := r.ResolvePath(SpecFile)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}

	// G306: Use 0600 for files
	return os.WriteFile(path, data, 0600)
}

func (r *FilesystemRepository) LoadSpec() (*spec.ProductSpec, error) {
	retryer := retry.New[*spec.ProductSpec](r.retryConfig)

	return retryer.Execute(context.Background(), func(ctx context.Context) (*spec.ProductSpec, error) {
		path, err := r.ResolvePath(SpecFile)
		if err != nil {
			return nil, err
		}

		// #nosec G304 -- Path is resolved and validated via resolvePath
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read spec file: %w", err)
		}

		var s spec.ProductSpec
		if err := yaml.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}

		return &s, nil
	})
}

func (r *FilesystemRepository) SaveSpecLock(s *spec.ProductSpec) error {
	path, err := r.ResolvePath(SpecLockFile)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal spec lock: %w", err)
	}

	return os.WriteFile(path, data, 0600)
}

func (r *FilesystemRepository) LoadSpecLock() (*spec.ProductSpec, error) {
	path, err := r.ResolvePath(SpecLockFile)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- Path is resolved and validated via ResolvePath
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec lock file: %w", err)
	}

	var s spec.ProductSpec
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec lock: %w", err)
	}

	return &s, nil
}
