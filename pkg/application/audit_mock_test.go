package application_test

// mockAuditLogger records audit events for test assertions.
type mockAuditLogger struct {
	Events []auditEvent
}

type auditEvent struct {
	Action   string
	Actor    string
	Metadata map[string]interface{}
}

func (m *mockAuditLogger) Log(action string, actor string, metadata map[string]interface{}) error {
	m.Events = append(m.Events, auditEvent{Action: action, Actor: actor, Metadata: metadata})
	return nil
}

func newTestAudit() *mockAuditLogger {
	return &mockAuditLogger{}
}
