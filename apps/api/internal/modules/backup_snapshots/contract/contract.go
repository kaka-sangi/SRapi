// Package contract defines the BackupSnapshot domain model. A BackupSnapshot
// is one row of the database-backup history: started_at / completed_at,
// the on-disk file path and sha256, status (running|success|failed|
// superseded), and (for manual triggers) the admin user that invoked it.
//
// The existing daily backup worker (apps/api/internal/workers/backup) writes
// rows here at run start (status=running) and stamps the final outcome at run
// finish. Retention cleanup marks rows superseded when the file is deleted.
// Operators read the history through the admin handlers in
// apps/api/internal/httpserver/runtime_admin_backups_handlers.go.
package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when a backup snapshot row does not exist.
var ErrNotFound = errors.New("backup snapshot not found")

// Snapshot kind values.
const (
	KindScheduled = "scheduled"
	KindManual    = "manual"
)

// Snapshot status values.
const (
	StatusRunning    = "running"
	StatusSuccess    = "success"
	StatusFailed     = "failed"
	StatusSuperseded = "superseded"
)

// BackupSnapshot mirrors the ent row. Times are UTC.
type BackupSnapshot struct {
	ID                int
	Kind              string
	StartedAt         time.Time
	CompletedAt       *time.Time
	SizeBytes         int64
	SHA256            string
	Status            string
	FilePath          string
	ErrorMessage      string
	TriggeredByUserID int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// CreateSnapshot is the create payload — the worker calls this at the start
// of a run so a "running" row is visible in the admin UI while pg_dump is
// still streaming.
type CreateSnapshot struct {
	Kind              string
	StartedAt         time.Time
	FilePath          string
	TriggeredByUserID int
}

// ListOptions paginates and filters the snapshot history.
type ListOptions struct {
	Offset int
	Limit  int
	Status string
}

// ListResult is the page returned by Store.List — Total is the count
// matching the filter (unbounded by Offset/Limit) so the UI can render a
// pagination footer.
type ListResult struct {
	Items []BackupSnapshot
	Total int
}

// Store persists snapshot history. The backup worker holds a SnapshotStore
// view of this interface; the admin handlers hold the full Store + Service.
type Store interface {
	Create(ctx context.Context, input CreateSnapshot) (BackupSnapshot, error)
	MarkComplete(ctx context.Context, id int, completedAt time.Time, sizeBytes int64, sha256 string) (BackupSnapshot, error)
	MarkFailed(ctx context.Context, id int, completedAt time.Time, errorMessage string) (BackupSnapshot, error)
	MarkSuperseded(ctx context.Context, id int) (BackupSnapshot, error)
	List(ctx context.Context, opts ListOptions) (ListResult, error)
	FindByID(ctx context.Context, id int) (BackupSnapshot, error)
	Delete(ctx context.Context, id int) error
}

// Service is the orchestration layer above Store — adds file I/O, audit
// recording, and the worker hand-off for manual triggers.
type Service interface {
	TriggerBackupNow(ctx context.Context, userID int) (BackupSnapshot, error)
	ListBackupSnapshots(ctx context.Context, opts ListOptions) (ListResult, error)
	GetBackupSnapshot(ctx context.Context, id int) (BackupSnapshot, error)
	DeleteBackupSnapshot(ctx context.Context, id int, userID int) error
	OpenBackupFile(ctx context.Context, id int) (OpenFile, error)
}

// OpenFile carries the streaming handle returned by OpenBackupFile so the
// HTTP layer can stream the dump to the operator without copying it through
// a byte slice.
type OpenFile struct {
	Reader   ReadCloser
	FileName string
	Size     int64
}

// ReadCloser is the minimal subset of io.ReadCloser the service exposes; kept
// here so callers don't have to import io just to type the field.
type ReadCloser interface {
	Read(p []byte) (int, error)
	Close() error
}
