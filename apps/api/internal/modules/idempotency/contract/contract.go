package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a record to complete does not exist.
var ErrNotFound = errors.New("idempotency record not found")

type Status string

const (
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
)

// Snapshot is the captured HTTP response of a completed idempotent request,
// replayed verbatim to a duplicate request.
type Snapshot struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

// Record is one stored idempotency entry, unique per (Key, Method, Path).
type Record struct {
	Key         string
	Method      string
	Path        string
	RequestHash string
	Status      Status
	Snapshot    *Snapshot
	LockedUntil *time.Time
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// BeginInput carries the values needed to insert or re-acquire a record.
type BeginInput struct {
	Key         string
	Method      string
	Path        string
	RequestHash string
	LockedUntil time.Time
	ExpiresAt   time.Time
	Now         time.Time
}

// Store persists idempotency records. Implementations must make InsertOrGet
// atomic with respect to the unique (Key, Method, Path) identity.
type Store interface {
	// InsertOrGet atomically inserts a fresh in-progress record and reports
	// inserted=true, or returns the pre-existing record with inserted=false.
	InsertOrGet(ctx context.Context, input BeginInput) (inserted bool, existing Record, err error)
	// Reacquire resets a stale in-progress record to a fresh in-progress lock and
	// request hash so a crashed request can be retried.
	Reacquire(ctx context.Context, input BeginInput) (Record, error)
	// Complete marks the record completed and stores the response snapshot.
	Complete(ctx context.Context, key, method, path string, snapshot *Snapshot, now time.Time) (Record, error)
	// DeleteExpired removes records whose ExpiresAt is before the given time and
	// reports how many were deleted.
	DeleteExpired(ctx context.Context, before time.Time) (int, error)
}
