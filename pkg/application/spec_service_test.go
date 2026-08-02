package application_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestSpecService_ImportFromMarkdown(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-spec-test-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewSpecService(repo)

	mdPath := filepath.Join(tempDir, "test.md")
	content := "# My Project\n\n## Feature 1\nDescription 1"
	_ = os.WriteFile(mdPath, []byte(content), 0600)

	s, err := service.ImportFromMarkdown(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "My Project" {
		t.Errorf("Expected title My Project, got %s", s.Title)
	}
}

func TestSpecService_ComplexMarkdown(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-spec-complex-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewSpecService(repo)

	mdPath := filepath.Join(tempDir, "complex.md")
	content := "# Project\nHigh level desc\n\n## F1\nDesc 1\n- item 1\n\n## F2\nDesc 2"
	_ = os.WriteFile(mdPath, []byte(content), 0600)

	s, err := service.ImportFromMarkdown(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(s.Features))
	}
}

func TestSpecService_LeadingDesc(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-spec-leading-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewSpecService(repo)

	mdPath := filepath.Join(tempDir, "leading.md")
	content := "This is a leading description.\n\n# Project Name"
	_ = os.WriteFile(mdPath, []byte(content), 0600)

	s, _ := service.ImportFromMarkdown(mdPath)
	if s.Description != "This is a leading description." {
		t.Errorf("expected leading desc, got %s", s.Description)
	}
}

func TestSpecService_GetSpec(t *testing.T) {
	repo := &MockRepo{Spec: &spec.ProductSpec{ID: "s1"}}
	service := application.NewSpecService(repo)
	s, _ := service.GetSpec()
	if s.ID != "s1" {
		t.Errorf("GetSpec failed")
	}
}

func TestSpecService_Import_Mock(t *testing.T) {
	repo := &MockRepo{}
	service := application.NewSpecService(repo)

	tempFile, _ := os.CreateTemp("", "import-*.md")
	defer func() { _ = os.Remove(tempFile.Name()) }()
	if _, err := tempFile.WriteString("# Hello"); err != nil {
		t.Fatal(err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatal(err)
	}

	_, err := service.ImportFromMarkdown(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if repo.Spec.Title != "Hello" {
		t.Errorf("Expected Title Hello, got %s", repo.Spec.Title)
	}
}

func TestSpecService_ImportError(t *testing.T) {
	repo := &MockRepo{}
	service := application.NewSpecService(repo)

	// File not found
	_, err := service.ImportFromMarkdown("/tmp/nonexistent-file-12345")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSpecService_AnalyzeDirectory(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-spec-analyze-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewSpecService(repo)

	first := filepath.Join(tempDir, "a.md")
	second := filepath.Join(tempDir, "b.md")
	_ = os.WriteFile(first, []byte("# Project\n\n## Feature One\nDesc A"), 0600)
	_ = os.WriteFile(second, []byte("# Project\n\n## Feature One\nDesc B"), 0600)

	spec, err := service.AnalyzeDirectory(tempDir)
	if err != nil {
		t.Fatalf("AnalyzeDirectory failed: %v", err)
	}
	if len(spec.Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(spec.Features))
	}
	if spec.Title != "Project" {
		t.Fatalf("expected project title, got %q", spec.Title)
	}
	if !strings.Contains(spec.Features[0].Description, "Desc B") {
		t.Fatalf("expected merged description, got %q", spec.Features[0].Description)
	}
}

func TestSpecService_AnalyzeDirectory_RecordsSourceCitations(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-spec-source-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewSpecService(repo)

	docPath := filepath.Join(tempDir, "auth.md")
	body := "# App\n\nIntro line.\n\n## User Authentication\nDescription.\n\n## Dashboard\nDesc.\n"
	if err := os.WriteFile(docPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}

	productSpec, err := service.AnalyzeDirectory(tempDir)
	if err != nil {
		t.Fatalf("AnalyzeDirectory: %v", err)
	}
	if len(productSpec.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(productSpec.Features))
	}

	for _, f := range productSpec.Features {
		if f.Source.IsZero() {
			t.Errorf("feature %q lost source citation", f.ID)
		}
		if f.Source.Doc == "" || f.Source.Line <= 0 {
			t.Errorf("feature %q invalid source: %+v", f.ID, f.Source)
		}
	}

	// Heading lines in the fixture: "## User Authentication" is line 5,
	// "## Dashboard" is line 8.
	wantLines := map[string]int{"user-authentication": 5, "dashboard": 8}
	for _, f := range productSpec.Features {
		if want, ok := wantLines[f.ID]; ok && f.Source.Line != want {
			t.Errorf("feature %q line = %d, want %d", f.ID, f.Source.Line, want)
		}
	}
}

func TestSpecService_AddFeature(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-spec-add-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewSpecService(repo)

	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:      "spec-1",
		Title:   "Project",
		Version: "0.1.0",
		Features: []spec.Feature{
			{ID: "f1", Title: "Feature 1"},
		},
	}); err != nil {
		t.Fatalf("save spec: %v", err)
	}

	oldWD, _ := os.Getwd()
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	updated, err := service.AddFeature("Feature 2", "Desc 2")
	if err != nil {
		t.Fatalf("AddFeature failed: %v", err)
	}
	if len(updated.Spec.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(updated.Spec.Features))
	}
	content, err := os.ReadFile(filepath.Join(tempDir, "docs", "backlog.md"))
	if err != nil {
		t.Fatalf("read backlog: %v", err)
	}
	if !strings.Contains(string(content), "Feature 2") {
		t.Fatalf("expected backlog to include feature, got %q", string(content))
	}
}

