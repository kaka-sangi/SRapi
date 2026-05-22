package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

func TestAcquireReleaseTracksRealtimeSlotLifecycle(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)}
	svc, err := New(Limits{MaxOpenSlots: 2, MaxOpenSlotsPerKey: 2}, clock)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	stickyAccountID := 9

	slot, err := svc.Acquire(context.Background(), contract.AcquireRequest{
		Kind:                  contract.SlotKindResponsesWebSocket,
		RequestID:             "req_1",
		UserID:                7,
		APIKeyID:              3,
		SourceEndpoint:        "/v1/responses/ws",
		SessionAffinityKey:    "conversation-secret",
		SessionAffinitySource: "query:session_affinity_key",
		StickyAccountID:       &stickyAccountID,
		StickyStrength:        "hard",
	})
	if err != nil {
		t.Fatalf("acquire slot: %v", err)
	}
	if slot.ID == "" || slot.ReleasedAt != nil || !slot.AcquiredAt.Equal(clock.now) {
		t.Fatalf("unexpected acquired slot: %+v", slot)
	}
	if slot.SessionAffinityKeyHash == "" || strings.Contains(slot.SessionAffinityKeyHash, "conversation-secret") {
		t.Fatalf("expected hashed affinity key, got %q", slot.SessionAffinityKeyHash)
	}
	if slot.StickyAccountID == nil || *slot.StickyAccountID != stickyAccountID || slot.StickyStrength != "hard" {
		t.Fatalf("expected sticky metadata on slot, got %+v", slot)
	}
	snapshot := svc.Snapshot(context.Background())
	if snapshot.ActiveSlots != 1 || snapshot.AcquiredTotal != 1 || snapshot.ActiveByEndpoint["/v1/responses/ws"] != 1 {
		t.Fatalf("unexpected active snapshot: %+v", snapshot)
	}

	released, err := svc.Release(context.Background(), slot.ID)
	if err != nil {
		t.Fatalf("release slot: %v", err)
	}
	if released.ReleasedAt == nil || !released.ReleasedAt.Equal(clock.now) {
		t.Fatalf("expected release timestamp, got %+v", released)
	}
	snapshot = svc.Snapshot(context.Background())
	if snapshot.ActiveSlots != 0 || snapshot.AcquiredTotal != 1 || snapshot.ReleasedTotal != 1 {
		t.Fatalf("unexpected released snapshot: %+v", snapshot)
	}
}

func TestAcquireRejectsGlobalRealtimeSlotLimit(t *testing.T) {
	svc, err := New(Limits{MaxOpenSlots: 1, MaxOpenSlotsPerKey: 10}, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Acquire(context.Background(), acquireRequest(1, 10)); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := svc.Acquire(context.Background(), acquireRequest(2, 11)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected global limit rejection, got %v", err)
	}
	snapshot := svc.Snapshot(context.Background())
	if snapshot.ActiveSlots != 1 || snapshot.RejectedTotal != 1 {
		t.Fatalf("unexpected limit snapshot: %+v", snapshot)
	}
}

func TestAcquireRejectsPerAPIKeyRealtimeSlotLimit(t *testing.T) {
	svc, err := New(Limits{MaxOpenSlots: 10, MaxOpenSlotsPerKey: 1}, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Acquire(context.Background(), acquireRequest(1, 10)); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := svc.Acquire(context.Background(), acquireRequest(2, 10)); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected per-key limit rejection, got %v", err)
	}
	if _, err := svc.Acquire(context.Background(), acquireRequest(3, 11)); err != nil {
		t.Fatalf("different api key should acquire: %v", err)
	}
	snapshot := svc.Snapshot(context.Background())
	if snapshot.ActiveSlots != 2 || snapshot.RejectedTotal != 1 {
		t.Fatalf("unexpected per-key limit snapshot: %+v", snapshot)
	}
}

func TestReleaseMissingSlotReturnsNotFound(t *testing.T) {
	svc, err := New(Limits{MaxOpenSlots: 1, MaxOpenSlotsPerKey: 1}, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Release(context.Background(), "missing"); !errors.Is(err, ErrSlotNotFound) {
		t.Fatalf("expected missing slot error, got %v", err)
	}
}

func acquireRequest(index int, apiKeyID int) contract.AcquireRequest {
	return contract.AcquireRequest{
		Kind:           contract.SlotKindResponsesWebSocket,
		RequestID:      "req_slot",
		UserID:         index,
		APIKeyID:       apiKeyID,
		SourceEndpoint: "/v1/responses/ws",
	}
}
