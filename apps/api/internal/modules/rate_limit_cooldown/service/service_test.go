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

	active, unblock := svc.IsAccountInCooldown(42, "")
	if active {
		t.Fatalf("fresh account should not be in cooldown, got unblock=%v", unblock)
	}

	svc.RecordRateLimitHit(42, "", 30*time.Second)
	active, unblock = svc.IsAccountInCooldown(42, "")
	if !active {
		t.Fatalf("after RecordRateLimitHit account should be in cooldown")
	}
	if !unblock.After(time.Unix(1029, 0)) {
		t.Fatalf("unblock too early: %v", unblock)
	}

	// Advance past the cooldown window.
	setClock(clock, time.Unix(1100, 0))
	active, unblock = svc.IsAccountInCooldown(42, "")
	if active {
		t.Fatalf("cooldown should have lapsed; unblock=%v", unblock)
	}
}

// TestPerModelIsolation is the new contract guarantee: a 429 on one
// model must not block a different model on the same account.
func TestPerModelIsolation_PerModelDoesNotBlockSiblingModels(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(4000, 0))

	svc.RecordRateLimitHit(11, "gemini-2.5-pro", 30*time.Second)
	if active, _ := svc.IsAccountInCooldown(11, "gemini-2.5-pro"); !active {
		t.Fatal("expected gemini-2.5-pro to be cooled")
	}
	if active, _ := svc.IsAccountInCooldown(11, "gemini-2.5-flash"); active {
		t.Fatal("sibling model must NOT be blocked by per-model cooldown")
	}
}

// Account-wide cooldown (model=="") still wins over every per-model check.
func TestAccountWideCooldown_BlocksEveryModel(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(4100, 0))

	svc.RecordRateLimitHit(12, "", 30*time.Second)
	for _, model := range []string{"", "gemini-2.5-pro", "gpt-4o", "claude-sonnet-4-6"} {
		if active, _ := svc.IsAccountInCooldown(12, model); !active {
			t.Fatalf("account-wide cooldown must block model=%q", model)
		}
	}
}

// Per-model and account-wide records may coexist; IsAccountInCooldown
// returns the latest unblock time so the caller waits the strictest.
func TestPerModelAndAccountWide_ReturnsLaterUnblock(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(4200, 0))

	svc.RecordRateLimitHit(13, "gpt-4o", 5*time.Second)
	svc.RecordRateLimitHit(13, "", 60*time.Second)
	active, unblock := svc.IsAccountInCooldown(13, "gpt-4o")
	if !active {
		t.Fatal("expected cooldown")
	}
	if got := unblock.Sub(time.Unix(4200, 0)); got < 60*time.Second {
		t.Fatalf("unblock should reflect the longer (account-wide) window, got %v", got)
	}
}

func TestRecord_ClampToMinCooldown(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(2000, 0))

	svc.RecordRateLimitHit(1, "", 0) // < minCooldown
	active, unblock := svc.IsAccountInCooldown(1, "")
	if !active {
		t.Fatalf("expected cooldown")
	}
	if unblock.Before(time.Unix(2000, 0).Add(minCooldown - time.Nanosecond)) {
		t.Fatalf("unblock not clamped: %v", unblock)
	}
}

func TestRecord_ClampToMaxCooldown(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(3000, 0))

	svc.RecordRateLimitHit(2, "", 100*time.Hour) // > maxCooldown
	_, unblock := svc.IsAccountInCooldown(2, "")
	if got := unblock.Sub(time.Unix(3000, 0)); got > maxCooldown {
		t.Fatalf("unblock %v exceeds maxCooldown %v", got, maxCooldown)
	}
}

func TestConsecutiveDisable_EscalatesToTempDisable_PerModel(t *testing.T) {
	svc, clock := newTestService(t)
	t0 := time.Unix(10_000, 0)
	setClock(clock, t0)

	for i := 0; i < consecutiveDisableThreshold; i++ {
		setClock(clock, t0.Add(time.Duration(i)*time.Second))
		svc.RecordRateLimitHit(7, "model-a", 5*time.Second)
	}
	if active, _ := svc.IsAccountInCooldown(7, "model-a"); !active {
		t.Fatal("expected model-a temp-disable")
	}
	// A different model must remain serviceable — the escalation is
	// scoped to (accountID, model).
	if active, _ := svc.IsAccountInCooldown(7, "model-b"); active {
		t.Fatal("escalation on model-a must not bleed into model-b")
	}
}

