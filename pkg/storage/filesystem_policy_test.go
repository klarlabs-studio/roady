package storage

import (
	"os"
	"testing"
)

// A policy.yaml written for a roady that still had budgets, team roles and a
// token budget keeps loading: the removed keys are ignored, the rest is read.
func TestLoadPolicyIgnoresRemovedKeys(t *testing.T) {
	repo := setupRepo(t)
	path, err := repo.ResolvePath(PolicyFile)
	if err != nil {
		t.Fatal(err)
	}
	old := "max_wip: 4\nallow_ai: true\ntoken_limit: 5000\nbudget_hours: 40\nenforce_team_roles: true\nverify_requires_evidence: true\n"
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.LoadPolicy()
	if err != nil {
		t.Fatalf("an old policy must still load: %v", err)
	}
	if cfg.MaxWIP != 4 || !cfg.AllowAI || !cfg.VerifyRequiresEvidence {
		t.Errorf("kept settings were lost: %+v", cfg)
	}

	// Unknown keys that were never valid are still refused.
	if err := os.WriteFile(path, []byte("max_wip: 4\nmax_wipp: 3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadPolicy(); err == nil {
		t.Error("a misspelt key must still be an error")
	}
}
