package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/backup_snapshots/store/memory"
)

func newService(t *testing.T, trigger Trigger) (*Service, *memory.Store) {
	t.Helper()
	store := memory.New()
	svc, err := New(store, trigger, func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, store
}

type fakeTrigger struct {
	id        int
	err       error
	called    int
	lastUser  int
	createRow bool
	store     contract.Store
}

func (t *fakeTrigger) RunOnceTriggered(ctx context.Context, userID int) (int, error) {
	t.called++
	t.lastUser = userID
	if t.err != nil {
		return 0, t.err
	}
	if t.createRow && t.store != nil {
		row, err := t.store.Create(ctx, contract.CreateSnapshot{
			Kind:              contract.KindManual,
			StartedAt:         time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC),
			FilePath:          "/tmp/srapi-test.dump",
			TriggeredByUserID: userID,
		})
		if err != nil {
			return 0, err
		}
		t.id = row.ID
	}
	return t.id, nil
}

func TestServiceTriggerBackupNow(t *testing.T) {
	ctx := context.Background()

	t.Run("no trigger wired", func(t *testing.T) {
		svc, _ := newService(t, nil)
		if _, err := svc.TriggerBackupNow(ctx, 1); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput, got %v", err)
		}
	})

	t.Run("trigger returns id=0", func(t *testing.T) {
		svc, _ := newService(t, &fakeTrigger{id: 0})
		if _, err := svc.TriggerBackupNow(ctx, 1); err == nil {
			t.Fatalf("expected error for id=0, got nil")
		}
	})

	t.Run("trigger returns missing id", func(t *testing.T) {
		svc, _ := newService(t, &fakeTrigger{id: 999})
		if _, err := svc.TriggerBackupNow(ctx, 1); !errors.Is(err, contract.ErrNotFound) {
			t.Fatalf("expected ErrNotFound (orphan id), got %v", err)
		}
	})

	t.Run("trigger succeeds", func(t *testing.T) {
		store := memory.New()
		trig := &fakeTrigger{createRow: true, store: store}
		svc, err := New(store, trig, func() time.Time { return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC) })
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		row, err := svc.TriggerBackupNow(ctx, 42)
		if err != nil {
			t.Fatalf("trigger: %v", err)
		}
		if row.ID == 0 || row.TriggeredByUserID != 42 || row.Kind != contract.KindManual {
			t.Fatalf("unexpected row: %+v", row)
		}
		if trig.called != 1 || trig.lastUser != 42 {
			t.Fatalf("expected one trigger call for user 42, got called=%d user=%d", trig.called, trig.lastUser)
		}
	})

	t.Run("trigger returns error", func(t *testing.T) {
		boom := errors.New("worker exploded")
		svc, _ := newService(t, &fakeTrigger{err: boom})
		if _, err := svc.TriggerBackupNow(ctx, 1); !errors.Is(err, boom) {
			t.Fatalf("expected boom, got %v", err)
		}
	})
}

