package circuitbreaker

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// helper: create a breaker with short timeouts for testing.
func testBreaker(opts ...func(*Config)) *Breaker {
	cfg := Config{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		Timeout:             50 * time.Millisecond,
		MaxTimeout:          400 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	}
	for _, o := range opts {
		o(&cfg)
	}
	return New(cfg)
}

// helper: call Allow and immediately record a result.
func fire(t *testing.T, b *Breaker, success bool) error {
	t.Helper()
	done, err := b.Allow()
	if err != nil {
		return err
	}
	done(success)
	return nil
}

// ---------------------------------------------------------------------------
// 1. Closed state allows requests
// ---------------------------------------------------------------------------

func TestClosedStateAllowsRequests(t *testing.T) {
	b := testBreaker()

	for i := 0; i < 10; i++ {
		done, err := b.Allow()
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		done(true)
	}

	if s := b.State(); s != StateClosed {
		t.Fatalf("expected StateClosed, got %s", s)
	}
}

// ---------------------------------------------------------------------------
// 2. Opens after failure threshold
// ---------------------------------------------------------------------------

func TestOpensAfterFailureThreshold(t *testing.T) {
	b := testBreaker() // threshold = 3

	for i := 0; i < 3; i++ {
		if err := fire(t, b, false); err != nil {
			t.Fatalf("failure %d rejected unexpectedly: %v", i, err)
		}
	}

	if s := b.State(); s != StateOpen {
		t.Fatalf("expected StateOpen after %d failures, got %s", 3, s)
	}
}

func TestDoesNotOpenBelowThreshold(t *testing.T) {
	b := testBreaker() // threshold = 3

	// Two failures should not trip the breaker.
	for i := 0; i < 2; i++ {
		fire(t, b, false)
	}
	if s := b.State(); s != StateClosed {
		t.Fatalf("expected StateClosed with only 2 failures, got %s", s)
	}
}

func TestInterleavedSuccessResetsConsecutiveFailures(t *testing.T) {
	b := testBreaker() // threshold = 3

	fire(t, b, false)
	fire(t, b, false)
	fire(t, b, true) // resets consecutive failures
	fire(t, b, false)
	fire(t, b, false)

	if s := b.State(); s != StateClosed {
		t.Fatalf("expected StateClosed because success broke the streak, got %s", s)
	}
}

// ---------------------------------------------------------------------------
// 3. Rejects when open
// ---------------------------------------------------------------------------

func TestRejectsWhenOpen(t *testing.T) {
	b := testBreaker()

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	_, err := b.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 4. Transitions to half-open after timeout
// ---------------------------------------------------------------------------

func TestTransitionsToHalfOpenAfterTimeout(t *testing.T) {
	b := testBreaker() // timeout = 50ms

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	// Wait for the timeout to elapse.
	time.Sleep(60 * time.Millisecond)

	done, err := b.Allow()
	if err != nil {
		t.Fatalf("expected request allowed after timeout, got %v", err)
	}

	if s := b.State(); s != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %s", s)
	}

	// Clean up: record the result so locks are released.
	done(true)
}

// ---------------------------------------------------------------------------
// 5. Closes again after success threshold in half-open
// ---------------------------------------------------------------------------

func TestClosesAfterSuccessThresholdInHalfOpen(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.SuccessThreshold = 2
		c.MaxHalfOpenRequests = 3 // allow enough concurrent half-open
	})

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	time.Sleep(60 * time.Millisecond)

	// First success in half-open.
	if err := fire(t, b, true); err != nil {
		t.Fatalf("half-open request 1: %v", err)
	}

	// Second success should close the circuit.
	if err := fire(t, b, true); err != nil {
		t.Fatalf("half-open request 2: %v", err)
	}

	if s := b.State(); s != StateClosed {
		t.Fatalf("expected StateClosed after success threshold, got %s", s)
	}
}

// ---------------------------------------------------------------------------
// 6. Re-opens on failure in half-open with doubled timeout
// ---------------------------------------------------------------------------

func TestReopensOnHalfOpenFailureWithBackoff(t *testing.T) {
	b := testBreaker() // timeout=50ms, maxTimeout=400ms

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	// Wait, enter half-open, then fail: timeout should double to 100ms.
	time.Sleep(60 * time.Millisecond)
	fire(t, b, false)

	if s := b.State(); s != StateOpen {
		t.Fatalf("expected StateOpen after half-open failure, got %s", s)
	}

	// Should still be open after 60ms (timeout is now 100ms).
	time.Sleep(60 * time.Millisecond)
	_, err := b.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected circuit to still be open at 60ms into 100ms timeout, got %v", err)
	}

	// After another 50ms (total ~110ms), should transition to half-open.
	time.Sleep(50 * time.Millisecond)
	done, err := b.Allow()
	if err != nil {
		t.Fatalf("expected half-open after doubled timeout, got %v", err)
	}
	done(true)
}

