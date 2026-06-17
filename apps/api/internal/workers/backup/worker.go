package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
	backupsnapcontract "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
	"github.com/srapi/srapi/apps/api/internal/workers/runonceguard"
)

const (
	defaultInterval      = time.Hour
	defaultBackupDir     = "backups"
	shutdownPollInterval = 10 * time.Millisecond
)

type SettingsService interface {
	GetAdminSettings(ctx context.Context) (admincontrolcontract.AdminSettings, error)
	UpdateAdminSettings(ctx context.Context, settings admincontrolcontract.AdminSettings, actorUserID int) (admincontrolcontract.AdminSettings, error)
}

// SnapshotStore is the subset of the BackupSnapshot store the worker holds.
// Kept narrow so the worker doesn't pull in the whole admin Service.
type SnapshotStore interface {
	Create(ctx context.Context, input backupsnapcontract.CreateSnapshot) (backupsnapcontract.BackupSnapshot, error)
	MarkComplete(ctx context.Context, id int, completedAt time.Time, sizeBytes int64, sha256 string) (backupsnapcontract.BackupSnapshot, error)
	MarkFailed(ctx context.Context, id int, completedAt time.Time, errorMessage string) (backupsnapcontract.BackupSnapshot, error)
	MarkSuperseded(ctx context.Context, id int) (backupsnapcontract.BackupSnapshot, error)
	List(ctx context.Context, opts backupsnapcontract.ListOptions) (backupsnapcontract.ListResult, error)
}

type Runner interface {
	RunBackup(ctx context.Context, filePath string) error
}

type Config struct {
	Interval  time.Duration
	BackupDir string
	Runner    Runner
	Clock     admincontrolservice.Clock
	RunGuard  runonceguard.Guard
	// Snapshots, when set, makes the worker record one BackupSnapshot row per
	// attempted run (status=running -> success|failed) and mark rows superseded
	// when retention cleanup deletes their on-disk file. Nil-safe: a nil
	// store turns the snapshot history into a no-op, keeping the legacy
	// LastBackupAt-only behavior so memory-storage runtimes still work.
	Snapshots SnapshotStore
}

type Worker struct {
	settings  SettingsService
	runner    Runner
	logger    *slog.Logger
	interval  time.Duration
	backupDir string
	clock     func() time.Time
	guard     runonceguard.Guard
	snapshots SnapshotStore

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func New(settings SettingsService, logger *slog.Logger, cfg Config) (*Worker, error) {
	if settings == nil || cfg.Runner == nil {
		return nil, admincontrolcontract.ErrInvalidInput
	}
	if logger == nil {
		logger = slog.Default()
	}
	clock := time.Now
	if cfg.Clock != nil {
		clock = func() time.Time { return cfg.Clock.Now() }
	}
	backupDir := cfg.BackupDir
	if backupDir == "" {
		backupDir = defaultBackupDir
	}
	return &Worker{
		settings:  settings,
		runner:    cfg.Runner,
		logger:    logger,
		interval:  durationOrDefault(cfg.Interval, defaultInterval),
		backupDir: backupDir,
		clock:     clock,
		guard:     cfg.RunGuard,
		snapshots: cfg.Snapshots,
	}, nil
}

func NewFromStore(store admincontrolcontract.Store, logger *slog.Logger, cfg Config) (*Worker, error) {
	settings, err := admincontrolservice.New(store, cfg.Clock)
	if err != nil {
		return nil, err
	}
	return New(settings, logger, cfg)
}

func (w *Worker) Start(parent context.Context) {
	if w == nil {
		return
	}
	if parent == nil {
		parent = context.Background()
	}
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				w.logger.Error("worker panicked; goroutine stopped", "worker", "backup", "panic", r, "stack", string(debug.Stack()))
			}
		}()
		w.run(ctx)
	}()
}