func TestServiceListBackupSnapshots(t *testing.T) {
	ctx := context.Background()
	svc, store := newService(t, nil)

	// Seed three rows, the second succeeded, the first failed, the third
	// still running. ListBackupSnapshots orders by started_at desc, so the
	// list comes back as third (newest), second, first (oldest).
	a, _ := store.Create(ctx, contract.CreateSnapshot{StartedAt: time.Date(2026, 6, 16, 9, 0, 0, 0, time.UTC), FilePath: "/a"})
	b, _ := store.Create(ctx, contract.CreateSnapshot{StartedAt: time.Date(2026, 6, 16, 10, 0, 0, 0, time.UTC), FilePath: "/b"})
	c, _ := store.Create(ctx, contract.CreateSnapshot{StartedAt: time.Date(2026, 6, 16, 11, 0, 0, 0, time.UTC), FilePath: "/c"})
	_ = a
	_ = c
	if _, err := store.MarkFailed(ctx, a.ID, time.Date(2026, 6, 16, 9, 5, 0, 0, time.UTC), "pg_dump exploded"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	if _, err := store.MarkComplete(ctx, b.ID, time.Date(2026, 6, 16, 10, 5, 0, 0, time.UTC), 1234, "deadbeef"); err != nil {
		t.Fatalf("mark complete: %v", err)
	}

	list, err := svc.ListBackupSnapshots(ctx, contract.ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list.Total != 3 || len(list.Items) != 3 {
		t.Fatalf("expected 3 rows, got total=%d items=%d", list.Total, len(list.Items))
	}
	if list.Items[0].ID != c.ID {
		t.Fatalf("expected first row to be newest (%d), got %d", c.ID, list.Items[0].ID)
	}
	if list.Items[2].ID != a.ID {
		t.Fatalf("expected last row to be oldest (%d), got %d", a.ID, list.Items[2].ID)
	}

	// Status filter: only the failed row.
	failed, err := svc.ListBackupSnapshots(ctx, contract.ListOptions{Status: contract.StatusFailed, Limit: 10})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if failed.Total != 1 || len(failed.Items) != 1 || failed.Items[0].ID != a.ID {
		t.Fatalf("expected one failed row %d, got %+v", a.ID, failed.Items)
	}

	// Pagination: offset 1 limit 1 returns the middle (b).
	mid, err := svc.ListBackupSnapshots(ctx, contract.ListOptions{Offset: 1, Limit: 1})
	if err != nil {
		t.Fatalf("paginated list: %v", err)
	}
	if mid.Total != 3 || len(mid.Items) != 1 || mid.Items[0].ID != b.ID {
		t.Fatalf("expected paginated mid row %d, got %+v", b.ID, mid)
	}
}

func TestServiceGetBackupSnapshot(t *testing.T) {
	ctx := context.Background()
	svc, store := newService(t, nil)
	if _, err := svc.GetBackupSnapshot(ctx, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for id=0, got %v", err)
	}
	if _, err := svc.GetBackupSnapshot(ctx, 999); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	row, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: "/x"})
	got, err := svc.GetBackupSnapshot(ctx, row.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != row.ID {
		t.Fatalf("expected id %d, got %d", row.ID, got.ID)
	}
}

func TestServiceDeleteBackupSnapshot(t *testing.T) {
	ctx := context.Background()
	svc, store := newService(t, nil)

	t.Run("missing", func(t *testing.T) {
		if err := svc.DeleteBackupSnapshot(ctx, 999, 1); !errors.Is(err, contract.ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("removes file and row", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "srapi-x.dump")
		if err := os.WriteFile(filePath, []byte("dump"), 0o640); err != nil {
			t.Fatalf("seed file: %v", err)
		}
		if err := os.WriteFile(filePath+".sha256", []byte("hash"), 0o640); err != nil {
			t.Fatalf("seed checksum: %v", err)
		}
		row, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: filePath})
		if err := svc.DeleteBackupSnapshot(ctx, row.ID, 7); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Fatalf("expected file removed, got err=%v", err)
		}
		if _, err := store.FindByID(ctx, row.ID); !errors.Is(err, contract.ErrNotFound) {
			t.Fatalf("expected row removed, got err=%v", err)
		}
	})

	t.Run("missing file is best-effort", func(t *testing.T) {
		row, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: "/tmp/srapi-this-does-not-exist.dump"})
		if err := svc.DeleteBackupSnapshot(ctx, row.ID, 7); err != nil {
			t.Fatalf("delete missing-file row: %v", err)
		}
	})
}

func TestServiceOpenBackupFile(t *testing.T) {
	ctx := context.Background()
	svc, store := newService(t, nil)

	// Status running: refuse.
	running, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: "/nowhere.dump"})
	if _, err := svc.OpenBackupFile(ctx, running.ID); err == nil {
		t.Fatalf("expected error opening running snapshot")
	}

	// Status superseded: refuse.
	supersededRow, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: "/x.dump"})
	_, _ = store.MarkSuperseded(ctx, supersededRow.ID)
	if _, err := svc.OpenBackupFile(ctx, supersededRow.ID); err == nil {
		t.Fatalf("expected error opening superseded snapshot")
	}

	// Status success but missing file: stat error.
	missing, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: "/tmp/srapi-missing-real.dump"})
	_, _ = store.MarkComplete(ctx, missing.ID, time.Now(), 0, "")
	if _, err := svc.OpenBackupFile(ctx, missing.ID); err == nil {
		t.Fatalf("expected error opening missing file")
	}

	// Happy path: real file on disk.
	dir := t.TempDir()
	path := filepath.Join(dir, "srapi-real.dump")
	if err := os.WriteFile(path, []byte("hello"), 0o640); err != nil {
		t.Fatalf("seed: %v", err)
	}
	ok, _ := store.Create(ctx, contract.CreateSnapshot{FilePath: path})
	_, _ = store.MarkComplete(ctx, ok.ID, time.Now(), 5, "0123")
	open, err := svc.OpenBackupFile(ctx, ok.ID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer open.Reader.Close()
	if open.FileName != "srapi-real.dump" {
		t.Fatalf("expected filename srapi-real.dump, got %q", open.FileName)
	}
	if open.Size != 5 {
		t.Fatalf("expected size 5, got %d", open.Size)
	}
	body, err := io.ReadAll(open.Reader)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("expected hello, got %q", string(body))
	}
}
