package contract

import (
	"context"
	"time"
)

type OutboxStatus string

const (
	OutboxStatusPending   OutboxStatus = "pending"
	OutboxStatusPublished OutboxStatus = "published"
	OutboxStatusFailed    OutboxStatus = "failed"
)

type InboxStatus string

const (
	InboxStatusPending   InboxStatus = "pending"
	InboxStatusProcessed InboxStatus = "processed"
	InboxStatusFailed    InboxStatus = "failed"
)

type OutboxEvent struct {
	ID             int
	EventID        string
	EventType      string
	EventVersion   string
	ProducerModule string
	AggregateType  string
	AggregateID    string
	CorrelationID  string
	CausationID    string
	IdempotencyKey string
	Payload        map[string]any
	Metadata       map[string]any
	Status         OutboxStatus
	AttemptCount   int
	NextRetryAt    *time.Time
	LastError      *string
	PublishedAt    *time.Time
	CreatedAt      time.Time
}

type EnqueueRequest struct {
	EventType      string
	EventVersion   string
	ProducerModule string
	AggregateType  string
	AggregateID    string
	CorrelationID  string
	CausationID    string
	IdempotencyKey string
	Payload        map[string]any
	Metadata       map[string]any
}

type InboxRecord struct {
	ID           int
	EventID      string
	ConsumerName string
	EventType    string
	Status       InboxStatus
	AttemptCount int
	LastError    *string
	ProcessedAt  *time.Time
	CreatedAt    time.Time
}

type RecordInboxRequest struct {
	EventID      string
	ConsumerName string
	EventType    string
}

type Store interface {
	CreateOutbox(ctx context.Context, input OutboxEvent) (OutboxEvent, error)
	ListOutbox(ctx context.Context) ([]OutboxEvent, error)
	ListDispatchableOutbox(ctx context.Context, now time.Time, limit int) ([]OutboxEvent, error)
	MarkOutboxPublished(ctx context.Context, id int, publishedAt time.Time) (OutboxEvent, error)
	MarkOutboxFailed(ctx context.Context, id int, attemptCount int, nextRetryAt *time.Time, lastError string) (OutboxEvent, error)
	CreateInbox(ctx context.Context, input InboxRecord) (InboxRecord, bool, error)
	ListInbox(ctx context.Context) ([]InboxRecord, error)
	MarkInboxProcessed(ctx context.Context, id int, processedAt time.Time) (InboxRecord, error)
	MarkInboxFailed(ctx context.Context, id int, attemptCount int, lastError string) (InboxRecord, error)
}
