package contract

import (
	"context"
	"time"
)

// Sample is one health observation folded into a daily rollup. It is provider
// agnostic so the module does not depend on the accounts contract.
type Sample struct {
	ProviderID  int
	Healthy     bool
	SuccessRate float32
	At          time.Time
}

// Rollup is a per-account, per-day availability aggregate.
type Rollup struct {
	ID                int
	AccountID         int
	ProviderID        int
	Date              string // YYYY-MM-DD (UTC)
	TotalSamples      int
	HealthySamples    int
	AvailabilityRatio float32
	AvgSuccessRate    float32
	ComputedAt        time.Time
}

// Store persists availability rollups.
type Store interface {
	UpsertRollup(ctx context.Context, rollup Rollup) (Rollup, error)
	ListRollupsByAccount(ctx context.Context, accountID int, sinceDate string) ([]Rollup, error)
	ListRollupsSince(ctx context.Context, sinceDate string) ([]Rollup, error)
}
