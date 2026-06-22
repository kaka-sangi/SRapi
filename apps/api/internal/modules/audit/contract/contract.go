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

// ListFilter narrows a paginated audit-log read at the store level. Empty
// strings and nil pointers are no-ops.
type ListFilter struct {
	Action       string
	ResourceType string
	ActorUserID  *int
	Since        *time.Time
}

// ListPageResult is the typed return of PageReader.ListPage.
type ListPageResult struct {
	Items []Log
	Total int
}

// PageReader is an optional Store capability that pushes filtering, ordering
// (newest-first by id), and LIMIT/OFFSET pagination down to SQL — replaces
// admin handlers that loaded the entire audit_logs table to filter+paginate
// in Go memory.
type PageReader interface {
	ListPage(ctx context.Context, filter ListFilter, limit, offset int) (ListPageResult, error)
}