func TestBackoffCapsAtMaxTimeout(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.Timeout = 10 * time.Millisecond
		c.MaxTimeout = 20 * time.Millisecond
	})

	// Trip the breaker (closed -> open, timeout = 10ms).
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	// First backoff: 10ms -> 20ms (capped at max).
	time.Sleep(15 * time.Millisecond)
	fire(t, b, false) // half-open fail -> re-open, timeout = min(20,20) = 20ms

	// Second backoff: would be 40ms but capped at 20ms.
	time.Sleep(25 * time.Millisecond)
	fire(t, b, false) // half-open fail -> re-open, timeout = min(40,20) = 20ms

	// Verify it opens at 20ms, not 40ms.
	time.Sleep(25 * time.Millisecond)
	done, err := b.Allow()
	if err != nil {
		t.Fatalf("expected half-open after capped timeout of 20ms, got %v", err)
	}
	done(true)
}

// ---------------------------------------------------------------------------
// 7. MaxHalfOpenRequests enforcement
// ---------------------------------------------------------------------------

func TestMaxHalfOpenRequestsEnforcement(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.MaxHalfOpenRequests = 2
	})

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	time.Sleep(60 * time.Millisecond)

	// First request transitions open -> half-open (inflight = 1).
	done1, err := b.Allow()
	if err != nil {
		t.Fatalf("first half-open request rejected: %v", err)
	}

	// Second request should also be allowed (inflight = 2, max = 2).
	done2, err := b.Allow()
	if err != nil {
		t.Fatalf("second half-open request rejected: %v", err)
	}

	// Third should be rejected.
	_, err = b.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen when MaxHalfOpenRequests exceeded, got %v", err)
	}

	// Complete one; next request should be allowed again.
	done1(true)
	done3, err := b.Allow()
	if err != nil {
		t.Fatalf("request after completing one should be allowed: %v", err)
	}

	done2(true)
	done3(true)
}

func TestMaxHalfOpenRequestsDefaultsToOne(t *testing.T) {
	b := testBreaker() // MaxHalfOpenRequests = 1

	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	time.Sleep(60 * time.Millisecond)

	done, err := b.Allow()
	if err != nil {
		t.Fatalf("first half-open request rejected: %v", err)
	}

	_, err = b.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected second half-open request rejected with default max=1, got %v", err)
	}

	done(true)
}

// ---------------------------------------------------------------------------
// 8. Reset works
// ---------------------------------------------------------------------------

func TestResetFromOpen(t *testing.T) {
	b := testBreaker()

	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}
	if s := b.State(); s != StateOpen {
		t.Fatalf("precondition: expected StateOpen, got %s", s)
	}

	b.Reset()

	if s := b.State(); s != StateClosed {
		t.Fatalf("expected StateClosed after reset, got %s", s)
	}

	// Should accept requests immediately.
	if err := fire(t, b, true); err != nil {
		t.Fatalf("request after reset rejected: %v", err)
	}
}

func TestResetClearsBackoff(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.Timeout = 10 * time.Millisecond
	})

	// Trip and back off once.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}
	time.Sleep(15 * time.Millisecond)
	fire(t, b, false) // re-trip from half-open, timeout now 20ms

	b.Reset()

	// Trip again; timeout should be back to the original 10ms, not 20ms.
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	time.Sleep(15 * time.Millisecond)
	done, err := b.Allow()
	if err != nil {
		t.Fatalf("expected half-open at original timeout after reset, got %v", err)
	}
	done(true)
}

// ---------------------------------------------------------------------------
// 9. OnStateChange callback fires
// ---------------------------------------------------------------------------

func TestOnStateChangeCallback(t *testing.T) {
	type transition struct {
		from, to State
	}
	var mu sync.Mutex
	var transitions []transition

	b := testBreaker(func(c *Config) {
		c.OnStateChange = func(from, to State) {
			mu.Lock()
			transitions = append(transitions, transition{from, to})
			mu.Unlock()
		}
	})

	// Closed -> Open (3 failures).
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	// Open -> HalfOpen (wait timeout, allow request).
	time.Sleep(60 * time.Millisecond)
	done, err := b.Allow()
	if err != nil {
		t.Fatal(err)
	}

	// HalfOpen -> Open (fail).
	done(false)

	// Open -> HalfOpen -> Closed (wait, succeed twice).
	time.Sleep(110 * time.Millisecond) // doubled timeout = 100ms
	fire(t, b, true)
	fire(t, b, true)

	mu.Lock()
	defer mu.Unlock()

	expected := []transition{
		{StateClosed, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateOpen},
		{StateOpen, StateHalfOpen},
		{StateHalfOpen, StateClosed},
	}

	if len(transitions) != len(expected) {
		t.Fatalf("expected %d transitions, got %d: %+v", len(expected), len(transitions), transitions)
	}
	for i, want := range expected {
		if transitions[i] != want {
			t.Fatalf("transition %d: expected %+v, got %+v", i, want, transitions[i])
		}
	}
}

