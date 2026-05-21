package events

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entdomaineventsinbox "github.com/srapi/srapi/apps/api/ent/domaineventsinbox"
	entdomaineventsoutbox "github.com/srapi/srapi/apps/api/ent/domaineventsoutbox"
	"github.com/srapi/srapi/apps/api/internal/modules/events/contract"
)

var ErrInvalidStore = errors.New("invalid events ent store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) CreateOutbox(ctx context.Context, input contract.OutboxEvent) (contract.OutboxEvent, error) {
	if existing, err := s.findOutboxByIdempotency(ctx, input.ProducerModule, input.IdempotencyKey); err == nil {
		return existing, nil
	} else if !ent.IsNotFound(err) {
		return contract.OutboxEvent{}, err
	}

	create := s.client.DomainEventsOutbox.Create().
		SetEventID(input.EventID).
		SetEventType(input.EventType).
		SetEventVersion(input.EventVersion).
		SetProducerModule(input.ProducerModule).
		SetAggregateType(input.AggregateType).
		SetAggregateID(input.AggregateID).
		SetCorrelationID(input.CorrelationID).
		SetCausationID(input.CausationID).
		SetIdempotencyKey(input.IdempotencyKey).
		SetPayloadJSON(cloneMap(input.Payload)).
		SetMetadataJSON(cloneMap(input.Metadata)).
		SetStatus(string(input.Status)).
		SetAttemptCount(input.AttemptCount).
		SetNillableNextRetryAt(input.NextRetryAt).
		SetNillableLastError(input.LastError).
		SetNillablePublishedAt(input.PublishedAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findOutboxByIdempotency(ctx, input.ProducerModule, input.IdempotencyKey); findErr == nil {
				return existing, nil
			}
		}
		return contract.OutboxEvent{}, err
	}
	return toOutbox(created), nil
}

func (s *Store) ListOutbox(ctx context.Context) ([]contract.OutboxEvent, error) {
	rows, err := s.client.DomainEventsOutbox.Query().
		Order(entdomaineventsoutbox.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOutbox(row))
	}
	return out, nil
}

func (s *Store) ListDispatchableOutbox(ctx context.Context, now time.Time, limit int) ([]contract.OutboxEvent, error) {
	query := s.client.DomainEventsOutbox.Query().
		Where(
			entdomaineventsoutbox.Or(
				entdomaineventsoutbox.StatusEQ(string(contract.OutboxStatusPending)),
				entdomaineventsoutbox.And(
					entdomaineventsoutbox.StatusEQ(string(contract.OutboxStatusFailed)),
					entdomaineventsoutbox.Or(
						entdomaineventsoutbox.NextRetryAtIsNil(),
						entdomaineventsoutbox.NextRetryAtLTE(now),
					),
				),
			),
		).
		Order(entdomaineventsoutbox.ByID())
	if limit > 0 {
		query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		out = append(out, toOutbox(row))
	}
	return out, nil
}

func (s *Store) MarkOutboxPublished(ctx context.Context, id int, publishedAt time.Time) (contract.OutboxEvent, error) {
	row, err := s.client.DomainEventsOutbox.UpdateOneID(id).
		SetStatus(string(contract.OutboxStatusPublished)).
		SetPublishedAt(publishedAt).
		ClearNextRetryAt().
		ClearLastError().
		Save(ctx)
	if err != nil {
		return contract.OutboxEvent{}, err
	}
	return toOutbox(row), nil
}

func (s *Store) MarkOutboxFailed(ctx context.Context, id int, attemptCount int, nextRetryAt *time.Time, lastError string) (contract.OutboxEvent, error) {
	update := s.client.DomainEventsOutbox.UpdateOneID(id).
		SetStatus(string(contract.OutboxStatusFailed)).
		SetAttemptCount(attemptCount).
		SetLastError(lastError).
		ClearPublishedAt()
	if nextRetryAt == nil {
		update.ClearNextRetryAt()
	} else {
		update.SetNextRetryAt(*nextRetryAt)
	}
	row, err := update.Save(ctx)
	if err != nil {
		return contract.OutboxEvent{}, err
	}
	return toOutbox(row), nil
}

