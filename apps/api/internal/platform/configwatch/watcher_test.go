package configwatch

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatcherDetectsChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	w, err := New(Config{Path: path, Interval: 50 * time.Millisecond}, func(string) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	// Wait, then modify the file.
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(path, []byte("v2-changed"), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for at least one poll cycle.
	time.Sleep(120 * time.Millisecond)
	cancel()
	<-w.stopped

	if calls.Load() < 1 {
		t.Fatal("expected at least one onChange call after file modification")
	}
}

func TestWatcherNoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("stable"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	w, err := New(Config{Path: path, Interval: 50 * time.Millisecond}, func(string) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	// Wait a few poll cycles without modifying.
	time.Sleep(180 * time.Millisecond)
	cancel()
	<-w.stopped

	if calls.Load() != 0 {
		t.Fatalf("expected zero onChange calls without modification, got %d", calls.Load())
	}
}

func TestWatcherStopIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	w, err := New(Config{Path: path, Interval: time.Hour}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}

	go w.Start(context.Background())
	time.Sleep(10 * time.Millisecond)
	w.Stop()
	w.Stop() // second call should not panic
}

func TestDebounceCoalescesRapidChanges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	w, err := New(Config{
		Path:     path,
		Interval: 30 * time.Millisecond,
		Debounce: 100 * time.Millisecond,
	}, func(string) {
		calls.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go w.Start(ctx)

	// Rapid-fire 5 changes within the debounce window.
	for i := range 5 {
		time.Sleep(15 * time.Millisecond)
		if err := os.WriteFile(path, []byte("v"+string(rune('2'+i))), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for debounce to fire.
	time.Sleep(200 * time.Millisecond)
	cancel()
	<-w.stopped

	// Should coalesce to 1-2 calls, not 5.
	c := calls.Load()
	if c == 0 {
		t.Fatal("expected at least one debounced callback")
	}
	if c >= 5 {
		t.Fatalf("debounce failed: got %d calls, expected fewer than 5", c)
	}
}

func TestNewFailsOnMissingFile(t *testing.T) {
	_, err := New(Config{Path: "/nonexistent/path/config.yaml"}, func(string) {})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
