package balancereservation

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newStore(t *testing.T) (*Store, *miniredis.Miniredis, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store := New(client, "test:bres", time.Minute)
	return store, server, func() { _ = client.Close() }
}

func TestReserveAdmitsWhenBalanceCovers(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	ok, err := store.Reserve(ctx, 7, "req_a:attempt:1", "1.00000000", "0.30000000", 0)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if !ok {
		t.Fatalf("expected reservation accepted, balance 1.0 covers 0.3")
	}
	reserved, err := store.Reserved(ctx, 7)
	if err != nil {
		t.Fatalf("Reserved: %v", err)
	}
	if reserved != "0.30000000" {
		t.Fatalf("reserved = %q, want 0.30000000", reserved)
	}
}

// The headline race: a $1 user fires N concurrent requests, each estimated at
// $0.30. Before this change the gate would admit all N (each sees balance=1.0
// independently) and the deferred charger would later debit N*0.3 = overdraft.
// With the reservation, only floor(1.0/0.3)=3 admits succeed; the rest are
// denied at the gate.
func TestReserveDeniesOverspendUnderConcurrency(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	const n = 10
	const userID = 42
	const balance = "1.00000000"
	const amount = "0.30000000"

	results := make(chan bool, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := store.Reserve(ctx, userID, "req_burst:attempt:"+strconv.Itoa(i+1), balance, amount, 0)
			if err != nil {
				t.Errorf("Reserve %d: %v", i, err)
				return
			}
			results <- ok
		}(i)
	}
	wg.Wait()
	close(results)

	admits := 0
	for r := range results {
		if r {
			admits++
		}
	}
	// 1.0 / 0.3 = 3.33 → at most 3 admits before the 4th can't cover.
	if admits != 3 {
		t.Fatalf("admits = %d, want exactly 3 (floor(balance/amount)); race not contained", admits)
	}
	reserved, err := store.Reserved(ctx, userID)
	if err != nil {
		t.Fatalf("Reserved: %v", err)
	}
	if reserved != "0.90000000" {
		t.Fatalf("reserved = %q, want 0.90000000 (3 × 0.3)", reserved)
	}
}

// Same idempotency key is intentionally a no-op on the second call so a
// retried gate doesn't double-reserve. The reserved total must not grow.
func TestReserveIsIdempotentOnSameKey(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		ok, err := store.Reserve(ctx, 9, "req_idem:attempt:1", "1.00000000", "0.40000000", 0)
		if err != nil {
			t.Fatalf("Reserve iter %d: %v", i, err)
		}
		if !ok {
			t.Fatalf("idempotent re-reserve should succeed (iter %d)", i)
		}
	}
	reserved, err := store.Reserved(ctx, 9)
	if err != nil {
		t.Fatalf("Reserved: %v", err)
	}
	if reserved != "0.40000000" {
		t.Fatalf("reserved = %q, want 0.40000000 (idempotent)", reserved)
	}
}

func TestReleaseReturnsCapacity(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	if ok, err := store.Reserve(ctx, 1, "req_x:attempt:1", "1.00000000", "0.90000000", 0); err != nil || !ok {
		t.Fatalf("initial reserve: %v %v", ok, err)
	}
	// Second reserve denied — only 0.10 left.
	if ok, _ := store.Reserve(ctx, 1, "req_y:attempt:1", "1.00000000", "0.20000000", 0); ok {
		t.Fatalf("second reserve should have been denied before release")
	}
	if err := store.Release(ctx, 1, "req_x:attempt:1"); err != nil {
		t.Fatalf("release: %v", err)
	}
	if ok, err := store.Reserve(ctx, 1, "req_y:attempt:1", "1.00000000", "0.20000000", 0); err != nil || !ok {
		t.Fatalf("post-release reserve: %v %v", ok, err)
	}
}

// Releasing a key that was never reserved (e.g. usage record fires for a
// request that never hit the reservation path) must not error or corrupt the
// counter for other reservations.
func TestReleaseIdempotentOnMissingKey(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	if err := store.Release(context.Background(), 11, "req_never_reserved:attempt:1"); err != nil {
		t.Fatalf("release on missing key: %v", err)
	}
	// Existing reservation should be untouched.
	if _, err := store.Reserve(ctx, 11, "req_real:attempt:1", "1.00000000", "0.50000000", 0); err != nil {
		t.Fatalf("reserve after stray release: %v", err)
	}
	if err := store.Release(ctx, 11, "req_real:attempt:1"); err != nil {
		t.Fatalf("release real: %v", err)
	}
	if err := store.Release(ctx, 11, "req_real:attempt:1"); err != nil {
		t.Fatalf("second release on already-released key: %v", err)
	}
	reserved, _ := store.Reserved(ctx, 11)
	if reserved != "0.00000000" {
		t.Fatalf("reserved after release = %q, want 0.00000000", reserved)
	}
}

// A zero-cost request (cached / billable-cost-zero) shouldn't burn a hash
// slot — the gate admits without taking a reservation.
func TestReserveZeroAmountIsNoOp(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	ok, err := store.Reserve(ctx, 3, "req_free:attempt:1", "1.00000000", "0.00000000", 0)
	if err != nil || !ok {
		t.Fatalf("zero-amount reserve: ok=%v err=%v", ok, err)
	}
	reserved, _ := store.Reserved(ctx, 3)
	if reserved != "0.00000000" {
		t.Fatalf("reserved = %q, want 0 — zero-amount must not consume", reserved)
	}
}

// TTL bounds leaked reservations from poisoning a user's available balance
// if a release is somehow missed (panic, OOM, etc.).
func TestReservationExpiresAfterTTL(t *testing.T) {
	store, server, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	if ok, err := store.Reserve(ctx, 5, "req_leak:attempt:1", "1.00000000", "0.90000000", 2*time.Second); err != nil || !ok {
		t.Fatalf("reserve: %v %v", ok, err)
	}
	server.FastForward(3 * time.Second)
	reserved, err := store.Reserved(ctx, 5)
	if err != nil {
		t.Fatalf("Reserved after TTL: %v", err)
	}
	if reserved != "0.00000000" {
		t.Fatalf("reserved after TTL = %q, want 0 — TTL must reclaim leaked reservations", reserved)
	}
}

// Zero / negative balance must always deny, even when amount is zero.
func TestReserveDeniesZeroBalance(t *testing.T) {
	store, _, cleanup := newStore(t)
	defer cleanup()
	ctx := context.Background()

	if ok, _ := store.Reserve(context.Background(), 8, "req_z:attempt:1", "0.00000000", "0.10000000", 0); ok {
		t.Fatalf("expected denial for zero balance")
	}
	if ok, _ := store.Reserve(context.Background(), 8, "req_n:attempt:1", "-0.00000001", "0.10000000", 0); ok {
		t.Fatalf("expected denial for negative balance")
	}
	_ = ctx
}

// Smoke-test the lossless decimal <-> ticks conversion at the boundaries we
// actually hit in production.
func TestDecimalTicksRoundtrip(t *testing.T) {
	cases := []string{
		"0.00000000",
		"0.00000001",
		"0.12345678",
		"1.00000000",
		"100.50000000",
		"999999.99999999",
	}
	for _, c := range cases {
		ticks, err := decimalToTicks(c)
		if err != nil {
			t.Fatalf("decimalToTicks(%q): %v", c, err)
		}
		got := ticksToDecimal(ticks)
		if got != c {
			t.Fatalf("roundtrip %q -> %q -> %q", c, ticks.String(), got)
		}
	}
}
