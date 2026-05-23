package realtime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	realtimecontract "github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
)

var ErrInvalidStore = errors.New("invalid realtime redis slot store")

const (
	defaultSlotRetention = 5 * time.Minute
	slotKeyPrefix        = "realtime:slot:"
	activeKey            = "realtime:slots:active"
	countersKey          = "realtime:slots:counters"
)

type Store struct {
	client    *redis.Client
	retention time.Duration
}

var _ realtimecontract.Store = (*Store)(nil)

func New(client *redis.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client, retention: defaultSlotRetention}, nil
}

func (s *Store) AcquireSlot(ctx context.Context, input realtimecontract.PreparedSlot) (realtimecontract.Slot, error) {
	if s == nil || input.Slot.ID == "" || input.Slot.RequestID == "" || input.Slot.UserID <= 0 || input.Slot.APIKeyID <= 0 || input.Slot.SourceEndpoint == "" {
		return realtimecontract.Slot{}, realtimecontract.ErrInvalidInput
	}
	if input.ExpireAt.IsZero() {
		input.ExpireAt = input.Slot.AcquiredAt.Add(24 * time.Hour)
	}
	result, err := acquireSlotScript.Run(ctx, s.client, []string{
		s.slotKey(input.Slot.ID),
		activeKey,
		countersKey,
	},
		input.Slot.AcquiredAt.UnixNano(),
		input.Slot.ID,
		string(input.Slot.Kind),
		input.Slot.RequestID,
		input.Slot.UserID,
		input.Slot.APIKeyID,
		input.Slot.SourceEndpoint,
		input.Slot.SessionAffinitySource,
		input.Slot.SessionAffinityKeyHash,
		optionalIntString(input.Slot.StickyAccountID),
		input.Slot.StickyStrength,
		input.ExpireAt.UnixNano(),
		input.Limits.MaxOpenSlots,
		input.Limits.MaxOpenSlotsPerKey,
		slotKeyPrefix,
		ttlMillis(input.ExpireAt.Sub(input.Slot.AcquiredAt)+s.retention),
	).Slice()
	if err != nil {
		return realtimecontract.Slot{}, err
	}
	code, value := scriptResult(result)
	switch code {
	case "ok", "exists":
		return s.findSlotByID(ctx, value)
	case "full":
		return realtimecontract.Slot{}, realtimecontract.ErrLimitExceeded
	default:
		return realtimecontract.Slot{}, fmt.Errorf("unexpected realtime slot result: %s", code)
	}
}

func (s *Store) ReleaseSlot(ctx context.Context, slotID string, releasedAt time.Time) (realtimecontract.Slot, error) {
	if s == nil || slotID == "" {
		return realtimecontract.Slot{}, realtimecontract.ErrInvalidInput
	}
	result, err := releaseSlotScript.Run(ctx, s.client, []string{
		s.slotKey(slotID),
		activeKey,
		countersKey,
	}, releasedAt.UTC().UnixNano(), slotID, ttlMillis(s.retention)).Slice()
	if err != nil {
		return realtimecontract.Slot{}, err
	}
	code, value := scriptResult(result)
	if code != "ok" || value == "" {
		return realtimecontract.Slot{}, realtimecontract.ErrSlotNotFound
	}
	return s.findSlotByID(ctx, value)
}

func (s *Store) ListActiveSlots(ctx context.Context, now time.Time) (realtimecontract.ActiveSlotList, error) {
	if s == nil {
		return emptyActiveSlotList(), nil
	}
	if err := s.expireActiveSlots(ctx, now); err != nil {
		return realtimecontract.ActiveSlotList{}, err
	}
	ids, err := s.client.ZRangeByScore(ctx, activeKey, &redis.ZRangeBy{
		Min: strconv.FormatInt(now.UTC().UnixNano()+1, 10),
		Max: "+inf",
	}).Result()
	if err != nil {
		return realtimecontract.ActiveSlotList{}, err
	}
	slots := make([]realtimecontract.Slot, 0, len(ids))
	for _, id := range ids {
		slot, err := s.findSlotByID(ctx, id)
		if errors.Is(err, realtimecontract.ErrSlotNotFound) {
			continue
		}
		if err != nil {
			return realtimecontract.ActiveSlotList{}, err
		}
		if slot.ReleasedAt == nil {
			slots = append(slots, slot)
		}
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].AcquiredAt.Equal(slots[j].AcquiredAt) {
			return slots[i].ID < slots[j].ID
		}
		return slots[i].AcquiredAt.Before(slots[j].AcquiredAt)
	})
	snapshot, err := s.snapshotFromSlots(ctx, slots)
	if err != nil {
		return realtimecontract.ActiveSlotList{}, err
	}
	byKind := map[realtimecontract.SlotKind]int{}
	byAPIKeyID := map[int]int{}
	for _, slot := range slots {
		byKind[slot.Kind]++
		byAPIKeyID[slot.APIKeyID]++
	}
	return realtimecontract.ActiveSlotList{
		Slots:            slots,
		Snapshot:         snapshot,
		ActiveByKind:     byKind,
		ActiveByAPIKeyID: byAPIKeyID,
	}, nil
}