// Improving the slugifier must not silently re-id features that already
// exist. A task's feature_id points at the old id, so a re-analysis that
// renamed every feature would orphan the entire plan — the exact failure
// mode reported in issue #73, arrived at from the other direction.
func TestSpecService_AnalyzePreservesExistingFeatureIDs(t *testing.T) {
	tempDir := t.TempDir()
	repo := storage.NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}

	// A spec written by an older Roady, carrying an id the naive slugifier
	// produced.
	legacyID := "phase-a-—-pilot-finalization-(2-weeks)"
	if err := repo.SaveSpec(&spec.ProductSpec{
		ID:      "analyzed-spec",
		Version: "0.1.0",
		Features: []spec.Feature{{
			ID:    legacyID,
			Title: "Phase A — Pilot Finalization (2 weeks)",
		}},
	}); err != nil {
		t.Fatal(err)
	}

	docs := filepath.Join(tempDir, "docs")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}
	md := "# Product\n\n## Phase A — Pilot Finalization (2 weeks)\nShip it.\n\n## Brand New Feature (v2)\nSomething else.\n"
	if err := os.WriteFile(filepath.Join(docs, "spec.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := application.NewSpecService(repo).AnalyzeDirectory(docs)
	if err != nil {
		t.Fatal(err)
	}

	byTitle := map[string]string{}
	for _, f := range result.Features {
		byTitle[f.Title] = f.ID
	}

	if got := byTitle["Phase A — Pilot Finalization (2 weeks)"]; got != legacyID {
		t.Errorf("existing feature was re-identified: got %q, want the original %q", got, legacyID)
	}

	// A feature Roady has not seen before gets a clean id.
	if got := byTitle["Brand New Feature (v2)"]; got != "brand-new-feature-v2" {
		t.Errorf("new feature id = %q, want %q", got, "brand-new-feature-v2")
	}
}

// The MCP server runs from wherever it was started, which is routinely a
// different repository from the one named by project_path. Resolving the
// backlog document relatively wrote one project's feature text into another
// project's working tree — and reported success. See issue #71.
func TestSpecService_AddFeatureWritesBacklogToProjectNotWorkingDir(t *testing.T) {
	project := t.TempDir()
	elsewhere := t.TempDir()

	// Stand where the server would be standing: an unrelated repository.
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}

	repo := storage.NewFilesystemRepository(project)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSpec(&spec.ProductSpec{ID: "s", Version: "0.1.0"}); err != nil {
		t.Fatal(err)
	}

	result, err := application.NewSpecService(repo).AddFeature("Payment Retries", "Retry failed charges.")
	if err != nil {
		t.Fatalf("AddFeature: %v", err)
	}

	if len(result.Warnings) > 0 {
		t.Fatalf("unexpected warnings: %v", result.Warnings)
	}
	if !result.Synced() {
		t.Fatal("AddFeature reported no backlog sync")
	}

	// The backlog belongs to the project...
	inProject := filepath.Join(project, "docs", "backlog.md")
	body, err := os.ReadFile(inProject) //nolint:gosec // test-controlled path
	if err != nil {
		t.Fatalf("backlog missing from the project: %v", err)
	}
	if !strings.Contains(string(body), "Payment Retries") {
		t.Errorf("backlog does not mention the feature: %q", body)
	}
	if result.BacklogPath != inProject {
		t.Errorf("BacklogPath = %q, want %q", result.BacklogPath, inProject)
	}

	// ...and nothing was created in the directory we happened to stand in.
	if _, err := os.Stat(filepath.Join(elsewhere, "docs")); !os.IsNotExist(err) {
		t.Errorf("AddFeature created docs/ in the working directory %s", elsewhere)
	}
}
