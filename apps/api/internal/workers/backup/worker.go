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
	"sync"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	admincontrolservice "github.com/srapi/srapi/apps/api/internal/modules/admin_control/service"
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

type Runner interface {
	RunBackup(ctx context.Context, filePath string) error
}

type Config struct {
	Interval  time.Duration
	BackupDir string
	Runner    Runner
	Clock     admincontrolservice.Clock
	RunGuard  runonceguard.Guard
}

type Worker struct {
	settings  SettingsService
	runner    Runner
	logger    *slog.Logger
	interval  time.Duration
	backupDir string
	clock     func() time.Time
	guard     runonceguard.Guard

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

func (w *Worker) RunOnce(ctx context.Context) (string, int, error) {
	if w == nil {
		return "", 0, nil
	}
	var filePath string
	var deleted int
	_, err := runonceguard.Run(ctx, w.guard, "backup", func(runCtx context.Context) error {
		var runErr error
		filePath, deleted, runErr = w.runOnce(runCtx)
		return runErr
	})
	return filePath, deleted, err
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

func (w *Worker) runOnce(ctx context.Context) (string, int, error) {
	settings, err := w.settings.GetAdminSettings(ctx)
	if err != nil {
		return "", 0, err
	}
	if !settings.Backup.Enabled {
		return "", 0, nil
	}
	now := w.clock().UTC()
	if settings.Backup.LastBackupAt != nil && now.Sub(settings.Backup.LastBackupAt.UTC()) < 24*time.Hour {
		deleted, err := w.cleanupOldBackups(now, settings.Backup.RetentionDays)
		return "", deleted, err
	}
	if err := os.MkdirAll(w.backupDir, 0o750); err != nil {
		return "", 0, err
	}
	filePath := filepath.Join(w.backupDir, fmt.Sprintf("srapi-%s.dump", now.Format("20060102150405")))
	if err := w.runner.RunBackup(ctx, filePath); err != nil {
		return "", 0, err
	}
	if err := writeChecksum(filePath); err != nil {
		return "", 0, err
	}
	settings.Backup.LastBackupAt = &now
	if _, err := w.settings.UpdateAdminSettings(ctx, settings, 0); err != nil {
		return "", 0, err
	}
	deleted, err := w.cleanupOldBackups(now, settings.Backup.RetentionDays)
	return filePath, deleted, err
}

func (w *Worker) cleanupOldBackups(now time.Time, retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	cutoff := now.Add(-time.Duration(retentionDays) * 24 * time.Hour)
	matches, err := filepath.Glob(filepath.Join(w.backupDir, "srapi-*.dump"))
	if err != nil {
		return 0, err
	}
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
	}
	return deleted, nil
}

func writeChecksum(filePath string) error {
	body, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	line := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), filepath.Base(filePath))
	return os.WriteFile(filePath+".sha256", []byte(line), 0o640)
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
