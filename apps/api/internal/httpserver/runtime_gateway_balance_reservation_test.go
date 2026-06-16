package httpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	userscontract "github.com/srapi/srapi/apps/api/internal/modules/users/contract"
)

// fakeReservationStore is an in-memory stand-in for the Redis-backed Store.
// Mirrors the production semantics: per-user reservation pool (idempotency_key
// → amount); Reserve denies if balance - sum(reserved) < amount; idempotent on
// re-reservation; Release is idempotent. Used to drive the gate from tests
// without spinning up a real Redis.
type fakeReservationStore struct {
	mu       sync.Mutex
	bookings map[int]map[string]*big.Rat // user_id → idempotency_key → amount
	failNext atomic.Bool                 // when set, Reserve returns an error to drive the fail-open path
}

func newFakeReservationStore() *fakeReservationStore {
	return &fakeReservationStore{bookings: map[int]map[string]*big.Rat{}}
}

func (f *fakeReservationStore) Reserve(_ context.Context, userID int, idempotencyKey, balance, amount string, _ time.Duration) (bool, error) {
	if f.failNext.Swap(false) {
		return false, errors.New("simulated redis outage")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	balanceRat := mustRat(balance)
	amountRat := mustRat(amount)
	if balanceRat.Sign() <= 0 {
		return false, nil
	}
	if amountRat.Sign() <= 0 {
		return true, nil
	}
	user := f.bookings[userID]
	if user == nil {
		user = map[string]*big.Rat{}
		f.bookings[userID] = user
	}
	if existing, ok := user[idempotencyKey]; ok {
		_ = existing
		return true, nil
	}
	reserved := new(big.Rat)
	for _, r := range user {
		reserved.Add(reserved, r)
	}
	available := new(big.Rat).Sub(balanceRat, reserved)
	if available.Cmp(amountRat) < 0 {
		return false, nil
	}
	user[idempotencyKey] = amountRat
	return true, nil
}

func (f *fakeReservationStore) Release(_ context.Context, userID int, idempotencyKey string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if user := f.bookings[userID]; user != nil {
		delete(user, idempotencyKey)
	}
	return nil
}

func (f *fakeReservationStore) reservedCount(userID int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.bookings[userID])
}

func mustRat(s string) *big.Rat {
	r := new(big.Rat)
	if _, ok := r.SetString(s); ok {
		return r
	}
	return new(big.Rat) // zero on parse failure — tests pass well-formed inputs
}

func newReservationTestRuntime(store balanceReservationStore) *runtimeState {
	return &runtimeState{
		cfg:                config.Config{Gateway: config.GatewayConfig{RequirePositiveBalance: true}},
		logger:             slog.New(slog.NewTextHandler(io.Discard, nil)),
		balanceReservation: store,
	}
}

func payGoEntitlement() subscriptioncontract.EntitlementDecision {
	return subscriptioncontract.EntitlementDecision{Allowed: true, Reason: "system_default"}
}

