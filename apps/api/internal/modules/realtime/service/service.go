package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/realtime/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Limits struct {
	MaxOpenSlots       int
	MaxOpenSlotsPerKey int
}

type Service struct {
	clock  Clock
	limits Limits
	store  contract.Store
}

var _ contract.Manager = (*Service)(nil)

func New(limits Limits, clock Clock) (*Service, error) {
	store, err := NewMemoryStore()
	if err != nil {
		return nil, err
	}
	return NewWithStore(limits, clock, store)
}

func NewWithStore(limits Limits, clock Clock, store contract.Store) (*Service, error) {
	if limits.MaxOpenSlots < 0 || limits.MaxOpenSlotsPerKey < 0 || store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{
		clock:  clock,
		limits: limits,
		store:  store,
	}, nil
}

func (s *Service) Acquire(ctx context.Context, req contract.AcquireRequest) (contract.Slot, error) {
	if s == nil || s.store == nil {
		return contract.Slot{}, ErrInvalidInput
	}
	kind := req.Kind
	if kind == "" {
		kind = contract.SlotKindResponsesWebSocket
	}
	requestID := strings.TrimSpace(req.RequestID)
	sourceEndpoint := strings.TrimSpace(req.SourceEndpoint)
	if requestID == "" || req.UserID <= 0 || req.APIKeyID <= 0 || sourceEndpoint == "" {
		return contract.Slot{}, ErrInvalidInput
	}

	now := s.clock.Now()
	slot := contract.Slot{
		ID:                     newSlotID(),
		Kind:                   kind,
		RequestID:              requestID,
		UserID:                 req.UserID,
		APIKeyID:               req.APIKeyID,
		SourceEndpoint:         sourceEndpoint,
		SessionAffinitySource:  strings.TrimSpace(req.SessionAffinitySource),
		SessionAffinityKeyHash: AffinityHash(req.SessionAffinityKey),
		StickyAccountID:        CloneInt(req.StickyAccountID),
		StickyStrength:         strings.TrimSpace(req.StickyStrength),
		AcquiredAt:             now,
	}
	return s.store.AcquireSlot(ctx, contract.PreparedSlot{
		Slot: slot,
		Limits: contract.SlotLimits{
			MaxOpenSlots:       s.limits.MaxOpenSlots,
			MaxOpenSlotsPerKey: s.limits.MaxOpenSlotsPerKey,
		},
		ExpireAt: now.Add(slotLeaseTTL),
	})
}

func (s *Service) Release(ctx context.Context, slotID string) (contract.Slot, error) {
	if s == nil || s.store == nil {
		return contract.Slot{}, ErrInvalidInput
	}
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return contract.Slot{}, ErrInvalidInput
	}
	return s.store.ReleaseSlot(ctx, slotID, s.clock.Now())
}

func (s *Service) Snapshot(ctx context.Context) (contract.Snapshot, error) {
	if s == nil || s.store == nil {
		return emptySnapshot(), nil
	}
	return s.store.Snapshot(ctx, s.clock.Now())
}

func (s *Service) ListActiveSlots(ctx context.Context) (contract.ActiveSlotList, error) {
	if s == nil || s.store == nil {
		return emptyActiveSlotList(), nil
	}
	return s.store.ListActiveSlots(ctx, s.clock.Now())
}

func newSlotID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "rtws_" + hex.EncodeToString([]byte(time.Now().UTC().Format(time.RFC3339Nano)))
	}
	return "rtws_" + hex.EncodeToString(raw[:])
}

func AffinityHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func CloneSlot(slot contract.Slot) contract.Slot {
	slot.StickyAccountID = CloneInt(slot.StickyAccountID)
	if slot.ReleasedAt != nil {
		releasedAt := *slot.ReleasedAt
		slot.ReleasedAt = &releasedAt
	}
	return slot
}

func CloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func EmptyActiveSlotList() contract.ActiveSlotList {
	return emptyActiveSlotList()
}

func emptyActiveSlotList() contract.ActiveSlotList {
	return contract.ActiveSlotList{
		Snapshot:         emptySnapshot(),
		ActiveByKind:     map[contract.SlotKind]int{},
		ActiveByAPIKeyID: map[int]int{},
	}
}

func emptySnapshot() contract.Snapshot {
	return contract.Snapshot{ActiveByEndpoint: map[string]int{}}
}
