// Package memory is the in-memory Store implementation for the
// ops_error_logs module. Mirrors the layout of the other module memory
// stores in this repo (operations, subscriptions, content_safety): a single
// mutex around a map keyed by an auto-incrementing id, with a sorted slice
// view for paginated reads.
package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
)

type Store struct {
	mu      sync.Mutex
	nextID  int64
	entries map[int64]contract.Entry
}

func New() *Store {
	return &Store{
		nextID:  1,
		entries: map[int64]contract.Entry{},
	}
}

func (s *Store) Insert(_ context.Context, entry contract.Entry) (contract.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.ID = s.nextID
	s.nextID++
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = entry.CreatedAt
	}
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = entry.CreatedAt
	}
	entry.UpstreamErrors = cloneUpstreamErrors(entry.UpstreamErrors)
	s.entries[entry.ID] = entry
	return entry, nil
}

func (s *Store) Get(_ context.Context, id int64) (contract.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[id]
	if !ok {
		return contract.Entry{}, contract.ErrNotFound
	}
	return cloneEntry(entry), nil
}

func (s *Store) List(_ context.Context, filter contract.ListFilter) (contract.ListResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]contract.Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if !matchesFilter(e, filter) {
			continue
		}
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].OccurredAt.Equal(all[j].OccurredAt) {
			return all[i].ID > all[j].ID
		}
		return all[i].OccurredAt.After(all[j].OccurredAt)
	})
	total := len(all)
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	size := filter.PageSize
	if size <= 0 {
		size = 20
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return contract.ListResult{
		Items:    cloneEntries(all[start:end]),
		Total:    total,
		Page:     page,
		PageSize: size,
	}, nil
}

func (s *Store) UpdateResolution(_ context.Context, req contract.UpdateResolutionRequest) (contract.Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[req.ID]
	if !ok {
		return contract.Entry{}, contract.ErrNotFound
	}
	entry.Resolution = req.Resolution
	entry.ResolutionNote = req.Note
	entry.ResolvedByID = req.ResolvedByID
	entry.UpdatedAt = req.At
	if req.Resolution == contract.ResolutionResolved {
		t := req.At
		entry.ResolvedAt = &t
	} else {
		entry.ResolvedAt = nil
		entry.ResolvedByID = nil
	}
	s.entries[req.ID] = entry
	return cloneEntry(entry), nil
}

func (s *Store) DeleteOlderThan(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	for id, entry := range s.entries {
		if entry.OccurredAt.Before(before) {
			delete(s.entries, id)
			removed++
		}
	}
	return removed, nil
}

func matchesFilter(entry contract.Entry, filter contract.ListFilter) bool {
	if filter.UserID != nil {
		if entry.UserID == nil || *entry.UserID != *filter.UserID {
			return false
		}
	}
	if filter.AccountID != nil {
		if entry.AccountID == nil || *entry.AccountID != *filter.AccountID {
			return false
		}
	}
	if filter.ProviderID != nil {
		if entry.ProviderID == nil || *entry.ProviderID != *filter.ProviderID {
			return false
		}
	}
	if requestID := strings.TrimSpace(filter.RequestID); requestID != "" && entry.RequestID != requestID {
		return false
	}
	if filter.Platform != "" && entry.Platform != filter.Platform {
		return false
	}
	if filter.SourceEndpoint != "" && entry.SourceEndpoint != filter.SourceEndpoint {
		return false
	}
	if filter.Model != "" && entry.Model != filter.Model {
		return false
	}
	if filter.ErrorClass != "" && entry.ErrorClass != filter.ErrorClass {
		return false
	}
	if filter.ErrorPhase != "" && entry.ErrorPhase != filter.ErrorPhase {
		return false
	}
	if filter.ErrorOwner != "" && entry.ErrorOwner != filter.ErrorOwner {
		return false
	}
	if query := strings.ToLower(strings.TrimSpace(filter.Query)); query != "" && !entryMatchesQuery(entry, query) {
		return false
	}
	if filter.Resolution != "" && entry.Resolution != filter.Resolution {
		return false
	}
	if filter.StatusCodeMin != nil {
		if entry.StatusCode == nil || *entry.StatusCode < *filter.StatusCodeMin {
			return false
		}
	}
	if filter.StatusCodeMax != nil {
		if entry.StatusCode == nil || *entry.StatusCode > *filter.StatusCodeMax {
			return false
		}
	}
	if filter.From != nil && entry.OccurredAt.Before(*filter.From) {
		return false
	}
	if filter.To != nil && entry.OccurredAt.After(*filter.To) {
		return false
	}
	return true
}

func entryMatchesQuery(entry contract.Entry, query string) bool {
	fields := []string{
		entry.RequestID,
		entry.TraceID,
		entry.APIKeyPrefix,
		entry.SourceEndpoint,
		entry.TargetProtocol,
		entry.Model,
		entry.UpstreamRequestID,
		entry.ErrorClass,
		entry.ErrorPhase,
		entry.ErrorOwner,
		entry.ErrorSource,
		entry.ErrorMessage,
		entry.ErrorBodyExcerpt,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func cloneEntries(entries []contract.Entry) []contract.Entry {
	out := make([]contract.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, cloneEntry(entry))
	}
	return out
}

func cloneEntry(entry contract.Entry) contract.Entry {
	entry.UpstreamErrors = cloneUpstreamErrors(entry.UpstreamErrors)
	return entry
}

func cloneUpstreamErrors(events []contract.UpstreamErrorEvent) []contract.UpstreamErrorEvent {
	if len(events) == 0 {
		return nil
	}
	out := make([]contract.UpstreamErrorEvent, 0, len(events))
	for _, event := range events {
		if event.AccountID != nil {
			id := *event.AccountID
			event.AccountID = &id
		}
		out = append(out, event)
	}
	return out
}