func (s *Store) CreateInbox(ctx context.Context, input contract.InboxRecord) (contract.InboxRecord, bool, error) {
	if existing, err := s.findInbox(ctx, input.EventID, input.ConsumerName); err == nil {
		return existing, false, nil
	} else if !ent.IsNotFound(err) {
		return contract.InboxRecord{}, false, err
	}

	create := s.client.DomainEventsInbox.Create().
		SetEventID(input.EventID).
		SetConsumerName(input.ConsumerName).
		SetEventType(input.EventType).
		SetStatus(string(input.Status)).
		SetAttemptCount(input.AttemptCount).
		SetNillableLastError(input.LastError).
		SetNillableProcessedAt(input.ProcessedAt)
	if !input.CreatedAt.IsZero() {
		create.SetCreatedAt(input.CreatedAt).SetUpdatedAt(input.CreatedAt)
	}
	created, err := create.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			if existing, findErr := s.findInbox(ctx, input.EventID, input.ConsumerName); findErr == nil {
				return existing, false, nil
			}
		}
		return contract.InboxRecord{}, false, err
	}
	return toInbox(created), true, nil
}

func (s *Store) ListInbox(ctx context.Context) ([]contract.InboxRecord, error) {
	rows, err := s.client.DomainEventsInbox.Query().
		Order(entdomaineventsinbox.ByID()).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.InboxRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, toInbox(row))
	}
	return out, nil
}

func (s *Store) MarkInboxProcessed(ctx context.Context, id int, processedAt time.Time) (contract.InboxRecord, error) {
	row, err := s.client.DomainEventsInbox.UpdateOneID(id).
		SetStatus(string(contract.InboxStatusProcessed)).
		SetProcessedAt(processedAt).
		ClearLastError().
		Save(ctx)
	if err != nil {
		return contract.InboxRecord{}, err
	}
	return toInbox(row), nil
}

func (s *Store) MarkInboxFailed(ctx context.Context, id int, attemptCount int, lastError string) (contract.InboxRecord, error) {
	row, err := s.client.DomainEventsInbox.UpdateOneID(id).
		SetStatus(string(contract.InboxStatusFailed)).
		SetAttemptCount(attemptCount).
		SetLastError(lastError).
		ClearProcessedAt().
		Save(ctx)
	if err != nil {
		return contract.InboxRecord{}, err
	}
	return toInbox(row), nil
}

func (s *Store) findOutboxByIdempotency(ctx context.Context, producerModule, idempotencyKey string) (contract.OutboxEvent, error) {
	row, err := s.client.DomainEventsOutbox.Query().
		Where(
			entdomaineventsoutbox.ProducerModuleEQ(producerModule),
			entdomaineventsoutbox.IdempotencyKeyEQ(idempotencyKey),
		).
		Only(ctx)
	if err != nil {
		return contract.OutboxEvent{}, err
	}
	return toOutbox(row), nil
}

func (s *Store) findInbox(ctx context.Context, eventID, consumerName string) (contract.InboxRecord, error) {
	row, err := s.client.DomainEventsInbox.Query().
		Where(
			entdomaineventsinbox.EventIDEQ(eventID),
			entdomaineventsinbox.ConsumerNameEQ(consumerName),
		).
		Only(ctx)
	if err != nil {
		return contract.InboxRecord{}, err
	}
	return toInbox(row), nil
}

func toOutbox(row *ent.DomainEventsOutbox) contract.OutboxEvent {
	return contract.OutboxEvent{
		ID:             row.ID,
		EventID:        row.EventID,
		EventType:      row.EventType,
		EventVersion:   row.EventVersion,
		ProducerModule: row.ProducerModule,
		AggregateType:  row.AggregateType,
		AggregateID:    row.AggregateID,
		CorrelationID:  row.CorrelationID,
		CausationID:    row.CausationID,
		IdempotencyKey: row.IdempotencyKey,
		Payload:        cloneMap(row.PayloadJSON),
		Metadata:       cloneMap(row.MetadataJSON),
		Status:         contract.OutboxStatus(row.Status),
		AttemptCount:   row.AttemptCount,
		NextRetryAt:    cloneTime(row.NextRetryAt),
		LastError:      cloneString(row.LastError),
		PublishedAt:    cloneTime(row.PublishedAt),
		CreatedAt:      row.CreatedAt,
	}
}

func toInbox(row *ent.DomainEventsInbox) contract.InboxRecord {
	return contract.InboxRecord{
		ID:           row.ID,
		EventID:      row.EventID,
		ConsumerName: row.ConsumerName,
		EventType:    row.EventType,
		Status:       contract.InboxStatus(row.Status),
		AttemptCount: row.AttemptCount,
		LastError:    cloneString(row.LastError),
		ProcessedAt:  cloneTime(row.ProcessedAt),
		CreatedAt:    row.CreatedAt,
	}
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
