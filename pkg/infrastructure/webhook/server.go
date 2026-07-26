// Package webhook provides HTTP webhook server for receiving events from external systems.
package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/felixgeelhaar/roady/pkg/domain/planning"
)

// Event represents a webhook event from an external system.
type Event struct {
	Provider   string                 `json:"provider"`
	EventType  string                 `json:"event_type"`
	ExternalID string                 `json:"external_id"`
	TaskID     string                 `json:"task_id"`
	Status     planning.TaskStatus    `json:"status,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}

// Handler processes webhook events from a specific provider.
type Handler interface {
	// Provider returns the name of the provider (e.g., "github", "jira", "linear").
	Provider() string

	// ValidateSignature validates the webhook signature to ensure authenticity.
	ValidateSignature(r *http.Request, secret string) bool

	// ParseEvent parses the HTTP request into a webhook Event.
	ParseEvent(r *http.Request) (*Event, error)
}

// EventProcessor handles incoming webhook events.
type EventProcessor interface {
	ProcessEvent(ctx context.Context, event *Event) error
}

// Server is the HTTP webhook server.
type Server struct {
	addr      string
	handlers  map[string]Handler
	secrets   map[string]string // provider -> secret
	processor EventProcessor
	server    *http.Server
	mu        sync.RWMutex
	events    []Event // Recent events for debugging
}

// NewServer creates a new webhook server.
func NewServer(addr string, processor EventProcessor) *Server {
	return &Server{
		addr:      addr,
		handlers:  make(map[string]Handler),
		secrets:   make(map[string]string),
		processor: processor,
		events:    make([]Event, 0, 100),
	}
}

// RegisterHandler adds a webhook handler for a specific provider.
func (s *Server) RegisterHandler(handler Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[handler.Provider()] = handler
}

// SetSecret sets the webhook secret for a provider.
func (s *Server) SetSecret(provider, secret string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[provider] = secret
}

// Start starts the webhook server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register webhook endpoints for each provider
	mux.HandleFunc("/webhooks/github", s.handleWebhook("github"))
	mux.HandleFunc("/webhooks/jira", s.handleWebhook("jira"))
	mux.HandleFunc("/webhooks/linear", s.handleWebhook("linear"))

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Recent events endpoint (for debugging)
	mux.HandleFunc("/events", s.handleEvents)

	srv := &http.Server{
		Addr:         s.addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}
	// Publish under the lock, then serve through the local copy. ListenAndServe
	// blocks for the life of the server, so every caller runs Start in its own
	// goroutine and Shutdown from another — an unguarded s.server is read and
	// written concurrently by definition, not just under test.
	s.mu.Lock()
	s.server = srv
	s.mu.Unlock()

	log.Printf("Webhook server starting on %s", s.addr)
	return srv.ListenAndServe()
}

// Shutdown gracefully shuts down the server. Shutting down a server that was
// never started is a no-op, so racing Shutdown ahead of Start returns nil
// rather than blocking.
func (s *Server) Shutdown(ctx context.Context) error {
	s.mu.RLock()
	srv := s.server
	s.mu.RUnlock()
	if srv == nil {
		return nil
	}
	// Shutdown outside the lock: it blocks until connections drain, and holding
	// the mutex would stall every in-flight handler that needs it.
	return srv.Shutdown(ctx)
}

func (s *Server) handleWebhook(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		s.mu.RLock()
		handler, ok := s.handlers[provider]
		secret := s.secrets[provider]
		s.mu.RUnlock()

		if !ok {
			http.Error(w, fmt.Sprintf("No handler registered for %s", provider), http.StatusNotFound)
			return
		}

		// Validate signature if secret is configured
		if secret != "" && !handler.ValidateSignature(r, secret) {
			log.Printf("Invalid signature for %s webhook", provider)
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}

		// Parse the event
		event, err := handler.ParseEvent(r)
		if err != nil {
			log.Printf("Failed to parse %s webhook: %v", provider, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Store event for debugging
		s.storeEvent(event)

		// Process the event
		if s.processor != nil {
			if err := s.processor.ProcessEvent(r.Context(), event); err != nil {
				log.Printf("Failed to process %s event: %v", provider, err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		log.Printf("Processed %s webhook: type=%s task=%s", provider, event.EventType, event.TaskID)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	events := make([]Event, len(s.events))
	copy(events, s.events)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

func (s *Server) storeEvent(event *Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Keep only last 100 events
	if len(s.events) >= 100 {
		s.events = s.events[1:]
	}
	s.events = append(s.events, *event)
}

// RecentEvents returns recent webhook events (for debugging).
func (s *Server) RecentEvents() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	events := make([]Event, len(s.events))
	copy(events, s.events)
	return events
}