func (s *Store) Snapshot(ctx context.Context, now time.Time) (realtimecontract.Snapshot, error) {
	list, err := s.ListActiveSlots(ctx, now)
	if err != nil {
		return realtimecontract.Snapshot{}, err
	}
	return list.Snapshot, nil
}

func (s *Store) expireActiveSlots(ctx context.Context, now time.Time) error {
	return expireSlotsScript.Run(ctx, s.client, []string{activeKey, countersKey}, now.UTC().UnixNano(), slotKeyPrefix, ttlMillis(s.retention)).Err()
}

func (s *Store) findSlotByID(ctx context.Context, slotID string) (realtimecontract.Slot, error) {
	row, err := s.client.HGetAll(ctx, s.slotKey(slotID)).Result()
	if err != nil {
		return realtimecontract.Slot{}, err
	}
	if len(row) == 0 {
		return realtimecontract.Slot{}, realtimecontract.ErrSlotNotFound
	}
	slot := slotFromHash(row)
	if slot.ID == "" {
		return realtimecontract.Slot{}, realtimecontract.ErrSlotNotFound
	}
	return slot, nil
}

func (s *Store) snapshotFromSlots(ctx context.Context, slots []realtimecontract.Slot) (realtimecontract.Snapshot, error) {
	counters, err := s.client.HGetAll(ctx, countersKey).Result()
	if err != nil {
		return realtimecontract.Snapshot{}, err
	}
	byEndpoint := map[string]int{}
	for _, slot := range slots {
		endpoint := slot.SourceEndpoint
		if endpoint == "" {
			endpoint = "unknown"
		}
		byEndpoint[endpoint]++
	}
	return realtimecontract.Snapshot{
		ActiveSlots:      len(slots),
		AcquiredTotal:    parseInt(counters["acquired"]),
		ReleasedTotal:    parseInt(counters["released"]),
		RejectedTotal:    parseInt(counters["rejected"]),
		ActiveByEndpoint: byEndpoint,
	}, nil
}

func (s *Store) slotKey(id string) string {
	return slotKeyPrefix + id
}

func emptyActiveSlotList() realtimecontract.ActiveSlotList {
	return realtimecontract.ActiveSlotList{
		Snapshot:         realtimecontract.Snapshot{ActiveByEndpoint: map[string]int{}},
		ActiveByKind:     map[realtimecontract.SlotKind]int{},
		ActiveByAPIKeyID: map[int]int{},
	}
}

func slotFromHash(row map[string]string) realtimecontract.Slot {
	var stickyAccountID *int
	if parsed := parseInt(row["sticky_account_id"]); parsed > 0 {
		stickyAccountID = &parsed
	}
	var releasedAt *time.Time
	if value := parseUnixNano(row["released_at_unix_nano"]); !value.IsZero() {
		releasedAt = &value
	}
	return realtimecontract.Slot{
		ID:                     row["id"],
		Kind:                   realtimecontract.SlotKind(row["kind"]),
		RequestID:              row["request_id"],
		UserID:                 parseInt(row["user_id"]),
		APIKeyID:               parseInt(row["api_key_id"]),
		SourceEndpoint:         row["source_endpoint"],
		SessionAffinitySource:  row["session_affinity_source"],
		SessionAffinityKeyHash: row["session_affinity_key_hash"],
		StickyAccountID:        stickyAccountID,
		StickyStrength:         row["sticky_strength"],
		AcquiredAt:             parseUnixNano(row["acquired_at_unix_nano"]),
		ReleasedAt:             releasedAt,
	}
}

func optionalIntString(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func parseUnixNano(value string) time.Time {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed == 0 {
		return time.Time{}
	}
	return time.Unix(0, parsed).UTC()
}

func ttlMillis(value time.Duration) int64 {
	if value <= 0 {
		return int64(time.Millisecond)
	}
	return int64(value / time.Millisecond)
}

func scriptResult(values []any) (string, string) {
	if len(values) == 0 {
		return "", ""
	}
	code := scriptString(values[0])
	value := ""
	if len(values) > 1 {
		value = scriptString(values[1])
	}
	return code, value
}

func scriptString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return fmt.Sprint(value)
	}
}
