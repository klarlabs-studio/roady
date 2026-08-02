package application_test

import (
	"testing"

	"github.com/felixgeelhaar/roady/pkg/application"
	"github.com/felixgeelhaar/roady/pkg/storage"
)

// Two services write to events.jsonl and both answer "is this log intact?".
// They gave different answers: AuditService verified it as a hash-linked
// graph, EventSourcedAuditService required each entry to follow the previous
// line and so reported tampering for the branch shape concurrent appends
// legitimately produce. On the same file at the same moment the CLI said
// "intact and verified" while MCP reported a violation.
//
// An audit chain whose verdict depends on which code path asked is not
// evidence of anything, so the agreement is pinned here rather than assumed.
func TestBothAuditServicesAgreeOnTheSameLog(t *testing.T) {
	root := t.TempDir()
	repo := storage.NewFilesystemRepository(root)
	if err := repo.Initialize(); err != nil {
		t.Fatal(err)
	}

	// Write through the event-sourced service, which is what the running
	// system does, so the log has the shape production actually produces.
	store, err := storage.NewFileEventStore(repo.ProjectBase())
	if err != nil {
		t.Fatal(err)
	}
	es, err := application.NewEventSourcedAuditService(store, storage.NewInMemoryEventPublisher())
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"plan.generate", "task.start", "task.complete"} {
		if lErr := es.Log(action, "test", map[string]any{"n": action}); lErr != nil {
			t.Fatalf("log %s: %v", action, lErr)
		}
	}

	plain := application.NewAuditService(repo)

	plainViolations, err := plain.VerifyIntegrity()
	if err != nil {
		t.Fatalf("AuditService.VerifyIntegrity: %v", err)
	}
	esViolations, err := es.VerifyIntegrity()
	if err != nil {
		t.Fatalf("EventSourcedAuditService.VerifyIntegrity: %v", err)
	}

	if len(plainViolations) != len(esViolations) {
		t.Errorf("the two verifiers disagree: AuditService reports %d violation(s) %v, EventSourcedAuditService reports %d %v",
			len(plainViolations), plainViolations, len(esViolations), esViolations)
	}
}
