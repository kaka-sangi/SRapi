package contract

import (
	"context"
	"time"
)

type Log struct {
	ID           int
	ActorUserID  *int
	Action       string
	ResourceType string
	ResourceID   string
	Before       map[string]any
	After        map[string]any
	IP           string
	UserAgent    string
	TraceID      string
	CreatedAt    time.Time
}

type RecordRequest struct {
	ActorUserID  *int
	Action       string
	ResourceType string
	ResourceID   string
	Before       map[string]any
	After        map[string]any
	IP           string
	UserAgent    string
	TraceID      string
}

type Store interface {
	Create(ctx context.Context, input Log) (Log, error)
	List(ctx context.Context) ([]Log, error)
}
