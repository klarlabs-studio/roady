package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/felixgeelhaar/roady/pkg/domain"
)

func (r *FilesystemRepository) RecordEvent(event domain.Event) (err error) {
	path, err := r.ResolvePath(EventsFile)
	if err != nil {
		return err
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	data = append(data, '\n')

	// #nosec G304 -- Path is resolved and validated via resolvePath
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("failed to open events file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close events file: %w", cerr)
		}
	}()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}

	return nil
}

// LoadEventsRaw returns every line in the log, including duplicates that a
// union merge may have produced. Only integrity verification wants this;
// everything else reads through LoadEvents, which deduplicates so projections
// cannot double-count.
func (r *FilesystemRepository) LoadEventsRaw() ([]domain.Event, error) {
	return r.loadEvents(false)
}

func (r *FilesystemRepository) LoadEvents() ([]domain.Event, error) {
	return r.loadEvents(true)
}

func (r *FilesystemRepository) loadEvents(dedupe bool) ([]domain.Event, error) {
	path, err := r.ResolvePath(EventsFile)
	if err != nil {
		return nil, err
	}

	// #nosec G304 -- Path is resolved and validated via resolvePath
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.Event{}, nil
		}
		return nil, fmt.Errorf("failed to read events file: %w", err)
	}

	// events.jsonl is append-only and merged with `merge=union` (see
	// .gitattributes), so a merge can reproduce a line that both branches
	// already had. Deduplicating by event ID here keeps projections —
	// velocity, task state, cost — from counting the same event twice.
	// Verification reads through LoadEventsRaw instead, so a duplicate is
	// still reported to a reviewer even though it is harmless to the
	// derived state.
	var events []domain.Event
	seen := map[string]bool{}
	lines := bytes.Split(data, []byte("\n"))
	for _, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var e domain.Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue // Skip malformed lines
		}
		if dedupe && e.ID != "" {
			if seen[e.ID] {
				continue
			}
			seen[e.ID] = true
		}
		events = append(events, e)
	}

	return events, nil
}
