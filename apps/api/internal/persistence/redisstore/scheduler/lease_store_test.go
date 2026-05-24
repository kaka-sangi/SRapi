package scheduler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func TestRedisLeaseStorePreventsConcurrentSchedulingAndReleases(t *testing.T) {
	store, closeClient := newTestStore(t)
	defer closeClient()

	ctx := context.Background()
	maxConcurrency := 1
	first, err := store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_1_1_10",
		RequestID: "req_1",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, &maxConcurrency)
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	if first.Status != contract.LeaseStatusPending {
		t.Fatalf("expected pending first lease, got %+v", first)
	}

	_, err = store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_2_1_10",
		RequestID: "req_2",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, &maxConcurrency)
	if !errors.Is(err, ErrConcurrencyFull) {
		t.Fatalf("expected concurrency full, got %v", err)
	}

	committed, err := store.UpdateLeaseStatus(ctx, "req_1", 1, contract.LeaseStatusCommitted)
	if err != nil {
		t.Fatalf("commit first lease: %v", err)
	}
	if committed.Status != contract.LeaseStatusCommitted {
		t.Fatalf("expected committed lease, got %+v", committed)
	}

	third, err := store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_3_1_10",
		RequestID: "req_3",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, &maxConcurrency)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	if third.RequestID != "req_3" {
		t.Fatalf("unexpected third lease: %+v", third)
	}
}

func TestRedisLeaseStoreAllowsOnlyOneConcurrentAcquire(t *testing.T) {
	store, closeClient := newTestStore(t)
	defer closeClient()

	ctx := context.Background()
	maxConcurrency := 1
	const contenders = 12
	results := make(chan error, contenders)
	var wg sync.WaitGroup
	for idx := range contenders {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := store.AcquireLease(ctx, contract.Lease{
				ID:        "lease_req_concurrent_" + string(rune('a'+idx)),
				RequestID: "req_concurrent_" + string(rune('a'+idx)),
				AttemptNo: 1,
				AccountID: 10,
				ExpiresAt: time.Now().UTC().Add(time.Minute),
			}, &maxConcurrency)
			results <- err
		}(idx)
	}
	wg.Wait()
	close(results)

	successes := 0
	concurrencyFull := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConcurrencyFull):
			concurrencyFull++
		default:
			t.Fatalf("unexpected acquire error: %v", err)
		}
	}
	if successes != 1 || concurrencyFull != contenders-1 {
		t.Fatalf("expected exactly one success and %d concurrency failures, got successes=%d full=%d", contenders-1, successes, concurrencyFull)
	}
	leases, err := store.ListLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	pending := 0
	for _, lease := range leases {
		if lease.Status == contract.LeaseStatusPending {
			pending++
		}
	}
	if pending != 1 {
		t.Fatalf("expected one pending lease, got %+v", leases)
	}
}

func TestRedisLeaseStoreExpiresAndFreesConcurrency(t *testing.T) {
	store, closeClient := newTestStore(t)
	defer closeClient()

	ctx := context.Background()
	maxConcurrency := 1
	first, err := store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_expired_1_10",
		RequestID: "req_expired",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(5 * time.Millisecond),
	}, &maxConcurrency)
	if err != nil {
		t.Fatalf("acquire expiring lease: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	second, err := store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_after_expiry_1_10",
		RequestID: "req_after_expiry",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, &maxConcurrency)
	if err != nil {
		t.Fatalf("expected expired lease to free concurrency: %v", err)
	}
	if second.RequestID != "req_after_expiry" {
		t.Fatalf("unexpected second lease: %+v", second)
	}

	leases, err := store.ListLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	foundExpired := false
	for _, lease := range leases {
		if lease.ID == first.ID && lease.Status == contract.LeaseStatusExpired {
			foundExpired = true
		}
	}
	if !foundExpired {
		t.Fatalf("expected expired first lease, got %+v", leases)
	}
}

func TestRedisLeaseStoreFeedbackAfterExpiryDoesNotLeakConcurrency(t *testing.T) {
	store, closeClient := newTestStore(t)
	defer closeClient()

	ctx := context.Background()
	maxConcurrency := 1
	_, err := store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_feedback_expired_1_10",
		RequestID: "req_feedback_expired",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(5 * time.Millisecond),
	}, &maxConcurrency)
	if err != nil {
		t.Fatalf("acquire expiring lease: %v", err)
	}
	time.Sleep(20 * time.Millisecond)

	updated, err := store.UpdateLeaseStatus(ctx, "req_feedback_expired", 1, contract.LeaseStatusCommitted)
	if err != nil {
		t.Fatalf("update expired lease status: %v", err)
	}
	if updated.Status != contract.LeaseStatusExpired {
		t.Fatalf("expected expired status to be preserved, got %+v", updated)
	}

	_, err = store.AcquireLease(ctx, contract.Lease{
		ID:        "lease_req_after_feedback_expiry_1_10",
		RequestID: "req_after_feedback_expiry",
		AttemptNo: 1,
		AccountID: 10,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}, &maxConcurrency)
	if err != nil {
		t.Fatalf("expected expired feedback path to free concurrency: %v", err)
	}
}

func TestRedisLeaseStoreUpdatesAttemptScopedLease(t *testing.T) {
	store, closeClient := newTestStore(t)
	defer closeClient()

	ctx := context.Background()
	maxConcurrency := 2
	for attempt := 1; attempt <= 2; attempt++ {
		_, err := store.AcquireLease(ctx, contract.Lease{
			ID:        "lease_req_attempt_" + string(rune('0'+attempt)) + "_10",
			RequestID: "req_attempt",
			AttemptNo: attempt,
			AccountID: 10,
			ExpiresAt: time.Now().UTC().Add(time.Minute),
		}, &maxConcurrency)
		if err != nil {
			t.Fatalf("acquire attempt %d lease: %v", attempt, err)
		}
	}

	committed, err := store.UpdateLeaseStatus(ctx, "req_attempt", 1, contract.LeaseStatusCommitted)
	if err != nil {
		t.Fatalf("commit attempt 1 lease: %v", err)
	}
	if committed.AttemptNo != 1 || committed.Status != contract.LeaseStatusCommitted {
		t.Fatalf("expected attempt 1 committed, got %+v", committed)
	}
	leases, err := store.ListLeases(ctx)
	if err != nil {
		t.Fatalf("list leases: %v", err)
	}
	statusByAttempt := map[int]contract.LeaseStatus{}
	for _, lease := range leases {
		statusByAttempt[lease.AttemptNo] = lease.Status
	}
	if statusByAttempt[1] != contract.LeaseStatusCommitted || statusByAttempt[2] != contract.LeaseStatusPending {
		t.Fatalf("expected only attempt 1 released, got %+v", leases)
	}
}

func newTestStore(t *testing.T) (*Store, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := New(client)
	if err != nil {
		t.Fatalf("new redis lease store: %v", err)
	}
	return store, func() {
		_ = client.Close()
		server.Close()
	}
}
