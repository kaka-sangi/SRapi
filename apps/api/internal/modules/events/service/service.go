package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type OutboxHandler interface {
	HandleOutboxEvent(ctx context.Context, event contract.OutboxEvent) error
}

type OutboxHandlerFunc func(context.Context, contract.OutboxEvent) error

func (fn OutboxHandlerFunc) HandleOutboxEvent(ctx context.Context, event contract.OutboxEvent) error {
	return fn(ctx, event)
}

type DispatchOptions struct {
	Limit        int
	RetryBackoff time.Duration
}

type DispatchResult struct {
	Selected  int
	Published int
	Failed    int
}

type Service struct {
	store contract.Store
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Enqueue(ctx context.Context, req contract.EnqueueRequest) (contract.OutboxEvent, error) {
	eventType := strings.TrimSpace(req.EventType)
	producerModule := strings.TrimSpace(req.ProducerModule)
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if eventType == "" || producerModule == "" || idempotencyKey == "" {
		return contract.OutboxEvent{}, ErrInvalidInput
	}
	eventVersion := strings.TrimSpace(req.EventVersion)
	if eventVersion == "" {
		eventVersion = "v1"
	}
	return s.store.CreateOutbox(ctx, contract.OutboxEvent{
		EventID:        newEventID(),
		EventType:      eventType,
		EventVersion:   eventVersion,
		ProducerModule: producerModule,
		AggregateType:  strings.TrimSpace(req.AggregateType),
		AggregateID:    strings.TrimSpace(req.AggregateID),
		CorrelationID:  strings.TrimSpace(req.CorrelationID),
		CausationID:    strings.TrimSpace(req.CausationID),
		IdempotencyKey: idempotencyKey,
		Payload:        cloneMap(req.Payload),
		Metadata:       cloneMap(req.Metadata),
		Status:         contract.OutboxStatusPending,
		CreatedAt:      s.clock.Now(),
	})
}

func (s *Service) ListOutbox(ctx context.Context) ([]contract.OutboxEvent, error) {
	return s.store.ListOutbox(ctx)
}

// ListOutboxPage delegates to OutboxPageReader when supported so admin/outbox
// reads do not load the entire table. Falls back to ListOutbox + in-memory
// filter+slice when the store omits the capability.
func (s *Service) ListOutboxPage(ctx context.Context, filter contract.OutboxListFilter, limit, offset int) (contract.OutboxListPageResult, error) {
	if reader, ok := s.store.(contract.OutboxPageReader); ok {
		return reader.ListOutboxPage(ctx, filter, limit, offset)
	}
	all, err := s.store.ListOutbox(ctx)
	if err != nil {
		return contract.OutboxListPageResult{}, err
	}
	wantStatus := strings.TrimSpace(string(filter.Status))
	wantType := strings.TrimSpace(filter.EventType)
	matched := make([]contract.OutboxEvent, 0, len(all))
	for _, event := range all {
		if wantStatus != "" && string(event.Status) != wantStatus {
			continue
		}
		if wantType != "" && event.EventType != wantType {
			continue
		}
		matched = append(matched, event)
	}
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
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

func (s *Service) DispatchPending(ctx context.Context, handler OutboxHandler, options DispatchOptions) (DispatchResult, error) {
	if handler == nil {
		return DispatchResult{}, ErrInvalidInput
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 100
	}
	backoff := options.RetryBackoff
	if backoff <= 0 {
		backoff = 30 * time.Second
	}
	now := s.clock.Now()
	events, err := s.store.ListDispatchableOutbox(ctx, now, limit)
	if err != nil {
		return DispatchResult{}, err
	}
	result := DispatchResult{Selected: len(events)}
	for _, event := range events {
		if err := handler.HandleOutboxEvent(ctx, event); err != nil {
			if errors.Is(err, contract.ErrInboxClaimed) {
				continue
			}
			nextRetryAt := now.Add(retryDelay(backoff, event.AttemptCount+1))
			if _, markErr := s.store.MarkOutboxFailed(ctx, event.ID, event.AttemptCount+1, &nextRetryAt, truncateError(err.Error())); markErr != nil {
				return result, markErr
			}
			result.Failed++
			continue
		}
		if _, err := s.store.MarkOutboxPublished(ctx, event.ID, now); err != nil {
			if errors.Is(err, contract.ErrNotDispatchable) {
				continue
			}
			return result, err
		}
		result.Published++
	}
	return result, nil
}

func (s *Service) RecordInbox(ctx context.Context, req contract.RecordInboxRequest) (contract.InboxRecord, bool, error) {
	eventID := strings.TrimSpace(req.EventID)
	consumerName := strings.TrimSpace(req.ConsumerName)
	eventType := strings.TrimSpace(req.EventType)
	if eventID == "" || consumerName == "" || eventType == "" {
		return contract.InboxRecord{}, false, ErrInvalidInput
	}
	return s.store.CreateInbox(ctx, contract.InboxRecord{
		EventID:      eventID,
		ConsumerName: consumerName,
		EventType:    eventType,
		Status:       contract.InboxStatusPending,
		CreatedAt:    s.clock.Now(),
	})
}

func (s *Service) MarkInboxProcessed(ctx context.Context, id int) (contract.InboxRecord, error) {
	if id <= 0 {
		return contract.InboxRecord{}, ErrInvalidInput
	}
	return s.store.MarkInboxProcessed(ctx, id, s.clock.Now())
}

func (s *Service) MarkInboxFailed(ctx context.Context, record contract.InboxRecord, err error) (contract.InboxRecord, error) {
	if record.ID <= 0 || err == nil {
		return contract.InboxRecord{}, ErrInvalidInput
	}
	return s.store.MarkInboxFailed(ctx, record.ID, record.AttemptCount+1, truncateError(err.Error()))
}

func (s *Service) ListInbox(ctx context.Context) ([]contract.InboxRecord, error) {
	return s.store.ListInbox(ctx)
}

func newEventID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	return "evt_" + hex.EncodeToString(bytes[:])
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

func retryDelay(base time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return base * time.Duration(1<<(attempt-1))
}

func truncateError(value string) string {
	const maxLength = 1024
	value = strings.TrimSpace(value)
	if len(value) <= maxLength {
		return value
	}
	return value[:maxLength]
}
