package backup

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	backupsnapcontract "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
	backupsnapmemory "github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/store/memory"
)

func TestRunOnceHonorsEnabledAndRetention(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	settings := &fakeSettingsService{settings: admincontrolcontract.AdminSettings{
		General: admincontrolcontract.AdminSettingsGeneral{SiteName: "SRapi"},
		Users:   admincontrolcontract.AdminSettingsUsers{DefaultBalance: "0"},
		Gateway: admincontrolcontract.AdminSettingsGateway{StreamTimeoutSeconds: 600},
		Backup: admincontrolcontract.AdminSettingsBackup{
			Enabled:       false,
			RetentionDays: 1,
		},
	}}
	dir := t.TempDir()
	runner := &fakeRunner{}
	snapshots := backupsnapmemory.New()
	worker, err := New(settings, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		BackupDir: dir,
		Runner:    runner,
		Clock:     fixedClock{now: now},
		Snapshots: snapshots,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	filePath, deleted, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run disabled worker: %v", err)
	}
	if filePath != "" || deleted != 0 || runner.calls != 0 {
		t.Fatalf("disabled worker should not run backup, file=%q deleted=%d calls=%d", filePath, deleted, runner.calls)
	}

	old := filepath.Join(dir, "srapi-20260601000000.dump")
	if err := os.WriteFile(old, []byte("old"), 0o640); err != nil {
		t.Fatalf("write old backup: %v", err)
	}
	oldTime := now.Add(-48 * time.Hour)
	if err := os.Chtimes(old, oldTime, oldTime); err != nil {
		t.Fatalf("touch old backup: %v", err)
	}
	if err := os.WriteFile(old+".sha256", []byte("oldsum"), 0o640); err != nil {
		t.Fatalf("write old checksum: %v", err)
	}
	settings.settings.Backup.Enabled = true
	filePath, deleted, err = worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run enabled worker: %v", err)
	}
	if runner.calls != 1 || filePath == "" {
		t.Fatalf("expected one backup, file=%q calls=%d", filePath, runner.calls)
	}
	if settings.settings.Backup.LastBackupAt == nil || !settings.settings.Backup.LastBackupAt.Equal(now) {
		t.Fatalf("expected last backup time %v, got %v", now, settings.settings.Backup.LastBackupAt)
	}
	if _, err := os.Stat(filePath + ".sha256"); err != nil {
		t.Fatalf("expected checksum: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one old backup deleted, got %d", deleted)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("expected old backup removed, err=%v", err)
	}
	// Snapshot history row should reflect a successful scheduled run.
	list, err := snapshots.List(t.Context(), backupsnapcontract.ListOptions{})
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 {
		t.Fatalf("expected one snapshot row, got total=%d items=%d", list.Total, len(list.Items))
	}
	row := list.Items[0]
	if row.Status != backupsnapcontract.StatusSuccess {
		t.Fatalf("expected status=success, got %q (err=%q)", row.Status, row.ErrorMessage)
	}
	if row.Kind != backupsnapcontract.KindScheduled {
		t.Fatalf("expected kind=scheduled, got %q", row.Kind)
	}
	if row.FilePath != filePath {
		t.Fatalf("expected file_path %q, got %q", filePath, row.FilePath)
	}
	if row.SizeBytes <= 0 {
		t.Fatalf("expected non-zero size_bytes, got %d", row.SizeBytes)
	}
	if len(row.SHA256) != 64 {
		t.Fatalf("expected 64-char sha256, got %q", row.SHA256)
	}
}

func TestRunOnceTriggeredBypassesDailyCoolDown(t *testing.T) {
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	last := now.Add(-1 * time.Hour)
	settings := &fakeSettingsService{settings: admincontrolcontract.AdminSettings{
		Backup: admincontrolcontract.AdminSettingsBackup{
			Enabled:       true,
			RetentionDays: 0,
			LastBackupAt:  &last,
		},
	}}
	dir := t.TempDir()
	runner := &fakeRunner{}
	snapshots := backupsnapmemory.New()
	worker, err := New(settings, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		BackupDir: dir,
		Runner:    runner,
		Clock:     fixedClock{now: now},
		Snapshots: snapshots,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	// Scheduled run should NO-OP because LastBackupAt is well inside the 24h
	// window.
	if filePath, _, err := worker.RunOnce(t.Context()); err != nil || filePath != "" {
		t.Fatalf("scheduled cool-down should suppress run, got file=%q err=%v", filePath, err)
	}
	if runner.calls != 0 {
		t.Fatalf("expected runner not to be called, got %d calls", runner.calls)
	}
	// Manual trigger should bypass the cool-down.
	id, err := worker.RunOnceTriggered(t.Context(), 42)
	if err != nil {
		t.Fatalf("manual trigger: %v", err)
	}
	if id == 0 {
		t.Fatalf("expected manual trigger to create a snapshot id, got 0")
	}
	if runner.calls != 1 {
		t.Fatalf("expected one runner call, got %d", runner.calls)
	}
	list, err := snapshots.List(t.Context(), backupsnapcontract.ListOptions{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("expected one snapshot row, got %d", len(list.Items))
	}
	row := list.Items[0]
	if row.Kind != backupsnapcontract.KindManual {
		t.Fatalf("expected kind=manual, got %q", row.Kind)
	}
	if row.TriggeredByUserID != 42 {
		t.Fatalf("expected triggered_by_user_id=42, got %d", row.TriggeredByUserID)
	}
}

type fakeSettingsService struct {
	settings admincontrolcontract.AdminSettings
}

func (s *fakeSettingsService) GetAdminSettings(context.Context) (admincontrolcontract.AdminSettings, error) {
	return s.settings, nil
}

func (s *fakeSettingsService) UpdateAdminSettings(_ context.Context, settings admincontrolcontract.AdminSettings, _ int) (admincontrolcontract.AdminSettings, error) {
	s.settings = settings
	return settings, nil
}

type fakeRunner struct {
	calls int
}

func (r *fakeRunner) RunBackup(_ context.Context, filePath string) error {
	r.calls++
	return os.WriteFile(filePath, []byte("backup"), 0o640)
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
