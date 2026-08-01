package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/internal/infrastructure/config"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestLoadServicesSucceeds(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}

	_ = config.SaveAIConfig(tempDir, &config.AIConfig{Provider: "mock", Model: "test"})

	services, err := loadServices(tempDir)
	if err != nil {
		t.Fatalf("load services: %v", err)
	}
	if services == nil || services.Plan == nil || services.Prompt == nil {
		t.Fatalf("expected services, got %+v", services)
	}
}

func TestLoadServicesForCurrentDir(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}

	_ = config.SaveAIConfig(tempDir, &config.AIConfig{Provider: "mock", Model: "test"})

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get wd: %v", err)
	}
	defer func() {
		_ = os.Chdir(original)
	}()

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	services, err := loadServicesForCurrentDir()
	if err != nil {
		t.Fatalf("load services for current dir: %v", err)
	}
	if services == nil || services.Spec == nil {
		t.Fatalf("expected services, got %+v", services)
	}
}

func TestGetProjectRoot_DefaultToCwd(t *testing.T) {
	old := projectPath
	defer func() { projectPath = old }()
	projectPath = ""

	got, err := getProjectRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cwd, _ := os.Getwd()
	if got != cwd {
		t.Fatalf("expected %s, got %s", cwd, got)
	}
}

func TestGetProjectRoot_WithFlag(t *testing.T) {
	tmpDir := t.TempDir()

	old := projectPath
	defer func() { projectPath = old }()
	projectPath = tmpDir

	got, err := getProjectRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	abs, _ := filepath.Abs(tmpDir)
	if got != abs {
		t.Fatalf("expected %s, got %s", abs, got)
	}
}

func TestGetProjectRoot_InvalidPath(t *testing.T) {
	old := projectPath
	defer func() { projectPath = old }()
	projectPath = "/nonexistent/path/that/does/not/exist"

	_, err := getProjectRoot()
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "project path") {
		t.Fatalf("expected 'project path' in error, got: %v", err)
	}
}

func TestGetProjectRoot_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	original, _ := os.Getwd()
	defer func() { _ = os.Chdir(original) }()
	_ = os.Chdir(tmpDir)

	// Create a subdirectory and use relative path
	subDir := filepath.Join(tmpDir, "subproject")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	old := projectPath
	defer func() { projectPath = old }()
	projectPath = "subproject"

	got, err := getProjectRoot()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Resolve symlinks for macOS where /var -> /private/var
	wantResolved, _ := filepath.EvalSymlinks(subDir)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved {
		t.Fatalf("expected %s, got %s", wantResolved, gotResolved)
	}
}

func TestGetProjectRoot_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "notadir.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	old := projectPath
	defer func() { projectPath = old }()
	projectPath = filePath

	_, err := getProjectRoot()
	if err == nil {
		t.Fatal("expected error for file path")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("expected 'not a directory' in error, got: %v", err)
	}
}

func TestLoadServices_AIProviderFails(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatalf("initialize repo: %v", err)
	}

	// There is no provider to configure any more — Roady assembles prompts
	// and the caller runs inference — so services must load regardless.
	svc, err := loadServices(tempDir)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if svc == nil || svc.Prompt == nil {
		t.Fatal("expected services with a prompt builder")
	}
}
