package storage

import (
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/domain/planning"
	"github.com/felixgeelhaar/roady/pkg/domain/spec"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesystemRepository_ResolvePath_Edge(t *testing.T) {
	repo := NewFilesystemRepository("/tmp")

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Empty", "", true},
		{"Dot", ".", true},
		{"Parent", "..", true},
		{"Subdir", "sub/file", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.ResolvePath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolvePath(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestFilesystemRepository_InitError(t *testing.T) {
	tempFile, _ := os.CreateTemp("", "roady-init-fail-*")
	defer func() { _ = os.Remove(tempFile.Name()) }()

	repo := NewFilesystemRepository(tempFile.Name())
	if err := repo.Initialize(); err == nil {
		t.Error("expected init error when root is a file")
	}
}

func TestFilesystemRepository_Thorough(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-storage-thorough-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := NewFilesystemRepository(tempDir)

	// 1. Init
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}
	if !repo.IsInitialized() {
		t.Error("Expected initialized")
	}

	// 2. Spec Save/Load
	s := &spec.ProductSpec{ID: "s1", Title: "T1", Features: []spec.Feature{{ID: "f1"}}}
	if err := repo.SaveSpec(s); err != nil {
		t.Fatal(err)
	}
	loadedSpec, err := repo.LoadSpec()
	if err != nil {
		t.Fatal(err)
	}
	if loadedSpec.ID != "s1" {
		t.Errorf("Expected s1, got %s", loadedSpec.ID)
	}

	// 3. Spec Lock
	if err := repo.SaveSpecLock(s); err != nil {
		t.Fatal(err)
	}
	loadedLock, err := repo.LoadSpecLock()
	if err != nil {
		t.Fatal(err)
	}
	if loadedLock.ID != "s1" {
		t.Error("LoadSpecLock failed")
	}

	// 4. Plan Save/Load
	p := &planning.Plan{ID: "p1", Tasks: []planning.Task{{ID: "t1"}}}
	if err := repo.SavePlan(p); err != nil {
		t.Fatal(err)
	}
	loadedPlan, err := repo.LoadPlan()
	if err != nil {
		t.Fatal(err)
	}
	if loadedPlan.ID != p.ID {
		t.Errorf("Expected p1, got %s", loadedPlan.ID)
	}

	// 4.1 Plan Load Error (missing dir)
	repoMissing := NewFilesystemRepository("/nonexistent/path/999")
	_, err = repoMissing.LoadPlan()
	if err == nil {
		t.Error("expected error for missing plan directory")
	}

	// 5. Policy Save/Load
	pol := &domain.PolicyConfig{MaxWIP: 99}
	if err := repo.SavePolicy(pol); err != nil {
		t.Fatal(err)
	}
	loadedPol, err := repo.LoadPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if loadedPol.MaxWIP != 99 {
		t.Errorf("Expected 99, got %d", loadedPol.MaxWIP)
	}

	// 6. Usage Update

	// 7. Events Record
	ev := domain.Event{ID: "e1", Action: "act"}
	if err := repo.RecordEvent(ev); err != nil {
		t.Fatal(err)
	}

	// 7.1 RecordEvent marshalling fail
	err = repo.RecordEvent(domain.Event{
		Metadata: map[string]interface{}{"fail": func() {}},
	})
	if err == nil {
		t.Error("expected marshal error for function in metadata")
	}

	// 8. ResolvePath Traversal
	_, err = repo.ResolvePath("../../etc/passwd")
	if err == nil {
		t.Error("Expected error for traversal")
	}

	// 8.1 ResolvePath rejects arbitrary nested paths on a root-project repo.
	// Sub-project files are addressed by constructing a sub-project repository
	// (NewFilesystemRepositoryForProject), not by passing a nested filename here.
	_, err = repo.ResolvePath("sub/file.yaml")
	if err == nil {
		t.Error("expected error for nested path on root-project repo")
	}

	validPath, _ := repo.ResolvePath("spec.yaml")
	if !strings.Contains(validPath, ".roady/spec.yaml") {
		t.Errorf("Unexpected path: %s", validPath)
	}

	// 9. LoadPolicy default
	tempEmpty, _ := os.MkdirTemp("", "roady-empty-*")
	defer func() { _ = os.RemoveAll(tempEmpty) }()
	repoEmpty := NewFilesystemRepository(tempEmpty)
	if err := repoEmpty.Initialize(); err != nil {
		t.Fatal(err)
	}
	pPol, _ := repoEmpty.LoadPolicy()
	if pPol.MaxWIP != 3 {
		t.Errorf("expected default 3, got %d", pPol.MaxWIP)
	}

	// 10. Invalid JSON in plan
	_ = os.WriteFile(filepath.Join(repoEmpty.root, ".roady", "plan.json"), []byte("invalid json"), 0600)
	if _, err := repoEmpty.LoadPlan(); err == nil {
		t.Error("expected json error in LoadPlan")
	}

	// 11. Invalid YAML in policy (syntax error)
	_ = os.WriteFile(filepath.Join(repoEmpty.root, ".roady", "policy.yaml"), []byte("[}"), 0600)
	if _, err := repoEmpty.LoadPolicy(); err == nil {
		t.Error("expected yaml syntax error")
	}

	// 12. Invalid YAML in spec
	_ = os.WriteFile(filepath.Join(repoEmpty.root, ".roady", "spec.yaml"), []byte("[}"), 0600)
	if _, err := repoEmpty.LoadSpec(); err == nil {
		t.Error("expected yaml error in LoadSpec")
	}

	// 13. LoadPolicy type mismatch
	_ = os.WriteFile(filepath.Join(repoEmpty.root, ".roady", "policy.yaml"), []byte("123"), 0600)
	if _, err := repoEmpty.LoadPolicy(); err == nil {
		t.Error("expected yaml error for integer policy")
	}

	// 14. Read failure (is a directory)
	_ = os.Remove(repoEmpty.root + "/.roady/spec.yaml")
	if err := os.Mkdir(repoEmpty.root+"/.roady/spec.yaml", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := repoEmpty.LoadSpec(); err == nil {
		t.Error("expected read error for directory (spec)")
	}

	if err := os.Mkdir(repoEmpty.root+"/.roady/policy.isadir", 0700); err != nil {
		t.Fatal(err)
	}
	// We can't rename because PolicyFile is a constant "policy.yaml"
	// I'll just remove the file and create a dir with same name
	_ = os.Remove(repoEmpty.root + "/.roady/policy.yaml")
	if err := os.Mkdir(repoEmpty.root+"/.roady/policy.yaml", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := repoEmpty.LoadPolicy(); err == nil {
		t.Error("expected read error for directory (policy)")
	}
}

func TestFilesystemRepository_Errors(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-readonly-*")
	defer func() { _ = os.RemoveAll(tempDir) }()
	repo := NewFilesystemRepository(tempDir)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}

	// Make .roady read-only to force WriteFile failure
	if err := os.Chmod(filepath.Join(repo.root, ".roady"), 0400); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chmod(filepath.Join(repo.root, ".roady"), 0700); err != nil {
			t.Fatal(err)
		}
	}()

	if err := repo.SaveSpec(&spec.ProductSpec{ID: "fail"}); err == nil {
		t.Error("expected write error on readonly dir (spec)")
	}
	if err := repo.RecordEvent(domain.Event{ID: "fail"}); err == nil {
		t.Error("expected write error on readonly dir (event)")
	}
	if err := repo.SavePlan(&planning.Plan{ID: "fail"}); err == nil {
		t.Error("expected write error on readonly dir (plan)")
	}
	if err := repo.SavePolicy(&domain.PolicyConfig{}); err == nil {
		t.Error("expected write error on readonly dir (policy)")
	}
	if err := repo.SaveSpecLock(&spec.ProductSpec{ID: "fail"}); err == nil {
		t.Error("expected write error on readonly dir (lock)")
	}
}
