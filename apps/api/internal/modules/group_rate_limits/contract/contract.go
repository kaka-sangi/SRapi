package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when no rate limit exists for a group.
var ErrNotFound = errors.New("account group rate limit not found")

// Limit is a per-account-group capacity ceiling (requests-per-minute and/or
// max concurrent in-flight requests). 0 on either field means unlimited.
type Limit struct {
	ID             int
	GroupID        int
	RPMLimit       int
	TPMLimit       int
	MaxConcurrency int
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type UpsertLimit struct {
	GroupID        int
	RPMLimit       int
	TPMLimit       int
	MaxConcurrency int
	Enabled        bool
}

// Store persists per-account-group rate limits.
type Store interface {
	UpsertLimit(ctx context.Context, input UpsertLimit) (Limit, error)
	DeleteByGroup(ctx context.Context, groupID int) error
	FindByGroup(ctx context.Context, groupID int) (Limit, error)
	ListLimits(ctx context.Context) ([]Limit, error)
}