func TestConsecutiveDisable_ExponentialBackoff(t *testing.T) {
	svc, clock := newTestService(t)
	t0 := time.Unix(30_000, 0)

	// Record exactly consecutiveDisableThreshold hits (5) → base cooldown (10 min).
	for i := 0; i < consecutiveDisableThreshold; i++ {
		setClock(clock, t0.Add(time.Duration(i)*time.Second))
		svc.RecordRateLimitHit(50, "model-exp", 1*time.Second)
	}
	active, unblock := svc.IsAccountInCooldown(50, "model-exp")
	if !active {
		t.Fatal("expected temp-disable at threshold")
	}
	dur5 := unblock.Sub(t0.Add(time.Duration(consecutiveDisableThreshold-1) * time.Second))
	if dur5 != baseCooldown {
		t.Fatalf("5 hits: want %v, got %v", baseCooldown, dur5)
	}

	// 6th hit → 20 min.
	setClock(clock, t0.Add(time.Duration(consecutiveDisableThreshold)*time.Second))
	svc.RecordRateLimitHit(50, "model-exp", 1*time.Second)
	_, unblock = svc.IsAccountInCooldown(50, "model-exp")
	dur6 := unblock.Sub(t0.Add(time.Duration(consecutiveDisableThreshold) * time.Second))
	if dur6 != 2*baseCooldown {
		t.Fatalf("6 hits: want %v, got %v", 2*baseCooldown, dur6)
	}

	// 7th hit → 30 min (cap).
	setClock(clock, t0.Add(time.Duration(consecutiveDisableThreshold+1)*time.Second))
	svc.RecordRateLimitHit(50, "model-exp", 1*time.Second)
	_, unblock = svc.IsAccountInCooldown(50, "model-exp")
	dur7 := unblock.Sub(t0.Add(time.Duration(consecutiveDisableThreshold+1) * time.Second))
	if dur7 != maxDisableCooldown {
		t.Fatalf("7 hits: want %v, got %v", maxDisableCooldown, dur7)
	}

	// 8th hit → still 30 min (cap).
	setClock(clock, t0.Add(time.Duration(consecutiveDisableThreshold+2)*time.Second))
	svc.RecordRateLimitHit(50, "model-exp", 1*time.Second)
	_, unblock = svc.IsAccountInCooldown(50, "model-exp")
	dur8 := unblock.Sub(t0.Add(time.Duration(consecutiveDisableThreshold+2) * time.Second))
	if dur8 != maxDisableCooldown {
		t.Fatalf("8 hits: want %v (cap), got %v", maxDisableCooldown, dur8)
	}
}

func TestEscalatedCooldown_Values(t *testing.T) {
	tests := []struct {
		hits int
		want time.Duration
	}{
		{4, baseCooldown},           // below threshold, exponent clamped to 0
		{5, baseCooldown},           // 2^0 = 1x
		{6, 2 * baseCooldown},      // 2^1 = 2x
		{7, maxDisableCooldown},     // 2^2 = 4x but capped at 30 min
		{8, maxDisableCooldown},     // still capped
		{20, maxDisableCooldown},    // large value, still capped
	}
	for _, tt := range tests {
		got := escalatedCooldown(tt.hits)
		if got != tt.want {
			t.Errorf("escalatedCooldown(%d) = %v, want %v", tt.hits, got, tt.want)
		}
	}
}

