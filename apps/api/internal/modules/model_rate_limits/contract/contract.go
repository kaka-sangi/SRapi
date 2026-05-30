package contract

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound is returned when no rate limit exists for a model.
var ErrNotFound = errors.New("model rate limit not found")

// Limit is a global per-model capacity ceiling (requests-per-minute and/or max
// concurrent in-flight requests). 0 on either field means unlimited.
type Limit struct {
	ID             int
	ModelID        int
	RPMLimit       int
	MaxConcurrency int
	Enabled        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type UpsertLimit struct {
	ModelID        int
	RPMLimit       int
	MaxConcurrency int
	Enabled        bool
}

// Store persists per-model rate limits.
type Store interface {
	UpsertLimit(ctx context.Context, input UpsertLimit) (Limit, error)
	DeleteByModel(ctx context.Context, modelID int) error
	FindByModel(ctx context.Context, modelID int) (Limit, error)
	ListLimits(ctx context.Context) ([]Limit, error)
}
