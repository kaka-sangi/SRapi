package contract

import (
	"context"
	"errors"
	"time"
)

var ErrKeyNotFound = errors.New("api key not found")

type Status string

const (
	StatusActive   Status = "active"
	StatusDisabled Status = "disabled"
	StatusExpired  Status = "expired"
)

type APIKey struct {
	ID                int
	UserID            int
	WorkspaceID       *int
	Name              string
	Prefix            string
	Hash              string
	Status            Status
	Scopes            []string
	AllowedModels     []string
	GroupIDs          []int
	RPMLimit          *int
	TPMLimit          *int
	ConcurrencyLimit  *int
	RequestLimit5h    *int
	RequestLimit1d    *int
	RequestLimit7d    *int
	CostQuota         *string
	CostUsed          string
	CostLimit5h       *string
	CostUsed5h        string
	CostWindowStart5h *time.Time
	CostLimit1d       *string
	CostUsed1d        string
	CostWindowStart1d *time.Time
	CostLimit7d       *string
	CostUsed7d        string
	CostWindowStart7d *time.Time
	AllowedIPs        []string
	DeniedIPs         []string
	ExpiresAt         *time.Time
	LastUsedAt        *time.Time
	CreatedAt         time.Time
}

type CreateRequest struct {
	UserID           int
	WorkspaceID      *int
	Name             string
	Scopes           []string
	AllowedModels    []string
	GroupIDs         []int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	CostQuota        *string
	CostLimit5h      *string
	CostLimit1d      *string
	CostLimit7d      *string
	AllowedIPs       []string
	DeniedIPs        []string
	ExpiresAt        *time.Time
}

type UpdateRequest struct {
	UserID           int
	KeyID            int
	Name             *string
	Status           *Status
	Scopes           *[]string
	AllowedModels    *[]string
	GroupIDs         *[]int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	CostQuota        *string
	CostLimit5h      *string
	CostLimit1d      *string
	CostLimit7d      *string
	AllowedIPs       *[]string
	DeniedIPs        *[]string
	ExpiresAt        *time.Time
}

type CreatedKey struct {
	Key          APIKey
	PlaintextKey string
}

type AuthResult struct {
	Key    APIKey
	UserID int
	// CachedAuth is true when this result came from the in-memory auth
	// cache rather than a fresh SQL lookup. Observability-only — callers
	// must not change enforcement based on this flag (the cached snapshot
	// is the same APIKey shape, just sourced from the LRU). Populated by
	// service.Authenticate when the cache fast-path serves the request.
	CachedAuth bool
}

// DeletedKeyMatch is low-sensitive evidence that a failed plaintext key exactly
// matches a soft-deleted API key tombstone. It never carries the full key,
// secret segment, or stored HMAC.
type DeletedKeyMatch struct {
	KeyID  int
	UserID int
	Name   string
	Prefix string
}

// APIKeyRPMStats is the read-only projection of the in-memory per-key
// request counter exposed by the api_keys service. Lives in contract so
// admin/observability layers can consume the snapshot without pulling in
// the service package's worker types.
//
// Mirrors sub2api's billing_cache_service per-key RPM telemetry, scoped down
// to the fields srapi actually needs today; new fields can be added without
// touching call sites (struct is value-receiver only).
type APIKeyRPMStats struct {
	KeyID    int
	Requests int64
}

type Store interface {
	Create(ctx context.Context, input CreateStoredKey) (APIKey, error)
	Update(ctx context.Context, key APIKey) (APIKey, error)
	Delete(ctx context.Context, id int) error
	FindByPrefix(ctx context.Context, prefix string) (APIKey, error)
	FindDeletedByPrefix(ctx context.Context, prefix string) (APIKey, error)
	FindByID(ctx context.Context, id int) (APIKey, error)
	List(ctx context.Context) ([]APIKey, error)
	ListByUser(ctx context.Context, userID int) ([]APIKey, error)
	TouchLastUsed(ctx context.Context, id int, usedAt time.Time) error
	ApplyCostUsage(ctx context.Context, input CostUsageUpdate) (APIKey, error)
	// ResetUsage zeros the rolling cost-used counters and clears their window
	// starts so the next charge opens a fresh window. Used by admins to
	// recover a key that has bumped against its quota due to a runaway client
	// or after debugging. A single UPDATE so it can't race with ApplyCostUsage.
	ResetUsage(ctx context.Context, id int) (APIKey, error)
}

type CreateStoredKey struct {
	UserID           int
	WorkspaceID      *int
	Name             string
	Prefix           string
	Hash             string
	Status           Status
	Scopes           []string
	AllowedModels    []string
	GroupIDs         []int
	RPMLimit         *int
	TPMLimit         *int
	ConcurrencyLimit *int
	RequestLimit5h   *int
	RequestLimit1d   *int
	RequestLimit7d   *int
	CostQuota        *string
	CostLimit5h      *string
	CostLimit1d      *string
	CostLimit7d      *string
	AllowedIPs       []string
	DeniedIPs        []string
	ExpiresAt        *time.Time
}

// CostUsageUpdate appends a successful gateway request's billable USD cost to
// an API key's materialized spend counters.
type CostUsageUpdate struct {
	KeyID        int
	BillableCost string
	OccurredAt   time.Time
}
