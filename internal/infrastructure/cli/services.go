package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/felixgeelhaar/roady/internal/infrastructure/wiring"
)

func loadServices(root string) (*wiring.AppServices, error) {
	return loadServicesForProject(root, currentSubProject())
}

func loadServicesForProject(root, project string) (*wiring.AppServices, error) {
	services, loadErr := wiring.BuildAppServicesForProject(root, project)
	if services == nil {
		return nil, fmt.Errorf("failed to build services: %w", loadErr)
	}
	if loadErr != nil {
		// Warnings go to stderr so they never corrupt machine-readable
		// stdout — `roady report > status.md` and every `--json` flag
		// depend on stdout carrying the payload alone.
		fmt.Fprintf(os.Stderr, "Warning: %v\n", loadErr)
	}
	return services, nil
}

// currentSubProject resolves the active sub-project name from the --project
// flag, falling back to the ROADY_PROJECT environment variable. Empty string
// means the root project.
func currentSubProject() string {
	if subProjectFlag != "" {
		return subProjectFlag
	}
	return os.Getenv("ROADY_PROJECT")
}

func getProjectRoot() (string, error) {
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return "", fmt.Errorf("invalid project path %q: %w", projectPath, err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			return "", fmt.Errorf("project path %q: %w", abs, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("project path %q is not a directory", abs)
		}
		return abs, nil
	}
	return os.Getwd()
}

func loadServicesForCurrentDir() (*wiring.AppServices, error) {
	root, err := getProjectRoot()
	if err != nil {
		return nil, err
	}
	return loadServicesForProject(root, currentSubProject())
}
