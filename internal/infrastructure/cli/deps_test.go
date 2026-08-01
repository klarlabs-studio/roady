package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/domain/dependency"
)

func TestDepsListCmd(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	// Test list with no dependencies
	cmd := depsListCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.RunE(cmd, nil)
	if err != nil {
		t.Fatalf("depsListCmd failed: %v", err)
	}
}

func TestDepsListCmd_JSON(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	// Set JSON output flag
	_ = depsListCmd.Flags().Set("output", "json")
	defer func() { _ = depsListCmd.Flags().Set("output", "text") }()

	err := depsListCmd.RunE(depsListCmd, nil)
	if err != nil {
		t.Fatalf("depsListCmd --output json failed: %v", err)
	}
}

func TestDepsAddCmd_MissingFlags(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	// Test missing --repo flag
	_ = depsAddCmd.Flags().Set("repo", "")
	_ = depsAddCmd.Flags().Set("type", "")

	err := depsAddCmd.RunE(depsAddCmd, nil)
	if err == nil {
		t.Error("Expected error for missing --repo flag")
	}
	if !strings.Contains(err.Error(), "--repo is required") {
		t.Errorf("Expected '--repo is required' error, got: %v", err)
	}

	// Test missing --type flag
	_ = depsAddCmd.Flags().Set("repo", "/some/repo")
	err = depsAddCmd.RunE(depsAddCmd, nil)
	if err == nil {
		t.Error("Expected error for missing --type flag")
	}
	if !strings.Contains(err.Error(), "--type is required") {
		t.Errorf("Expected '--type is required' error, got: %v", err)
	}
}

func TestDepsGraphCmd(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	err := depsGraphCmd.RunE(depsGraphCmd, nil)
	if err != nil {
		t.Fatalf("depsGraphCmd failed: %v", err)
	}
}

func TestDepsGraphCmd_WithCycleCheck(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	// Enable cycle check
	_ = depsGraphCmd.Flags().Set("check-cycles", "true")
	defer func() { _ = depsGraphCmd.Flags().Set("check-cycles", "false") }()

	err := depsGraphCmd.RunE(depsGraphCmd, nil)
	if err != nil {
		t.Fatalf("depsGraphCmd --check-cycles failed: %v", err)
	}
}

func TestDepsScanCmd(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	err := depsScanCmd.RunE(depsScanCmd, nil)
	if err != nil {
		t.Fatalf("depsScanCmd failed: %v", err)
	}
}

func TestDepsRemoveCmd_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	roadyDir := filepath.Join(tmpDir, ".roady")
	if err := os.MkdirAll(roadyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write minimal spec file
	specContent := `name: test-project
version: "1.0.0"
`
	if err := os.WriteFile(filepath.Join(roadyDir, "spec.yaml"), []byte(specContent), 0600); err != nil {
		t.Fatal(err)
	}

	// Change to temp dir
	oldDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldDir) }()

	err := depsRemoveCmd.RunE(depsRemoveCmd, []string{"nonexistent-id"})
	if err == nil {
		t.Error("Expected error for removing non-existent dependency")
	}
}

func TestDependencyType_Validation(t *testing.T) {
	tests := []struct {
		name    string
		depType dependency.DependencyType
		valid   bool
	}{
		{"runtime", dependency.DependencyRuntime, true},
		{"data", dependency.DependencyData, true},
		{"build", dependency.DependencyBuild, true},
		{"intent", dependency.DependencyIntent, true},
		{"invalid", dependency.DependencyType("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.depType.IsValid(); got != tt.valid {
				t.Errorf("DependencyType(%s).IsValid() = %v, want %v", tt.depType, got, tt.valid)
			}
		})
	}
}
