package service

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
)

const slotLeaseTTL = 24 * time.Hour

type MemoryStore struct {
	mu            sync.Mutex
	active        map[string]contract.PreparedSlot
	acquiredTotal int
	releasedTotal int
	rejectedTotal int
}

var _ contract.Store = (*MemoryStore)(nil)

func NewMemoryStore() (*MemoryStore, error) {
	return &MemoryStore{active: map[string]contract.PreparedSlot{}}, nil
}

func (s *MemoryStore) AcquireSlot(_ context.Context, input contract.PreparedSlot) (contract.Slot, error) {
	if s == nil || input.Slot.ID == "" || input.Slot.RequestID == "" || input.Slot.UserID <= 0 || input.Slot.APIKeyID <= 0 || input.Slot.SourceEndpoint == "" {
		return contract.Slot{}, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(input.Slot.AcquiredAt)
	active, activeByKey := s.activeCountsLocked()
	if input.Limits.MaxOpenSlots > 0 && active >= input.Limits.MaxOpenSlots {
		s.rejectedTotal++
		return contract.Slot{}, ErrLimitExceeded
	}
	if input.Limits.MaxOpenSlotsPerKey > 0 && activeByKey[input.Slot.APIKeyID] >= input.Limits.MaxOpenSlotsPerKey {
		s.rejectedTotal++
		return contract.Slot{}, ErrLimitExceeded
	}
	s.active[input.Slot.ID] = clonePreparedSlot(input)
	s.acquiredTotal++
	return CloneSlot(input.Slot), nil
}

func (s *MemoryStore) ReleaseSlot(_ context.Context, slotID string, releasedAt time.Time) (contract.Slot, error) {
	if s == nil || slotID == "" {
		return contract.Slot{}, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	active, ok := s.active[slotID]
	if !ok {
		return contract.Slot{}, ErrSlotNotFound
	}
	delete(s.active, slotID)
	slot := CloneSlot(active.Slot)
	releasedAt = releasedAt.UTC()
	slot.ReleasedAt = &releasedAt
	s.releasedTotal++
	return slot, nil
}

func (s *MemoryStore) ListActiveSlots(_ context.Context, now time.Time) (contract.ActiveSlotList, error) {
	if s == nil {
		return emptyActiveSlotList(), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	return s.activeListLocked(), nil
}

func (s *MemoryStore) Snapshot(_ context.Context, now time.Time) (contract.Snapshot, error) {
	if s == nil {
		return emptySnapshot(), nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(now)
	return s.snapshotLocked(), nil
}

func (s *MemoryStore) expireLocked(now time.Time) {
	if now.IsZero() {
		return
	}
	for id, slot := range s.active {
		if !slot.ExpireAt.IsZero() && !slot.ExpireAt.After(now) {
			delete(s.active, id)
			s.releasedTotal++
		}
	}
}

func (s *MemoryStore) activeListLocked() contract.ActiveSlotList {
	slots := make([]contract.Slot, 0, len(s.active))
	byKind := map[contract.SlotKind]int{}
	byAPIKeyID := map[int]int{}
	for _, active := range s.active {
		slot := CloneSlot(active.Slot)
		slots = append(slots, slot)
		byKind[slot.Kind]++
		byAPIKeyID[slot.APIKeyID]++
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].AcquiredAt.Equal(slots[j].AcquiredAt) {
			return slots[i].ID < slots[j].ID
		}
		return slots[i].AcquiredAt.Before(slots[j].AcquiredAt)
	})
	return contract.ActiveSlotList{
		Slots:            slots,
		Snapshot:         s.snapshotLocked(),
		ActiveByKind:     byKind,
		ActiveByAPIKeyID: byAPIKeyID,
	}
}

func (s *MemoryStore) snapshotLocked() contract.Snapshot {
	byEndpoint := map[string]int{}
	for _, active := range s.active {
		endpoint := active.Slot.SourceEndpoint
		if endpoint == "" {
			endpoint = "unknown"
		}
		byEndpoint[endpoint]++
	}
	return contract.Snapshot{
		ActiveSlots:      len(s.active),
		AcquiredTotal:    s.acquiredTotal,
		ReleasedTotal:    s.releasedTotal,
		RejectedTotal:    s.rejectedTotal,
		ActiveByEndpoint: byEndpoint,
	}
}

func (s *MemoryStore) activeCountsLocked() (int, map[int]int) {
	byKey := map[int]int{}
	for _, slot := range s.active {
		byKey[slot.Slot.APIKeyID]++
	}
	return len(s.active), byKey
}

func clonePreparedSlot(slot contract.PreparedSlot) contract.PreparedSlot {
	slot.Slot = CloneSlot(slot.Slot)
	return slot
}
