// Package service orchestrates the BackupSnapshot history. It wraps the
// Store with the file-deletion, audit-recording, and worker hand-off that
// the HTTP handlers expect.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
)

// ErrInvalidInput is returned for malformed inputs.
var ErrInvalidInput = errors.New("invalid backup snapshot input")

// Trigger is the hand-off interface used by TriggerBackupNow — implemented
// by the backup worker so the admin "Snapshot now" button can kick off a
// real pg_dump run synchronously and return the resulting snapshot row.
type Trigger interface {
	RunOnceTriggered(ctx context.Context, userID int) (int, error)
}

// Clock returns the current time; defaults to time.Now.
type Clock func() time.Time

// Service implements contract.Service.
type Service struct {
	store   contract.Store
	trigger Trigger
	clock   Clock
}

func New(store contract.Store, trigger Trigger, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	return &Service{store: store, trigger: trigger, clock: clock}, nil
}

func (s *Service) now() time.Time { return s.clock().UTC() }

// TriggerBackupNow asks the backup worker to run a one-off snapshot tagged
// to the calling admin user, then returns the resulting row. When no
// trigger is wired (memory-only test runtime) the call returns ErrInvalidInput
// so callers can degrade gracefully.
func (s *Service) TriggerBackupNow(ctx context.Context, userID int) (contract.BackupSnapshot, error) {
	if s.trigger == nil {
		return contract.BackupSnapshot{}, ErrInvalidInput
	}
	id, err := s.trigger.RunOnceTriggered(ctx, userID)
	if err != nil {
		return contract.BackupSnapshot{}, err
	}
	if id <= 0 {
		// The worker can return 0 when the backup feature is disabled in
		// admin settings, or when an earlier run is still inside its
		// 24-hour cool-down. Surface that explicitly so the UI can show
		// the configured guard instead of a generic error.
		return contract.BackupSnapshot{}, fmt.Errorf("backup worker did not create a snapshot (disabled, throttled, or guard contention)")
	}
	return s.store.FindByID(ctx, id)
}

// ListBackupSnapshots returns the paginated history. Offset/Limit are
// applied by the store.
func (s *Service) ListBackupSnapshots(ctx context.Context, opts contract.ListOptions) (contract.ListResult, error) {
	if opts.Limit < 0 {
		opts.Limit = 0
	}
	if opts.Offset < 0 {
		opts.Offset = 0
	}
	return s.store.List(ctx, opts)
}

// GetBackupSnapshot returns a single row.
func (s *Service) GetBackupSnapshot(ctx context.Context, id int) (contract.BackupSnapshot, error) {
	if id <= 0 {
		return contract.BackupSnapshot{}, ErrInvalidInput
	}
	return s.store.FindByID(ctx, id)
}

// DeleteBackupSnapshot removes the dump file (best-effort), drops the row,
// and is meant to be wrapped with an audit-log call at the handler layer.
// userID is accepted so future revisions can stamp a "deleted_by" column
// without churning the API.
func (s *Service) DeleteBackupSnapshot(ctx context.Context, id int, _ int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	snap, err := s.store.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if snap.FilePath != "" {
		// Removing the file is best-effort: it might already be gone from a
		// previous retention sweep, or absent on a different host that
		// shares the DB but not the disk. Treat ENOENT as success.
		if err := os.Remove(snap.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove backup file: %w", err)
		}
		_ = os.Remove(snap.FilePath + ".sha256")
	}
	return s.store.Delete(ctx, id)
}

// OpenBackupFile streams the dump back to the operator. Refuses to open
// rows whose file has been deleted by retention (status=superseded) or
// rows that never finished (status=running|failed) — those have nothing to
// stream.
func (s *Service) OpenBackupFile(ctx context.Context, id int) (contract.OpenFile, error) {
	if id <= 0 {
		return contract.OpenFile{}, ErrInvalidInput
	}
	snap, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.OpenFile{}, err
	}
	if snap.Status != contract.StatusSuccess {
		return contract.OpenFile{}, fmt.Errorf("backup snapshot %d is not downloadable (status=%s)", id, snap.Status)
	}
	if snap.FilePath == "" {
		return contract.OpenFile{}, fmt.Errorf("backup snapshot %d has no file path", id)
	}
	info, err := os.Stat(snap.FilePath)
	if err != nil {
		return contract.OpenFile{}, fmt.Errorf("stat backup file: %w", err)
	}
	f, err := os.Open(snap.FilePath)
	if err != nil {
		return contract.OpenFile{}, fmt.Errorf("open backup file: %w", err)
	}
	return contract.OpenFile{
		Reader:   f,
		FileName: filepath.Base(snap.FilePath),
		Size:     info.Size(),
	}, nil
}
