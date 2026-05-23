package contract

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput  = errors.New("invalid realtime slot input")
	ErrLimitExceeded = errors.New("realtime slot limit exceeded")
	ErrSlotNotFound  = errors.New("realtime slot not found")
)

type SlotKind string

const (
	SlotKindResponsesWebSocket SlotKind = "responses_websocket"
	SlotKindRealtimeWebSocket  SlotKind = "realtime_websocket"
)

type Slot struct {
	ID                     string
	Kind                   SlotKind
	RequestID              string
	UserID                 int
	APIKeyID               int
	SourceEndpoint         string
	SessionAffinitySource  string
	SessionAffinityKeyHash string
	StickyAccountID        *int
	StickyStrength         string
	AcquiredAt             time.Time
	ReleasedAt             *time.Time
}

type AcquireRequest struct {
	Kind                  SlotKind
	RequestID             string
	UserID                int
	APIKeyID              int
	SourceEndpoint        string
	SessionAffinityKey    string
	SessionAffinitySource string
	StickyAccountID       *int
	StickyStrength        string
}

type Snapshot struct {
	ActiveSlots      int
	AcquiredTotal    int
	ReleasedTotal    int
	RejectedTotal    int
	ActiveByEndpoint map[string]int
}

type ActiveSlotList struct {
	Slots            []Slot
	Snapshot         Snapshot
	ActiveByKind     map[SlotKind]int
	ActiveByAPIKeyID map[int]int
}

type PreparedSlot struct {
	Slot     Slot
	Limits   SlotLimits
	ExpireAt time.Time
}

type SlotLimits struct {
	MaxOpenSlots       int
	MaxOpenSlotsPerKey int
}

type Store interface {
	AcquireSlot(ctx context.Context, slot PreparedSlot) (Slot, error)
	ReleaseSlot(ctx context.Context, slotID string, releasedAt time.Time) (Slot, error)
	ListActiveSlots(ctx context.Context, now time.Time) (ActiveSlotList, error)
	Snapshot(ctx context.Context, now time.Time) (Snapshot, error)
}

type Manager interface {
	Acquire(ctx context.Context, req AcquireRequest) (Slot, error)
	Release(ctx context.Context, slotID string) (Slot, error)
	Snapshot(ctx context.Context) (Snapshot, error)
	ListActiveSlots(ctx context.Context) (ActiveSlotList, error)
}
