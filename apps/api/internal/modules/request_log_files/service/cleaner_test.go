package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCleaner_AgeBasedEviction asserts the retention sweep removes any
// managed file (request-* or error-*) whose mod-time predates the
// retention cutoff.
func TestCleaner_AgeBasedEviction(t *testing.T) {
	dir := t.TempDir()

	old := filepath.Join(dir, "request-1000-old.log")
	mid := filepath.Join(dir, "request-2000-mid.log")
	fresh := filepath.Join(dir, "request-3000-fresh.log")
	unrelated := filepath.Join(dir, "some-other.txt")
	for _, p := range []string{old, mid, fresh, unrelated} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", p, err)
		}
	}
	now := time.Now().UTC()
	if err := os.Chtimes(old, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(mid, now.Add(-3*24*time.Hour), now.Add(-3*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fresh, now, now); err != nil {
		t.Fatal(err)
	}

	c := NewCleaner(CleanerConfig{LogDir: dir, Retention: 7 * 24 * time.Hour, Now: func() time.Time { return now }})
	deleted, err := c.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion (the 30-day-old file), got %d", deleted)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old file removed, stat err=%v", err)
	}
	if _, err := os.Stat(mid); err != nil {
		t.Errorf("expected mid file kept, stat err=%v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("expected fresh file kept, stat err=%v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("expected unrelated file untouched, stat err=%v", err)
	}
}

// TestCleaner_ErrorFileCountCap asserts the per-prefix cap drops the
// oldest error-* files first.
func TestCleaner_ErrorFileCountCap(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC()
	files := []struct {
		name string
		age  time.Duration
	}{
		{"error-1-a.log", 4 * time.Hour},
		{"error-2-b.log", 3 * time.Hour},
		{"error-3-c.log", 2 * time.Hour},
		{"error-4-d.log", 1 * time.Hour},
	}
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := now.Add(-f.age)
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	c := NewCleaner(CleanerConfig{
		LogDir:        dir,
		Retention:     -1, // disable age-based eviction so we test the count cap in isolation
		MaxErrorFiles: 2,
		Now:           func() time.Time { return now },
	})
	deleted, err := c.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deletions (oldest 2 error files), got %d", deleted)
	}
	if _, err := os.Stat(filepath.Join(dir, "error-1-a.log")); !os.IsNotExist(err) {
		t.Errorf("expected oldest file removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "error-2-b.log")); !os.IsNotExist(err) {
		t.Errorf("expected second-oldest file removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "error-3-c.log")); err != nil {
		t.Errorf("expected newer file kept, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "error-4-d.log")); err != nil {
		t.Errorf("expected newest file kept, got err=%v", err)
	}
}

// TestCleaner_MissingDirIsNoOp asserts the cleaner does not error when
// the configured directory does not yet exist (e.g. capture is enabled
// but no request has been recorded yet).
func TestCleaner_MissingDirIsNoOp(t *testing.T) {
	c := NewCleaner(CleanerConfig{LogDir: filepath.Join(t.TempDir(), "nope")})
	deleted, err := c.SweepOnce(context.Background())
	if err != nil {
		t.Fatalf("SweepOnce: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deletions on missing dir, got %d", deleted)
	}
}
