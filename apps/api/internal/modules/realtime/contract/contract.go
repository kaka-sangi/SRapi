package contract

import (
	"context"
	"time"
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

type Manager interface {
	Acquire(ctx context.Context, req AcquireRequest) (Slot, error)
	Release(ctx context.Context, slotID string) (Slot, error)
	Snapshot(ctx context.Context) Snapshot
}
