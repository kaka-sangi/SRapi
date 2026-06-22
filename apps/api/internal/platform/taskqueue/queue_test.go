package taskqueue

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueueDropsOnOverflow(t *testing.T) {
	q := New(Config{QueueSize: 2, Overflow: OverflowDrop})
	// Start with zero workers by using a canceled context so workers exit
	// immediately — the channel fills up and stays full.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before start so workers exit immediately
	q.Start(ctx)

	ok1 := q.Submit(func(ctx context.Context) {})
	ok2 := q.Submit(func(ctx context.Context) {})
	ok3 := q.Submit(func(ctx context.Context) {}) // should be dropped

	if !ok1 || !ok2 {
		t.Fatal("first two submits should succeed")
	}
	if ok3 {
		t.Fatal("third submit should be dropped (overflow)")
	}
	_, dropped := q.Stats()
	if dropped != 1 {
		t.Fatalf("expected 1 dropped, got %d", dropped)
	}
}

func TestQueueRunsSync(t *testing.T) {
	q := New(Config{QueueSize: 1, Overflow: OverflowRunSync})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q.Start(ctx)

	var ran atomic.Int64
	// Fill the channel.
	q.Submit(func(ctx context.Context) { ran.Add(1) })
	// This one should run synchronously in the caller's goroutine.
	ok := q.Submit(func(ctx context.Context) { ran.Add(1) })
	if !ok {
		t.Fatal("RunSync submit should return true")
	}
	// The sync task must have already executed before Submit returned.
	if ran.Load() < 1 {
		t.Fatal("expected sync task to have run")
	}
	executed, dropped := q.Stats()
	if dropped != 0 {
		t.Fatalf("expected 0 dropped, got %d", dropped)
	}
	if executed < 1 {
		t.Fatalf("expected at least 1 executed, got %d", executed)
	}
}

func TestQueueProcessesTasks(t *testing.T) {
	q := New(Config{Workers: 2, QueueSize: 16, Overflow: OverflowDrop})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	const n = 10
	var count atomic.Int64
	for i := 0; i < n; i++ {
		q.Submit(func(ctx context.Context) { count.Add(1) })
	}

	// Wait for workers to drain.
	deadline := time.After(2 * time.Second)
	for {
		if count.Load() >= n {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for tasks; got %d of %d", count.Load(), n)
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	executed, dropped := q.Stats()
	if executed != n {
		t.Fatalf("expected %d executed, got %d", n, executed)
	}
	if dropped != 0 {
		t.Fatalf("expected 0 dropped, got %d", dropped)
	}
}

func TestShutdownDrainsQueue(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 16, Overflow: OverflowDrop})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	var count atomic.Int64
	for i := 0; i < 5; i++ {
		q.Submit(func(ctx context.Context) {
			time.Sleep(10 * time.Millisecond)
			count.Add(1)
		})
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := q.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
	if count.Load() != 5 {
		t.Fatalf("expected 5 executed after shutdown, got %d", count.Load())
	}
}

func TestSubmitAfterShutdown(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 8, Overflow: OverflowDrop})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	shutdownCtx := context.Background()
	if err := q.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}

	ok := q.Submit(func(ctx context.Context) {})
	if ok {
		t.Fatal("submit after shutdown should return false")
	}
}

func TestWorkerRecoversPanic(t *testing.T) {
	q := New(Config{Workers: 1, QueueSize: 8, Overflow: OverflowDrop})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx)

	// Submit a panicking task.
	q.Submit(func(ctx context.Context) { panic("boom") })
	// Submit a normal task after the panicking one.
	var ran atomic.Bool
	q.Submit(func(ctx context.Context) { ran.Store(true) })

	deadline := time.After(2 * time.Second)
	for {
		if ran.Load() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out: worker did not recover from panic")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func TestLen(t *testing.T) {
	q := New(Config{QueueSize: 4, Overflow: OverflowDrop})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q.Start(ctx)

	q.Submit(func(ctx context.Context) {})
	q.Submit(func(ctx context.Context) {})

	if q.Len() != 2 {
		t.Fatalf("expected Len()=2, got %d", q.Len())
	}
}
