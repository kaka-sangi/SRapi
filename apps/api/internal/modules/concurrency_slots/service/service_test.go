package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestAcquire_CapacityZeroIsNoGate mirrors sub2api's
// TestAcquireAccountSlot_UnlimitedConcurrency: any non-positive capacity must
// admit the request immediately with a no-op release.
func TestAcquire_CapacityZeroIsNoGate(t *testing.T) {
	svc := New()
	for _, cap := range []int{0, -1, -10} {
		release, err := svc.AcquireSlot(context.Background(), 42, cap, 0)
		if err != nil {
			t.Fatalf("cap=%d unexpected err: %v", cap, err)
		}
		if release == nil {
			t.Fatalf("cap=%d release nil", cap)
		}
		release() // must be safely callable
	}
}

func TestAcquire_FastPath(t *testing.T) {
	svc := New()
	release, err := svc.AcquireSlot(context.Background(), 1, 2, 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if svc.InFlight(1) != 1 {
		t.Fatalf("InFlight = %d, want 1", svc.InFlight(1))
	}
	release()
	if svc.InFlight(1) != 0 {
		t.Fatalf("InFlight after release = %d, want 0", svc.InFlight(1))
	}
}

func TestAcquire_BlocksAtCapacity_ReleaseUnblocks(t *testing.T) {
	svc := New()
	r1, err := svc.AcquireSlot(context.Background(), 1, 1, 0)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer r1()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gotChan := make(chan error, 1)
	go func() {
		_, err := svc.AcquireSlot(ctx, 1, 1, 0)
		gotChan <- err
	}()
	select {
	case err := <-gotChan:
		t.Fatalf("second acquire returned %v while first still holds slot", err)
	case <-time.After(50 * time.Millisecond):
	}
	r1() // release should unblock the waiter

	select {
	case err := <-gotChan:
		if err != nil {
			t.Fatalf("waiter got err: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("waiter not unblocked after release")
	}
}

func TestAcquire_CtxCancelReturnsErr(t *testing.T) {
	svc := New()
	r1, err := svc.AcquireSlot(context.Background(), 7, 1, 0)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer r1()

	ctx, cancel := context.WithCancel(context.Background())
	gotChan := make(chan error, 1)
	go func() {
		_, err := svc.AcquireSlot(ctx, 7, 1, 0)
		gotChan <- err
	}()
	cancel()
	select {
	case err := <-gotChan:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("waiter not woken on cancel")
	}
}

func TestAcquire_WaitBudgetTimeout(t *testing.T) {
	svc := New()
	r1, err := svc.AcquireSlot(context.Background(), 3, 1, 0)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer r1()
	_, err = svc.AcquireSlot(context.Background(), 3, 1, 30*time.Millisecond)
	if !errors.Is(err, ErrSlotAcquireTimeout) {
		t.Fatalf("want ErrSlotAcquireTimeout, got %v", err)
	}
}

func TestAcquireStrict_ZeroCapErrors(t *testing.T) {
	svc := New()
	_, err := svc.AcquireSlotStrict(context.Background(), 1, 0, 0)
	if !errors.Is(err, ErrCapacityZero) {
		t.Fatalf("want ErrCapacityZero, got %v", err)
	}
}

func TestRelease_Idempotent(t *testing.T) {
	svc := New()
	release, err := svc.AcquireSlot(context.Background(), 2, 1, 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	release() // must not panic or under-flow
	if svc.InFlight(2) != 0 {
		t.Fatalf("InFlight = %d, want 0", svc.InFlight(2))
	}
}

func TestConcurrent_AcquireRelease_NoDeadlock(t *testing.T) {
	svc := New()
	const capacity = 4
	const workers = 50
	const perWorker = 20
	var wg sync.WaitGroup
	var peak atomic.Int64
	var inFlight atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				release, err := svc.AcquireSlot(context.Background(), 99, capacity, 0)
				if err != nil {
					t.Errorf("acquire: %v", err)
					return
				}
				now := inFlight.Add(1)
				for {
					p := peak.Load()
					if now <= p {
						break
					}
					if peak.CompareAndSwap(p, now) {
						break
					}
				}
				inFlight.Add(-1)
				release()
			}
		}()
	}
	wg.Wait()
	if got := peak.Load(); got > capacity {
		t.Fatalf("peak in-flight %d exceeded capacity %d", got, capacity)
	}
	if svc.InFlight(99) != 0 {
		t.Fatalf("InFlight after drain = %d", svc.InFlight(99))
	}
}

func TestPool_ResizesLazily(t *testing.T) {
	svc := New()
	// First call sets capacity=2.
	r1, err := svc.AcquireSlot(context.Background(), 5, 2, 0)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer r1()
	// Second call asks for capacity=4 — must NOT block.
	r2, err := svc.AcquireSlot(context.Background(), 5, 4, 0)
	if err != nil {
		t.Fatalf("acquire after resize: %v", err)
	}
	defer r2()
	r3, err := svc.AcquireSlot(context.Background(), 5, 4, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("third acquire after resize: %v", err)
	}
	defer r3()
}

func TestPool_LRUEviction(t *testing.T) {
	svc := NewWithMax(2)
	// Acquire+release on accounts 1, 2 → both idle and live.
	r1, _ := svc.AcquireSlot(context.Background(), 1, 1, 0)
	r1()
	r2, _ := svc.AcquireSlot(context.Background(), 2, 1, 0)
	r2()
	// Touching account 3 should evict the LRU (account 1).
	r3, _ := svc.AcquireSlot(context.Background(), 3, 1, 0)
	r3()
	// InFlight on 1 is now 0 — pool may have been evicted; both are fine.
	if svc.InFlight(1) != 0 {
		t.Fatalf("expected 0 in-flight on evicted account, got %d", svc.InFlight(1))
	}
}
