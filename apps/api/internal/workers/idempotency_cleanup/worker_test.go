package idempotencycleanup

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/idempotency/contract"
	memory "github.com/srapi/srapi/apps/api/internal/modules/idempotency/store/memory"
)

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestRunOnceReapsOnlyExpiredRecords(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	store := memory.New()

	if _, _, err := store.InsertOrGet(ctx, contract.BeginInput{
		Key: "expired", Method: "POST", Path: "/v1/responses", RequestHash: "h",
		LockedUntil: now, ExpiresAt: now.Add(-time.Hour), Now: now,
	}); err != nil {
		t.Fatalf("insert expired: %v", err)
	}
	if _, _, err := store.InsertOrGet(ctx, contract.BeginInput{
		Key: "fresh", Method: "POST", Path: "/v1/responses", RequestHash: "h",
		LockedUntil: now, ExpiresAt: now.Add(time.Hour), Now: now,
	}); err != nil {
		t.Fatalf("insert fresh: %v", err)
	}

	worker, err := New(store, nil, Config{Clock: fixedClock{now: now}})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	deleted, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected exactly one expired record reaped, got %d", deleted)
	}

	// The expired key is now re-insertable; the fresh key is still present.
	if reinserted, _, _ := store.InsertOrGet(ctx, contract.BeginInput{
		Key: "expired", Method: "POST", Path: "/v1/responses", RequestHash: "h",
		ExpiresAt: now.Add(time.Hour), Now: now,
	}); !reinserted {
		t.Fatalf("expected reaped record to be gone (re-insertable)")
	}
	if reinserted, _, _ := store.InsertOrGet(ctx, contract.BeginInput{
		Key: "fresh", Method: "POST", Path: "/v1/responses", RequestHash: "h",
		ExpiresAt: now.Add(time.Hour), Now: now,
	}); reinserted {
		t.Fatalf("expected fresh record to be retained")
	}
}

func TestNewRejectsNilStore(t *testing.T) {
	if _, err := New(nil, nil, Config{}); err == nil {
		t.Fatal("expected an error for a nil store")
	}
}
