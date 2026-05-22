package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
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
	mu            sync.Mutex
	clock         Clock
	limits        Limits
	nextID        int
	active        map[string]contract.Slot
	activeByKey   map[int]int
	acquiredTotal int
	releasedTotal int
	rejectedTotal int
}

var _ contract.Manager = (*Service)(nil)

func New(limits Limits, clock Clock) (*Service, error) {
	if limits.MaxOpenSlots < 0 || limits.MaxOpenSlotsPerKey < 0 {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{
		clock:       clock,
		limits:      limits,
		active:      map[string]contract.Slot{},
		activeByKey: map[int]int{},
	}, nil
}

func (s *Service) Acquire(_ context.Context, req contract.AcquireRequest) (contract.Slot, error) {
	if s == nil {
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

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.limits.MaxOpenSlots > 0 && len(s.active) >= s.limits.MaxOpenSlots {
		s.rejectedTotal++
		return contract.Slot{}, ErrLimitExceeded
	}
	if s.limits.MaxOpenSlotsPerKey > 0 && s.activeByKey[req.APIKeyID] >= s.limits.MaxOpenSlotsPerKey {
		s.rejectedTotal++
		return contract.Slot{}, ErrLimitExceeded
	}

	s.nextID++
	slot := contract.Slot{
		ID:                     fmt.Sprintf("rtws_%d", s.nextID),
		Kind:                   kind,
		RequestID:              requestID,
		UserID:                 req.UserID,
		APIKeyID:               req.APIKeyID,
		SourceEndpoint:         sourceEndpoint,
		SessionAffinitySource:  strings.TrimSpace(req.SessionAffinitySource),
		SessionAffinityKeyHash: affinityHash(req.SessionAffinityKey),
		StickyAccountID:        cloneInt(req.StickyAccountID),
		StickyStrength:         strings.TrimSpace(req.StickyStrength),
		AcquiredAt:             s.clock.Now(),
	}
	s.active[slot.ID] = slot
	s.activeByKey[slot.APIKeyID]++
	s.acquiredTotal++
	return slot, nil
}

func (s *Service) Release(_ context.Context, slotID string) (contract.Slot, error) {
	if s == nil {
		return contract.Slot{}, ErrInvalidInput
	}
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return contract.Slot{}, ErrInvalidInput
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	slot, ok := s.active[slotID]
	if !ok {
		return contract.Slot{}, ErrSlotNotFound
	}
	delete(s.active, slotID)
	if s.activeByKey[slot.APIKeyID] > 1 {
		s.activeByKey[slot.APIKeyID]--
	} else {
		delete(s.activeByKey, slot.APIKeyID)
	}
	now := s.clock.Now()
	slot.ReleasedAt = &now
	s.releasedTotal++
	return slot, nil
}

func (s *Service) Snapshot(_ context.Context) contract.Snapshot {
	if s == nil {
		return contract.Snapshot{ActiveByEndpoint: map[string]int{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	byEndpoint := map[string]int{}
	for _, slot := range s.active {
		endpoint := strings.TrimSpace(slot.SourceEndpoint)
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

func affinityHash(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
