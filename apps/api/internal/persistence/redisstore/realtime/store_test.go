package realtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
	realtimeservice "github.com/srapi/srapi/apps/api/internal/modules/realtime/service"
)

func TestRedisRealtimeStoreEnforcesDistributedSlotLimits(t *testing.T) {
	first, second, closeClient := newPair(t)
	defer closeClient()

	ctx := context.Background()
	acquiredAt := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if _, err := first.AcquireSlot(ctx, preparedSlot("slot_1", 10, acquiredAt, limits(1, 10))); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if _, err := second.AcquireSlot(ctx, preparedSlot("slot_2", 11, acquiredAt.Add(time.Second), limits(1, 10))); !errors.Is(err, realtimeservice.ErrLimitExceeded) {
		t.Fatalf("expected second instance to see global limit, got %v", err)
	}

	snapshot, err := first.Snapshot(ctx, acquiredAt.Add(2*time.Second))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.ActiveSlots != 1 || snapshot.AcquiredTotal != 1 || snapshot.RejectedTotal != 1 {
		t.Fatalf("unexpected distributed snapshot: %+v", snapshot)
	}
}

func TestRedisRealtimeStoreReleaseFromAnotherInstanceFreesCapacity(t *testing.T) {
	first, second, closeClient := newPair(t)
	defer closeClient()

	ctx := context.Background()
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	slot, err := first.AcquireSlot(ctx, preparedSlot("slot_release", 10, now, limits(1, 1)))
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	released, err := second.ReleaseSlot(ctx, slot.ID, now.Add(time.Second))
	if err != nil {
		t.Fatalf("release from second instance: %v", err)
	}
	if released.ReleasedAt == nil || !released.ReleasedAt.Equal(now.Add(time.Second)) {
		t.Fatalf("unexpected released slot: %+v", released)
	}
	if _, err := second.AcquireSlot(ctx, preparedSlot("slot_after_release", 10, now.Add(2*time.Second), limits(1, 1))); err != nil {
		t.Fatalf("expected released capacity to be free: %v", err)
	}
	snapshot, err := second.Snapshot(ctx, now.Add(3*time.Second))
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.ActiveSlots != 1 || snapshot.AcquiredTotal != 2 || snapshot.ReleasedTotal != 1 {
		t.Fatalf("unexpected snapshot after cross-instance release: %+v", snapshot)
	}
}

func TestRedisRealtimeStoreExpiresSlotsWithoutLeakingSensitiveData(t *testing.T) {
	store, closeClient := newStore(t)
	defer closeClient()

	ctx := context.Background()
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	input := preparedSlot("slot_expire", 10, now, limits(10, 10))
	input.ExpireAt = now.Add(5 * time.Millisecond)
	input.Slot.SessionAffinityKeyHash = realtimeservice.AffinityHash("raw-affinity-secret")
	if _, err := store.AcquireSlot(ctx, input); err != nil {
		t.Fatalf("acquire expiring slot: %v", err)
	}
	list, err := store.ListActiveSlots(ctx, now.Add(time.Second))
	if err != nil {
		t.Fatalf("list active slots: %v", err)
	}
	if len(list.Slots) != 0 || list.Snapshot.ActiveSlots != 0 || list.Snapshot.ReleasedTotal != 1 {
		t.Fatalf("expected expired slot to be inactive and counted released, got %+v", list)
	}
	stored := flattenSlotHash(t, store, input.Slot.ID)
	if strings.Contains(stored, "raw-affinity-secret") {
		t.Fatalf("redis slot hash leaked raw affinity secret: %s", stored)
	}
}

func TestRedisRealtimeStoreListsActiveSlotsWithAggregates(t *testing.T) {
	store, closeClient := newStore(t)
	defer closeClient()

	ctx := context.Background()
	now := time.Date(2026, 5, 23, 10, 0, 0, 0, time.UTC)
	if _, err := store.AcquireSlot(ctx, preparedSlot("slot_a", 10, now, limits(10, 10))); err != nil {
		t.Fatalf("acquire a: %v", err)
	}
	second := preparedSlot("slot_b", 11, now.Add(time.Second), limits(10, 10))
	second.Slot.Kind = contract.SlotKindRealtimeWebSocket
	second.Slot.SourceEndpoint = "/v1/realtime"
	if _, err := store.AcquireSlot(ctx, second); err != nil {
		t.Fatalf("acquire b: %v", err)
	}
	list, err := store.ListActiveSlots(ctx, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(list.Slots) != 2 ||
		list.ActiveByKind[contract.SlotKindResponsesWebSocket] != 1 ||
		list.ActiveByKind[contract.SlotKindRealtimeWebSocket] != 1 ||
		list.ActiveByAPIKeyID[10] != 1 ||
		list.Snapshot.ActiveByEndpoint["/v1/realtime"] != 1 {
		t.Fatalf("unexpected active list aggregates: %+v", list)
	}
}

func newStore(t *testing.T) (*Store, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store, func() {
		_ = client.Close()
		server.Close()
	}
}

func newPair(t *testing.T) (*Store, *Store, func()) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	first, err := New(client)
	if err != nil {
		t.Fatalf("new first store: %v", err)
	}
	second, err := New(client)
	if err != nil {
		t.Fatalf("new second store: %v", err)
	}
	return first, second, func() {
		_ = client.Close()
		server.Close()
	}
}

func preparedSlot(id string, apiKeyID int, acquiredAt time.Time, slotLimits contract.SlotLimits) contract.PreparedSlot {
	return contract.PreparedSlot{
		Slot: contract.Slot{
			ID:                     id,
			Kind:                   contract.SlotKindResponsesWebSocket,
			RequestID:              "req_" + id,
			UserID:                 7,
			APIKeyID:               apiKeyID,
			SourceEndpoint:         "/v1/responses/ws",
			SessionAffinitySource:  "query:session_affinity_key",
			SessionAffinityKeyHash: realtimeservice.AffinityHash("affinity-" + id),
			StickyStrength:         "soft",
			AcquiredAt:             acquiredAt,
		},
		Limits:   slotLimits,
		ExpireAt: acquiredAt.Add(time.Minute),
	}
}

func limits(global int, perKey int) contract.SlotLimits {
	return contract.SlotLimits{MaxOpenSlots: global, MaxOpenSlotsPerKey: perKey}
}

func flattenSlotHash(t *testing.T, store *Store, slotID string) string {
	t.Helper()
	row, err := store.client.HGetAll(t.Context(), store.slotKey(slotID)).Result()
	if err != nil {
		t.Fatalf("read slot hash: %v", err)
	}
	var values []string
	for key, value := range row {
		values = append(values, key, value)
	}
	return strings.Join(values, " ")
}