func TestNoCallbackOnSameState(t *testing.T) {
	var calls int
	b := testBreaker(func(c *Config) {
		c.OnStateChange = func(_, _ State) { calls++ }
	})

	// Staying in closed should not fire the callback.
	fire(t, b, true)
	fire(t, b, true)

	if calls != 0 {
		t.Fatalf("expected 0 callback calls for same-state, got %d", calls)
	}
}

// ---------------------------------------------------------------------------
// 10. Counts are accurate
// ---------------------------------------------------------------------------

func TestCountsAccuracy(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.FailureThreshold = 100 // keep closed so we can fire many requests
	})

	for i := 0; i < 5; i++ {
		fire(t, b, true)
	}
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}
	for i := 0; i < 2; i++ {
		fire(t, b, true)
	}

	c := b.Counts()
	if c.Requests != 10 {
		t.Fatalf("expected 10 requests, got %d", c.Requests)
	}
	if c.TotalSuccesses != 7 {
		t.Fatalf("expected 7 total successes, got %d", c.TotalSuccesses)
	}
	if c.TotalFailures != 3 {
		t.Fatalf("expected 3 total failures, got %d", c.TotalFailures)
	}
	if c.ConsecutiveSuccesses != 2 {
		t.Fatalf("expected 2 consecutive successes, got %d", c.ConsecutiveSuccesses)
	}
	if c.ConsecutiveFailures != 0 {
		t.Fatalf("expected 0 consecutive failures, got %d", c.ConsecutiveFailures)
	}
}

func TestCountsAfterRejection(t *testing.T) {
	b := testBreaker()

	// Trip the breaker (3 failures).
	for i := 0; i < 3; i++ {
		fire(t, b, false)
	}

	// Rejected request still increments the request counter.
	_, err := b.Allow()
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatal("expected rejection")
	}

	c := b.Counts()
	if c.Requests != 4 {
		t.Fatalf("expected 4 requests (3 allowed + 1 rejected), got %d", c.Requests)
	}
	if c.TotalFailures != 3 {
		t.Fatalf("expected 3 total failures (rejected calls don't count as failures), got %d", c.TotalFailures)
	}
}

// ---------------------------------------------------------------------------
// 11. Concurrent safety
// ---------------------------------------------------------------------------

func TestConcurrentSafety(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.FailureThreshold = 50
		c.SuccessThreshold = 5
		c.MaxHalfOpenRequests = 10
		c.Timeout = 5 * time.Millisecond
	})

	const goroutines = 20
	const iterations = 200

	var wg sync.WaitGroup
	var allowedCount atomic.Int64
	var rejectedCount atomic.Int64

	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				done, err := b.Allow()
				if err != nil {
					rejectedCount.Add(1)
					continue
				}
				allowedCount.Add(1)
				// Alternate success/failure to exercise transitions.
				done(i%3 != 0)
			}
		}(g)
	}

	wg.Wait()

	total := allowedCount.Load() + rejectedCount.Load()
	if total != goroutines*iterations {
		t.Fatalf("expected %d total operations, got %d", goroutines*iterations, total)
	}

	c := b.Counts()
	if c.Requests != int64(goroutines*iterations) {
		t.Fatalf("expected %d requests, got %d", goroutines*iterations, c.Requests)
	}
	if c.TotalSuccesses+c.TotalFailures != allowedCount.Load() {
		t.Fatalf("successes (%d) + failures (%d) = %d, expected %d (allowed count)",
			c.TotalSuccesses, c.TotalFailures,
			c.TotalSuccesses+c.TotalFailures, allowedCount.Load())
	}
}

func TestConcurrentResetDuringTraffic(t *testing.T) {
	b := testBreaker(func(c *Config) {
		c.FailureThreshold = 5
		c.Timeout = 2 * time.Millisecond
	})

	var wg sync.WaitGroup

	// Goroutines firing requests.
	wg.Add(5)
	for i := 0; i < 5; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				done, err := b.Allow()
				if err != nil {
					continue
				}
				done(j%2 == 0)
			}
		}()
	}

	// Goroutine periodically resetting.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			time.Sleep(time.Millisecond)
			b.Reset()
		}
	}()

	wg.Wait()

	// The test passes if no panic or data race occurred. Verify the breaker
	// is in a consistent state after reset.
	s := b.State()
	if s != StateClosed && s != StateOpen && s != StateHalfOpen {
		t.Fatalf("unexpected state %d", s)
	}
}

// ---------------------------------------------------------------------------
// Config defaults
// ---------------------------------------------------------------------------

func TestConfigDefaults(t *testing.T) {
	b := New(Config{})

	// Verify defaults were applied by checking that the breaker opens
	// after exactly 5 failures (the default FailureThreshold).
	for i := 0; i < 4; i++ {
		fire(t, b, false)
	}
	if s := b.State(); s != StateClosed {
		t.Fatalf("expected StateClosed after 4 failures with default threshold of 5, got %s", s)
	}
	fire(t, b, false)
	if s := b.State(); s != StateOpen {
		t.Fatalf("expected StateOpen after 5 failures with default threshold, got %s", s)
	}
}

// ---------------------------------------------------------------------------
// State.String()
// ---------------------------------------------------------------------------

func TestStateString(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}
