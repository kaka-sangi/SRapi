package service

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestService(t *testing.T) (*Service, *atomic.Int64) {
	t.Helper()
	svc := New()
	clock := &atomic.Int64{}
	svc.now = func() time.Time { return time.Unix(0, clock.Load()) }
	return svc, clock
}

func setClock(c *atomic.Int64, when time.Time) {
	c.Store(when.UnixNano())
}

func TestRecord_AndIsAccountInCooldown(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(1000, 0))

	active, unblock := svc.IsAccountInCooldown(42)
	if active {
		t.Fatalf("fresh account should not be in cooldown, got unblock=%v", unblock)
	}

	svc.RecordRateLimitHit(42, 30*time.Second)
	active, unblock = svc.IsAccountInCooldown(42)
	if !active {
		t.Fatalf("after RecordRateLimitHit account should be in cooldown")
	}
	if !unblock.After(time.Unix(1029, 0)) {
		t.Fatalf("unblock too early: %v", unblock)
	}

	// Advance past the cooldown window.
	setClock(clock, time.Unix(1100, 0))
	active, unblock = svc.IsAccountInCooldown(42)
	if active {
		t.Fatalf("cooldown should have lapsed; unblock=%v", unblock)
	}
}

func TestRecord_ClampToMinCooldown(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(2000, 0))

	svc.RecordRateLimitHit(1, 0) // < minCooldown
	active, unblock := svc.IsAccountInCooldown(1)
	if !active {
		t.Fatalf("expected cooldown")
	}
	// Should clamp to >= 1s.
	if unblock.Before(time.Unix(2000, 0).Add(minCooldown - time.Nanosecond)) {
		t.Fatalf("unblock not clamped: %v", unblock)
	}
}

func TestRecord_ClampToMaxCooldown(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(3000, 0))

	svc.RecordRateLimitHit(2, 100*time.Hour) // > maxCooldown
	_, unblock := svc.IsAccountInCooldown(2)
	if got := unblock.Sub(time.Unix(3000, 0)); got > maxCooldown {
		t.Fatalf("unblock %v exceeds maxCooldown %v", got, maxCooldown)
	}
}

// TestConsecutiveDisable mirrors sub2api's "N consecutive 429s in window
// escalates to temp-disable" semantics.
func TestConsecutiveDisable_EscalatesToTempDisable(t *testing.T) {
	svc, clock := newTestService(t)
	t0 := time.Unix(10_000, 0)
	setClock(clock, t0)

	// Five hits in quick succession within consecutiveWindow.
	for i := 0; i < consecutiveDisableThreshold; i++ {
		setClock(clock, t0.Add(time.Duration(i)*time.Second))
		svc.RecordRateLimitHit(7, 5*time.Second)
	}
	active, unblock := svc.IsAccountInCooldown(7)
	if !active {
		t.Fatalf("expected temp-disable")
	}
	// Should be at least disableCooldown into the future from the last hit.
	lastHit := t0.Add(time.Duration(consecutiveDisableThreshold-1) * time.Second)
	want := lastHit.Add(disableCooldown).Add(-time.Second)
	if !unblock.After(want) {
		t.Fatalf("temp-disable too short: unblock=%v want > %v", unblock, want)
	}
}

func TestConsecutiveDisable_WindowSlidesOff(t *testing.T) {
	svc, clock := newTestService(t)
	t0 := time.Unix(20_000, 0)
	setClock(clock, t0)

	// One hit, then advance past the window, then more hits → counter resets.
	svc.RecordRateLimitHit(8, time.Second)
	setClock(clock, t0.Add(consecutiveWindow+time.Minute))
	for i := 0; i < consecutiveDisableThreshold-1; i++ {
		svc.RecordRateLimitHit(8, time.Second)
	}
	// Below threshold ⇒ only normal cooldown, not temp-disable.
	setClock(clock, t0.Add(consecutiveWindow+time.Minute+2*time.Second))
	active, _ := svc.IsAccountInCooldown(8)
	// Either expired or in normal cooldown — both fine; what we're
	// asserting is that we have NOT crossed into a multi-minute temp-disable
	// triggered by stale hits.
	if active {
		// If active, it should be sub-disableCooldown — assert by re-recording 0s.
		// Easier path: ensure entry hits length < threshold.
		svc.mu.Lock()
		elem := svc.entries[8]
		entry := elem.Value.(*cooldownEntry)
		hits := len(entry.hits)
		svc.mu.Unlock()
		if hits >= consecutiveDisableThreshold {
			t.Fatalf("stale hits not slid off: hits=%d", hits)
		}
	}
}

func TestFilterCooldownedAccounts(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(5000, 0))

	svc.RecordRateLimitHit(1, 10*time.Second)
	svc.RecordRateLimitHit(3, 10*time.Second)

	got := svc.FilterCooldownedAccounts([]int64{1, 2, 3, 4})
	if len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("FilterCooldownedAccounts = %v, want [1 3]", got)
	}
}

func TestReset_ClearsEntry(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(7000, 0))

	svc.RecordRateLimitHit(9, 30*time.Second)
	svc.Reset(9)
	active, _ := svc.IsAccountInCooldown(9)
	if active {
		t.Fatalf("Reset did not clear cooldown")
	}
}

func TestLRU_BoundedSize(t *testing.T) {
	svc := NewWithMax(3)
	clock := &atomic.Int64{}
	svc.now = func() time.Time { return time.Unix(0, clock.Load()) }
	setClock(clock, time.Unix(1, 0))

	for id := int64(1); id <= 10; id++ {
		svc.RecordRateLimitHit(id, time.Hour)
	}
	if got := svc.Size(); got > 3 {
		t.Fatalf("Size = %d, want ≤ 3", got)
	}
}

func TestConcurrent_NoRaceOrPanic(t *testing.T) {
	svc := New()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				svc.RecordRateLimitHit(id, time.Second)
				svc.IsAccountInCooldown(id)
			}
		}(int64(i%5 + 1))
	}
	wg.Wait()
}

func TestZeroAccountID_NoOp(t *testing.T) {
	svc := New()
	svc.RecordRateLimitHit(0, time.Second)
	svc.RecordRateLimitHit(-1, time.Second)
	active, _ := svc.IsAccountInCooldown(0)
	if active {
		t.Fatalf("zero ID should never be in cooldown")
	}
}