func TestConsecutiveDisable_WindowSlidesOff(t *testing.T) {
	svc, clock := newTestService(t)
	t0 := time.Unix(20_000, 0)
	setClock(clock, t0)

	svc.RecordRateLimitHit(8, "", time.Second)
	setClock(clock, t0.Add(consecutiveWindow+time.Minute))
	for i := 0; i < consecutiveDisableThreshold-1; i++ {
		svc.RecordRateLimitHit(8, "", time.Second)
	}
	setClock(clock, t0.Add(consecutiveWindow+time.Minute+2*time.Second))
	active, _ := svc.IsAccountInCooldown(8, "")
	if active {
		svc.mu.Lock()
		elem := svc.entries[Key{AccountID: 8}]
		entry := elem.Value.(*cooldownEntry)
		hits := len(entry.hits)
		svc.mu.Unlock()
		if hits >= consecutiveDisableThreshold {
			t.Fatalf("stale hits not slid off: hits=%d", hits)
		}
	}
}

func TestFilterCooldownedAccounts_RespectsModel(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(5000, 0))

	svc.RecordRateLimitHit(1, "model-x", 10*time.Second)
	svc.RecordRateLimitHit(3, "", 10*time.Second) // account-wide

	gotModelX := svc.FilterCooldownedAccounts([]int64{1, 2, 3, 4}, "model-x")
	if len(gotModelX) != 2 || gotModelX[0] != 1 || gotModelX[1] != 3 {
		t.Fatalf("model-x filter = %v, want [1 3]", gotModelX)
	}
	// model-y is only blocked by account-wide entries; per-model X
	// must not appear.
	gotModelY := svc.FilterCooldownedAccounts([]int64{1, 2, 3, 4}, "model-y")
	if len(gotModelY) != 1 || gotModelY[0] != 3 {
		t.Fatalf("model-y filter = %v, want [3]", gotModelY)
	}
}

func TestCooldownedIDs_ScopedToModel(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(5500, 0))

	svc.RecordRateLimitHit(21, "model-a", 10*time.Second)
	svc.RecordRateLimitHit(22, "", 10*time.Second)
	svc.RecordRateLimitHit(23, "model-b", 10*time.Second)

	idsA := svc.CooldownedIDs("model-a")
	if !containsInt64(idsA, 21) || !containsInt64(idsA, 22) || containsInt64(idsA, 23) {
		t.Fatalf("CooldownedIDs(model-a) = %v, expected 21+22, never 23", idsA)
	}
}

func TestReset_ClearsEntry(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(7000, 0))

	svc.RecordRateLimitHit(9, "model-x", 30*time.Second)
	svc.Reset(9, "model-x")
	if active, _ := svc.IsAccountInCooldown(9, "model-x"); active {
		t.Fatalf("Reset did not clear cooldown")
	}
}

func TestResetAccount_ClearsEveryModel(t *testing.T) {
	svc, clock := newTestService(t)
	setClock(clock, time.Unix(7100, 0))

	svc.RecordRateLimitHit(15, "m1", 30*time.Second)
	svc.RecordRateLimitHit(15, "m2", 30*time.Second)
	svc.RecordRateLimitHit(15, "", 30*time.Second)
	svc.ResetAccount(15)
	for _, m := range []string{"", "m1", "m2"} {
		if active, _ := svc.IsAccountInCooldown(15, m); active {
			t.Fatalf("ResetAccount did not clear %q", m)
		}
	}
}

func TestLRU_BoundedSize(t *testing.T) {
	svc := NewWithMax(3)
	clock := &atomic.Int64{}
	svc.now = func() time.Time { return time.Unix(0, clock.Load()) }
	setClock(clock, time.Unix(1, 0))

	for id := int64(1); id <= 10; id++ {
		svc.RecordRateLimitHit(id, "", time.Hour)
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
				svc.RecordRateLimitHit(id, "model-x", time.Second)
				svc.IsAccountInCooldown(id, "model-x")
			}
		}(int64(i%5 + 1))
	}
	wg.Wait()
}

func TestZeroAccountID_NoOp(t *testing.T) {
	svc := New()
	svc.RecordRateLimitHit(0, "", time.Second)
	svc.RecordRateLimitHit(-1, "", time.Second)
	if active, _ := svc.IsAccountInCooldown(0, ""); active {
		t.Fatalf("zero ID should never be in cooldown")
	}
}

func containsInt64(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
