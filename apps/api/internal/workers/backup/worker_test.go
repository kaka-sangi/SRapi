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
	worker, err := New(settings, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		BackupDir: dir,
		Runner:    runner,
		Clock:     fixedClock{now: now},
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