func (w *Worker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	w.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	ticker := time.NewTicker(shutdownPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			w.mu.Lock()
			if w.done == done {
				w.cancel = nil
				w.done = nil
			}
			w.mu.Unlock()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RunOnce executes a scheduled snapshot pass. Returns the file path written
// (if any), the count of old files deleted by retention, and the snapshot
// row id (0 when no run happened). The id is non-zero only when Snapshots
// is configured AND a real backup ran.
func (w *Worker) RunOnce(ctx context.Context) (string, int, error) {
	id, filePath, deleted, err := w.runGuarded(ctx, "", 0)
	_ = id
	return filePath, deleted, err
}

// RunOnceTriggered forces an immediate snapshot tagged kind=manual and the
// supplied admin userID, bypassing the 24-hour scheduled-run cool-down so
// an operator who pressed "Snapshot now" gets a real run on the same
// request. Returns the snapshot id created by the run (0 when no run
// happened — e.g. backup disabled in admin settings).
func (w *Worker) RunOnceTriggered(ctx context.Context, userID int) (int, error) {
	id, _, _, err := w.runGuarded(ctx, backupsnapcontract.KindManual, userID)
	return id, err
}

func (w *Worker) runGuarded(ctx context.Context, kind string, userID int) (int, string, int, error) {
	if w == nil {
		return 0, "", 0, nil
	}
	var id int
	var filePath string
	var deleted int
	_, err := runonceguard.Run(ctx, w.guard, "backup", func(runCtx context.Context) error {
		var runErr error
		id, filePath, deleted, runErr = w.runOnce(runCtx, kind, userID)
		return runErr
	})
	return id, filePath, deleted, err
}

func (w *Worker) run(ctx context.Context) {
	w.runAndLog(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runAndLog(ctx)
		}
	}
}

func (w *Worker) runAndLog(ctx context.Context) {
	filePath, deleted, err := w.RunOnce(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("backup worker failed", "error", err)
		return
	}
	if filePath != "" {
		w.logger.Info("backup completed", "file", filePath, "deleted_old_files", deleted)
	}
}

// runOnce performs the actual snapshot pass. The `kind` argument selects the
// snapshot row's kind field; empty means "scheduled". userID is the admin
// that triggered a manual run (0 for scheduled).
//
// Returns: (snapshot id, file path, deleted old files, error). id is 0 when
// no snapshot row was created (backup disabled, throttled, or store nil).
func (w *Worker) runOnce(ctx context.Context, kind string, userID int) (int, string, int, error) {
	settings, err := w.settings.GetAdminSettings(ctx)
	if err != nil {
		return 0, "", 0, err
	}
	if !settings.Backup.Enabled {
		return 0, "", 0, nil
	}
	now := w.clock().UTC()
	manual := kind == backupsnapcontract.KindManual
	if !manual && settings.Backup.LastBackupAt != nil && now.Sub(settings.Backup.LastBackupAt.UTC()) < 24*time.Hour {
		// Scheduled pass and we're still within the per-day cool-down: do
		// retention cleanup, then return without recording a snapshot row.
		deleted, err := w.cleanupOldBackups(ctx, now, settings.Backup.RetentionDays)
		return 0, "", deleted, err
	}
	if err := os.MkdirAll(w.backupDir, 0o750); err != nil {
		return 0, "", 0, err
	}
	filePath := filepath.Join(w.backupDir, fmt.Sprintf("srapi-%s.dump", now.Format("20060102150405")))

	// Create the history row BEFORE the long-running pg_dump so the UI shows
	// a "running" badge while the file streams to disk.
	resolvedKind := backupsnapcontract.KindScheduled
	if manual {
		resolvedKind = backupsnapcontract.KindManual
	}
	snapshotID := 0
	if w.snapshots != nil {
		row, sErr := w.snapshots.Create(ctx, backupsnapcontract.CreateSnapshot{
			Kind:              resolvedKind,
			StartedAt:         now,
			FilePath:          filePath,
			TriggeredByUserID: userID,
		})
		if sErr != nil {
			w.logger.Warn("failed to record backup snapshot row", "error", sErr)
		} else {
			snapshotID = row.ID
		}
	}

	if err := w.runner.RunBackup(ctx, filePath); err != nil {
		if snapshotID > 0 {
			_, _ = w.snapshots.MarkFailed(ctx, snapshotID, w.clock().UTC(), err.Error())
		}
		return snapshotID, "", 0, err
	}
	size, sum, checksumErr := writeChecksumAndStat(filePath)
	if checksumErr != nil {
		if snapshotID > 0 {
			_, _ = w.snapshots.MarkFailed(ctx, snapshotID, w.clock().UTC(), checksumErr.Error())
		}
		return snapshotID, "", 0, checksumErr
	}
	if snapshotID > 0 {
		if _, sErr := w.snapshots.MarkComplete(ctx, snapshotID, w.clock().UTC(), size, sum); sErr != nil {
			w.logger.Warn("failed to mark backup snapshot complete", "snapshot_id", snapshotID, "error", sErr)
		}
	}
	settings.Backup.LastBackupAt = &now
	if _, err := w.settings.UpdateAdminSettings(ctx, settings, 0); err != nil {
		return snapshotID, "", 0, err
	}
	deleted, err := w.cleanupOldBackups(ctx, now, settings.Backup.RetentionDays)
	return snapshotID, filePath, deleted, err
}

// cleanupOldBackups removes dump files older than retentionDays. For each
// file removed it also marks the matching BackupSnapshot row (looked up by
// file_path) status=superseded so the UI can show the row but disable
// download. Files with no matching row (e.g. created before the history
// layer landed) are still cleaned up — the row marking is best-effort.
func (w *Worker) cleanupOldBackups(ctx context.Context, now time.Time, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	matches, err := filepath.Glob(filepath.Join(w.backupDir, "srapi-*.dump"))
	if err != nil {
		return 0, err
	}
	var deletedPaths []string
	var deleted int
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return deleted, err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return deleted, err
		}
		_ = os.Remove(path + ".sha256")
		deleted++
		deletedPaths = append(deletedPaths, path)
	}
	if w.snapshots != nil && len(deletedPaths) > 0 {
		w.markSupersededByPath(ctx, deletedPaths)
	}
	return deleted, nil
}