// The headline race: a $1 user fires 10 concurrent requests, each estimated
// at $0.30. With the gate wired, only floor(1.0/0.3)=3 admit; the rest are
// denied with insufficient_balance. Without the gate (or before this change)
// all 10 admit and the deferred charger drives the balance to -$2.
func TestGatewayBalanceGateContainsConcurrentOverspend(t *testing.T) {
	store := newFakeReservationStore()
	rt := newReservationTestRuntime(store)
	user := userscontract.StoredUser{User: userscontract.User{ID: 7, Balance: "1.00000000"}}
	entitlement := payGoEntitlement()
	pricing := gatewayPricingEvidence{Amount: "0.30000000"}

	const n = 10
	var admits atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			denied, err := rt.gatewayBalanceGate(context.Background(), user, entitlement, pricing, "req_burst:"+strconv.Itoa(i))
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			if !denied {
				admits.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := admits.Load(); got != 3 {
		t.Fatalf("concurrent admits = %d, want exactly 3 (floor(1.0/0.3))", got)
	}
	if got := store.reservedCount(user.ID); got != 3 {
		t.Fatalf("reserved entries = %d, want 3", got)
	}
}

// Releasing a reservation must return capacity for the next admission.
func TestGatewayBalanceGateReleaseFreesCapacity(t *testing.T) {
	store := newFakeReservationStore()
	rt := newReservationTestRuntime(store)
	user := userscontract.StoredUser{User: userscontract.User{ID: 9, Balance: "1.00000000"}}
	entitlement := payGoEntitlement()
	pricing := gatewayPricingEvidence{Amount: "0.90000000"}
	ctx := context.Background()

	// First request reserves 0.90 — leaving 0.10 available.
	if denied, err := rt.gatewayBalanceGate(ctx, user, entitlement, pricing, "req_first"); err != nil || denied {
		t.Fatalf("first gate: denied=%v err=%v", denied, err)
	}
	// Second request also wants 0.90 — denied (only 0.10 left).
	if denied, _ := rt.gatewayBalanceGate(ctx, user, entitlement, pricing, "req_second"); !denied {
		t.Fatalf("second gate should deny; insufficient available")
	}
	// Release the first.
	rt.releaseGatewayReservation(ctx, user.ID, "req_first")
	// Now the second one fits.
	if denied, err := rt.gatewayBalanceGate(ctx, user, entitlement, pricing, "req_second"); err != nil || denied {
		t.Fatalf("third gate after release: denied=%v err=%v", denied, err)
	}
}

// Replays of the same request_id must be no-ops on the reservation store —
// otherwise a client retry would deny itself by double-booking.
func TestGatewayBalanceGateIdempotentOnRetry(t *testing.T) {
	store := newFakeReservationStore()
	rt := newReservationTestRuntime(store)
	user := userscontract.StoredUser{User: userscontract.User{ID: 3, Balance: "1.00000000"}}
	entitlement := payGoEntitlement()
	pricing := gatewayPricingEvidence{Amount: "0.60000000"}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if denied, err := rt.gatewayBalanceGate(ctx, user, entitlement, pricing, "req_retry"); err != nil || denied {
			t.Fatalf("retry %d: denied=%v err=%v", i, denied, err)
		}
	}
	if got := store.reservedCount(user.ID); got != 1 {
		t.Fatalf("reserved entries = %d, want 1 (idempotent retries)", got)
	}
}

// Redis outage / store error must fail OPEN — denying every paying user is
// worse than briefly returning to the original race window. The cheap
// read-only check that ran before still catches $0-balance users.
func TestGatewayBalanceGateFailsOpenOnStoreError(t *testing.T) {
	store := newFakeReservationStore()
	store.failNext.Store(true)
	rt := newReservationTestRuntime(store)
	user := userscontract.StoredUser{User: userscontract.User{ID: 5, Balance: "1.00000000"}}
	entitlement := payGoEntitlement()
	pricing := gatewayPricingEvidence{Amount: "0.10000000"}

	denied, err := rt.gatewayBalanceGate(context.Background(), user, entitlement, pricing, "req_fail")
	if err != nil {
		t.Fatalf("err = %v, want nil (fail open)", err)
	}
	if denied {
		t.Fatalf("denied = true, want false (fail open on store error)")
	}
}

// $0 balance must always deny even before the reservation store is consulted.
func TestGatewayBalanceGateDeniesZeroBalance(t *testing.T) {
	rt := newReservationTestRuntime(newFakeReservationStore())
	user := userscontract.StoredUser{User: userscontract.User{ID: 1, Balance: "0.00000000"}}
	denied, err := rt.gatewayBalanceGate(context.Background(), user, payGoEntitlement(), gatewayPricingEvidence{Amount: "0.10000000"}, "req_zero")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !denied {
		t.Fatalf("expected denial for zero balance")
	}
}

// When no reservation store is wired (single-instance / no Redis), the gate
// degrades to the read-only check — the original behaviour. This keeps dev
// environments working without Redis.
func TestGatewayBalanceGateWithoutReservationStore(t *testing.T) {
	rt := newReservationTestRuntime(nil)
	user := userscontract.StoredUser{User: userscontract.User{ID: 1, Balance: "1.00000000"}}
	// Positive balance covers a small request even without the reservation
	// store — same as v1 of the gate.
	denied, _ := rt.gatewayBalanceGate(context.Background(), user, payGoEntitlement(), gatewayPricingEvidence{Amount: "0.10000000"}, "req_x")
	if denied {
		t.Fatalf("expected admit without reservation store (read-only gate covers)")
	}
}
