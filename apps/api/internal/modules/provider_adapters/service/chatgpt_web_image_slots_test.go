package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestChatGPTWebImageSlotLimiterCapsConcurrency(t *testing.T) {
	l := NewChatGPTWebImageSlotLimiter(2)
	ctx := context.Background()
	if err := l.Acquire(ctx, "a", 0); err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if err := l.Acquire(ctx, "a", 0); err != nil {
		t.Fatalf("acquire 2: %v", err)
	}
	if got := l.Inflight("a"); got != 2 {
		t.Fatalf("Inflight = %d", got)
	}
	// Third acquire must block; verify with a short ctx timeout.
	ctx3, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	err := l.Acquire(ctx3, "a", 0)
	if !errors.Is(err, ErrChatGPTWebImageSlotCancelled) {
		t.Fatalf("expected cancelled; got %v", err)
	}
	l.Release("a")
	l.Release("a")
	if got := l.Inflight("a"); got != 0 {
		t.Fatalf("Inflight after release = %d", got)
	}
}

func TestChatGPTWebImageSlotLimiterReleaseWakesWaiter(t *testing.T) {
	l := NewChatGPTWebImageSlotLimiter(1)
	ctx := context.Background()
	if err := l.Acquire(ctx, "x", 0); err != nil {
		t.Fatalf("acquire: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	acquired := make(chan struct{})
	go func() {
		defer wg.Done()
		_ = l.Acquire(ctx, "x", 0)
		close(acquired)
	}()
	// Give the goroutine a moment to enqueue.
	time.Sleep(20 * time.Millisecond)
	l.Release("x")
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("waiter did not acquire after release")
	}
	l.Release("x")
	wg.Wait()
}

func TestChatGPTWebImageSlotKey(t *testing.T) {
	if k := chatGPTWebImageSlotKey(7, ""); k != "acct-7" {
		t.Errorf("key for id 7 = %q", k)
	}
	if k := chatGPTWebImageSlotKey(0, "abcdefghijklmnopqrstuv"); k != "tok-abcdefghijklmnop" {
		t.Errorf("key for token = %q", k)
	}
	if k := chatGPTWebImageSlotKey(0, ""); k != "" {
		t.Errorf("expected empty key, got %q", k)
	}
}

func TestChatGPTWebImageSlotLimiterPerAccountIsolation(t *testing.T) {
	l := NewChatGPTWebImageSlotLimiter(1)
	ctx := context.Background()
	if err := l.Acquire(ctx, "a", 0); err != nil {
		t.Fatalf("a1: %v", err)
	}
	// Different account must not block.
	done := make(chan struct{})
	go func() {
		_ = l.Acquire(ctx, "b", 0)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("acquire on different account blocked")
	}
}
