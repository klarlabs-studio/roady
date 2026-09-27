package application_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/domain"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

func TestAuditService_Log(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "roady-audit-test-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	repo := storage.NewFilesystemRepository(tempDir)
	_ = repo.Initialize()
	service := application.NewAuditService(repo)

	// 1. Log Event
	if err := service.Log("test.action", "tester", map[string]interface{}{"key": "val"}); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// 2. Verify File
	content, err := os.ReadFile(filepath.Join(tempDir, ".roady", "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "test.action") {
		t.Error("Event not logged")
	}

	// Note: Usage tracking is now handled by UsageService (SRP separation)
	// See usage_service_test.go for usage tracking tests
}

func TestAuditService_Error(t *testing.T) {
	repo := &MockRepo{SaveError: errors.New("audit fail")}
	service := application.NewAuditService(repo)

	if err := service.Log("act", "actor", nil); err == nil {
		t.Error("expected error on save fail")
	}
}

type eventRepo struct {
	*MockRepo
	Events []domain.Event
}

func (r *eventRepo) LoadEvents() ([]domain.Event, error) {
	return r.Events, r.LoadError
}

func TestAuditService_VerifyIntegrity(t *testing.T) {
	now := time.Now()
	first := domain.Event{
		ID:        "e1",
		Timestamp: now.Add(-2 * time.Hour),
		Action:    "spec.update",
		Actor:     "tester",
	}
	first.Hash = first.CalculateHash()

	second := domain.Event{
		ID:        "e2",
		Timestamp: now.Add(-1 * time.Hour),
		Action:    "plan.generate",
		Actor:     "tester",
		PrevHash:  first.Hash,
	}
	second.Hash = second.CalculateHash()

	repo := &eventRepo{
		MockRepo: &MockRepo{},
		Events:   []domain.Event{first, second},
	}
	service := application.NewAuditService(repo)

	violations, err := service.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %d", len(violations))
	}
}

func TestAuditService_VerifyIntegrityMismatch(t *testing.T) {
	now := time.Now()
	first := domain.Event{
		ID:        "e1",
		Timestamp: now.Add(-2 * time.Hour),
		Action:    "spec.update",
		Actor:     "tester",
	}
	first.Hash = first.CalculateHash()

	second := domain.Event{
		ID:        "e2",
		Timestamp: now.Add(-1 * time.Hour),
		Action:    "plan.generate",
		Actor:     "tester",
		PrevHash:  "bad-hash",
	}
	second.Hash = second.CalculateHash()

	repo := &eventRepo{
		MockRepo: &MockRepo{},
		Events:   []domain.Event{first, second},
	}
	service := application.NewAuditService(repo)

	violations, err := service.VerifyIntegrity()
	if err != nil {
		t.Fatalf("VerifyIntegrity failed: %v", err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violations for broken hash chain")
	}
}

func TestAuditService_GetTimeline(t *testing.T) {
	events := []domain.Event{
		{ID: "e1", Action: "spec.update"},
		{ID: "e2", Action: "plan.generate"},
	}
	repo := &eventRepo{
		MockRepo: &MockRepo{},
		Events:   events,
	}
	service := application.NewAuditService(repo)
	timeline, err := service.GetTimeline()
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(timeline) != len(events) {
		t.Fatalf("expected %d events in timeline, got %d", len(events), len(timeline))
	}
}
