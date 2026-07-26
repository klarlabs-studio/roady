package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/events"
)

func TestNotifier_DeliverySuccess(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ep := events.WebhookEndpoint{
		Name:    "test",
		URL:     server.URL,
		Enabled: true,
	}

	n := NewNotifier([]events.WebhookEndpoint{ep}, nil)
	event := &events.BaseEvent{Type: "task.started", Timestamp: time.Now()}
	n.Notify(context.Background(), event)

	time.Sleep(200 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 delivery, got %d", received.Load())
	}
}

func TestNotifier_HMACSignature(t *testing.T) {
	secret := "test-secret"
	// The handler runs on the server's goroutine and the assertions below run on
	// the test's, so everything crossing that boundary needs a lock.
	var mu sync.Mutex
	var receivedSig string
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig := r.Header.Get("X-Roady-Signature")
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedSig, receivedBody = sig, body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ep := events.WebhookEndpoint{
		Name:    "test",
		URL:     server.URL,
		Secret:  secret,
		Enabled: true,
	}

	n := NewNotifier([]events.WebhookEndpoint{ep}, nil)
	event := &events.BaseEvent{Type: "task.completed", Timestamp: time.Now()}
	n.Notify(context.Background(), event)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	gotSig, gotBody := receivedSig, receivedBody
	mu.Unlock()

	if gotSig == "" {
		t.Fatal("expected X-Roady-Signature header")
	}

	// Verify signature
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if gotSig != expected {
		t.Errorf("signature mismatch: got %s, want %s", gotSig, expected)
	}
}

func TestNotifier_RetryAndDeadLetter(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	dlPath := filepath.Join(t.TempDir(), "deadletters.jsonl")
	dlStore := NewDeadLetterStore(dlPath)

	ep := events.WebhookEndpoint{
		Name:       "test",
		URL:        server.URL,
		Enabled:    true,
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	}

	n := NewNotifier([]events.WebhookEndpoint{ep}, dlStore)
	event := &events.BaseEvent{Type: "task.blocked", Timestamp: time.Now()}
	n.Notify(context.Background(), event)

	time.Sleep(500 * time.Millisecond)

	if attempts.Load() != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts.Load())
	}

	entries, err := dlStore.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 dead letter, got %d", len(entries))
	}
}

func TestNotifier_EventFilter(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ep := events.WebhookEndpoint{
		Name:         "test",
		URL:          server.URL,
		Enabled:      true,
		EventFilters: []string{"task.started"},
	}

	n := NewNotifier([]events.WebhookEndpoint{ep}, nil)

	// This should NOT be delivered (filtered out)
	event := &events.BaseEvent{Type: "task.completed", Timestamp: time.Now()}
	n.Notify(context.Background(), event)

	time.Sleep(200 * time.Millisecond)

	if received.Load() != 0 {
		t.Errorf("expected 0 deliveries for filtered event, got %d", received.Load())
	}

	// This should be delivered
	event2 := &events.BaseEvent{Type: "task.started", Timestamp: time.Now()}
	n.Notify(context.Background(), event2)

	time.Sleep(200 * time.Millisecond)

	if received.Load() != 1 {
		t.Errorf("expected 1 delivery for matching event, got %d", received.Load())
	}
}

func TestPayloadFormat(t *testing.T) {
	// Guarded for the same reason, and the decode error is carried back to the
	// test goroutine rather than reported with t.Fatal from the handler's —
	// t.Fatal must be called from the goroutine running the test.
	var mu sync.Mutex
	var receivedPayload Payload
	var decodeErr error

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p Payload
		err := json.Unmarshal(body, &p)
		mu.Lock()
		receivedPayload, decodeErr = p, err
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ep := events.WebhookEndpoint{Name: "test", URL: server.URL, Enabled: true}
	n := NewNotifier([]events.WebhookEndpoint{ep}, nil)
	event := &events.BaseEvent{Type: "plan.created", Timestamp: time.Now()}
	n.Notify(context.Background(), event)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	got, err := receivedPayload, decodeErr
	mu.Unlock()

	if err != nil {
		t.Fatal(err)
	}
	if got.EventType != "plan.created" {
		t.Errorf("expected event_type plan.created, got %s", got.EventType)
	}
}
