package memory

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
)

type Store struct {
	mu                  sync.Mutex
	nextOutboxID        int
	nextInboxID         int
	outboxByID          map[int]contract.OutboxEvent
	outboxByEventID     map[string]int
	outboxByIdempotency map[string]int
	inboxByID           map[int]contract.InboxRecord
	inboxByKey          map[string]int
}

func New() *Store {
	return &Store{
		nextOutboxID:        1,
		nextInboxID:         1,
		outboxByID:          map[int]contract.OutboxEvent{},
		outboxByEventID:     map[string]int{},
		outboxByIdempotency: map[string]int{},
		inboxByID:           map[int]contract.InboxRecord{},
		inboxByKey:          map[string]int{},
	}
}

func (s *Store) CreateOutbox(_ context.Context, input contract.OutboxEvent) (contract.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.outboxByEventID[input.EventID]; ok {
		return contract.OutboxEvent{}, errors.New("outbox event already exists")
	}
	key := input.ProducerModule + ":" + input.IdempotencyKey
	if id, ok := s.outboxByIdempotency[key]; ok {
		return cloneOutbox(s.outboxByID[id]), nil
	}
	event := cloneOutbox(input)
	event.ID = s.nextOutboxID
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	s.outboxByID[event.ID] = event
	s.outboxByEventID[event.EventID] = event.ID
	s.outboxByIdempotency[key] = event.ID
	s.nextOutboxID++
	return cloneOutbox(event), nil
}

func (s *Store) ListOutbox(_ context.Context) ([]contract.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.OutboxEvent, 0, len(s.outboxByID))
	for _, event := range s.outboxByID {
		out = append(out, cloneOutbox(event))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListOutboxPage mirrors the SQL store with newest-first ordering and offset/
// limit slicing — supports the OutboxPageReader capability so memory-store
// tests exercise the same shape as the production ent store.
func (s *Store) ListOutboxPage(_ context.Context, filter contract.OutboxListFilter, limit, offset int) (contract.OutboxListPageResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wantStatus := strings.TrimSpace(string(filter.Status))
	wantType := strings.TrimSpace(filter.EventType)
	matched := make([]contract.OutboxEvent, 0)
	for _, event := range s.outboxByID {
		if wantStatus != "" && string(event.Status) != wantStatus {
			continue
		}
		if wantType != "" && event.EventType != wantType {
			continue
		}
		matched = append(matched, cloneOutbox(event))
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID > matched[j].ID })
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return contract.OutboxListPageResult{Items: []contract.OutboxEvent{}, Total: total}, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return contract.OutboxListPageResult{Items: matched[offset:end], Total: total}, nil
}

func (s *Store) ListDispatchableOutbox(_ context.Context, now time.Time, limit int) ([]contract.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.OutboxEvent, 0, len(s.outboxByID))
	for _, event := range s.outboxByID {
		if !isDispatchable(event, now) {
			continue
		}
		out = append(out, cloneOutbox(event))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) MarkOutboxPublished(_ context.Context, id int, publishedAt time.Time) (contract.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event, ok := s.outboxByID[id]
	if !ok {
		return contract.OutboxEvent{}, errors.New("outbox event not found")
	}
	if event.Status != contract.OutboxStatusPending && event.Status != contract.OutboxStatusFailed {
		return contract.OutboxEvent{}, contract.ErrNotDispatchable
	}
	event.Status = contract.OutboxStatusPublished
	event.PublishedAt = &publishedAt
	event.NextRetryAt = nil
	event.LastError = nil
	s.outboxByID[id] = event
	return cloneOutbox(event), nil
}

func (s *Store) MarkOutboxFailed(_ context.Context, id int, attemptCount int, nextRetryAt *time.Time, lastError string) (contract.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	event, ok := s.outboxByID[id]
	if !ok {
		return contract.OutboxEvent{}, errors.New("outbox event not found")
	}
	event.Status = contract.OutboxStatusFailed
	event.AttemptCount = attemptCount
	event.NextRetryAt = cloneTime(nextRetryAt)
	event.LastError = cloneString(&lastError)
	event.PublishedAt = nil
	s.outboxByID[id] = event
	return cloneOutbox(event), nil
}

func (s *Store) CreateInbox(_ context.Context, input contract.InboxRecord) (contract.InboxRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := input.EventID + ":" + input.ConsumerName
	if id, ok := s.inboxByKey[key]; ok {
		record := s.inboxByID[id]
		if record.Status == contract.InboxStatusFailed {
			record.Status = contract.InboxStatusPending
			record.LastError = nil
			record.ProcessedAt = nil
			s.inboxByID[id] = record
			return cloneInbox(record), true, nil
		}
		return cloneInbox(record), false, nil
	}
	record := input
	record.ID = s.nextInboxID
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	s.inboxByID[record.ID] = record
	s.inboxByKey[key] = record.ID
	s.nextInboxID++
	return cloneInbox(record), true, nil
}

func (s *Store) ListInbox(_ context.Context) ([]contract.InboxRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.InboxRecord, 0, len(s.inboxByID))
	for _, record := range s.inboxByID {
		out = append(out, cloneInbox(record))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) MarkInboxProcessed(_ context.Context, id int, processedAt time.Time) (contract.InboxRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.inboxByID[id]
	if !ok {
		return contract.InboxRecord{}, errors.New("inbox record not found")
	}
	record.Status = contract.InboxStatusProcessed
	record.ProcessedAt = cloneTime(&processedAt)
	record.LastError = nil
	s.inboxByID[id] = record
	return cloneInbox(record), nil
}

func (s *Store) MarkInboxFailed(_ context.Context, id int, attemptCount int, lastError string) (contract.InboxRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.inboxByID[id]
	if !ok {
		return contract.InboxRecord{}, errors.New("inbox record not found")
	}
	record.Status = contract.InboxStatusFailed
	record.AttemptCount = attemptCount
	record.LastError = cloneString(&lastError)
	record.ProcessedAt = nil
	s.inboxByID[id] = record
	return cloneInbox(record), nil
}

func cloneOutbox(value contract.OutboxEvent) contract.OutboxEvent {
	value.Payload = cloneMap(value.Payload)
	value.Metadata = cloneMap(value.Metadata)
	value.NextRetryAt = cloneTime(value.NextRetryAt)
	value.LastError = cloneString(value.LastError)
	value.PublishedAt = cloneTime(value.PublishedAt)
	return value
}

func cloneInbox(value contract.InboxRecord) contract.InboxRecord {
	value.LastError = cloneString(value.LastError)
	value.ProcessedAt = cloneTime(value.ProcessedAt)
	return value
}

func isDispatchable(event contract.OutboxEvent, now time.Time) bool {
	if event.Status == contract.OutboxStatusPending {
		return true
	}
	if event.Status != contract.OutboxStatusFailed {
		return false
	}
	return event.NextRetryAt == nil || !event.NextRetryAt.After(now)
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