// markSupersededByPath looks up still-success rows whose file_path matches
// one of the just-deleted files and marks them superseded. The lookup uses
// a single bounded List call rather than a per-path query — typical
// retention windows only delete a handful of files at a time, and the
// snapshot history is short by design.
func (w *Worker) markSupersededByPath(ctx context.Context, deletedPaths []string) {
	if len(deletedPaths) == 0 || w.snapshots == nil {
		return
	}
	pathSet := make(map[string]struct{}, len(deletedPaths))
	for _, p := range deletedPaths {
		pathSet[p] = struct{}{}
	}
	res, err := w.snapshots.List(ctx, backupsnapcontract.ListOptions{
		Status: backupsnapcontract.StatusSuccess,
		Limit:  500,
	})
	if err != nil {
		w.logger.Warn("failed to scan snapshots for retention markdown", "error", err)
		return
	}
	for _, row := range res.Items {
		if _, hit := pathSet[row.FilePath]; !hit {
			continue
		}
		if _, err := w.snapshots.MarkSuperseded(ctx, row.ID); err != nil {
			w.logger.Warn("failed to mark snapshot superseded", "snapshot_id", row.ID, "error", err)
		}
	}
}

func writeChecksumAndStat(filePath string) (int64, string, error) {
	body, err := os.ReadFile(filePath)
	if err != nil {
		return 0, "", err
	}
	sum := sha256.Sum256(body)
	hexSum := hex.EncodeToString(sum[:])
	line := fmt.Sprintf("%s  %s\n", hexSum, filepath.Base(filePath))
	if err := os.WriteFile(filePath+".sha256", []byte(line), 0o640); err != nil {
		return 0, "", err
	}
	return int64(len(body)), hexSum, nil
}

type PostgresRunner struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string
}

func (r PostgresRunner) RunBackup(ctx context.Context, filePath string) error {
	port := r.Port
	if port == 0 {
		port = 5432
	}
	args := []string{
		"--host", r.Host,
		"--port", fmt.Sprintf("%d", port),
		"--username", r.User,
		"--dbname", r.Database,
		"--format", "custom",
		"--file", filePath,
	}
	if r.SSLMode != "" {
		args = append(args, "--no-password")
	}
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	cmd.Env = append(os.Environ(),
		"PGPASSWORD="+r.Password,
		"PGSSLMODE="+r.SSLMode,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_dump failed: %w: %s", err, string(output))
	}
	return nil
}

func durationOrDefault(value, fallback time.Duration) time.Duration {
	if value <= 0 {
		return fallback
	}
	return value
}
