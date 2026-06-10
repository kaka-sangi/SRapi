package service

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
)

// ErrInvalidInput is returned for malformed input.
var ErrInvalidInput = errors.New("invalid health rollup input")

const bucketDateLayout = "2006-01-02"

type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

// Compute buckets samples into per-day availability rollups (UTC). It is pure;
// callers persist the result via RefreshAccount.
func Compute(accountID int, samples []contract.Sample, now time.Time) []contract.Rollup {
	type acc struct {
		providerID   int
		total        int
		healthy      int
		successTotal float32
	}
	buckets := map[string]*acc{}
	for _, sample := range samples {
		date := sample.At.UTC().Format(bucketDateLayout)
		bucket, ok := buckets[date]
		if !ok {
			bucket = &acc{providerID: sample.ProviderID}
			buckets[date] = bucket
		}
		bucket.total++
		if sample.Healthy {
			bucket.healthy++
		}
		bucket.successTotal += sample.SuccessRate
	}
	out := make([]contract.Rollup, 0, len(buckets))
	computedAt := now.UTC()
	for date, bucket := range buckets {
		ratio := float32(0)
		avgSuccess := float32(0)
		if bucket.total > 0 {
			ratio = float32(bucket.healthy) / float32(bucket.total)
			avgSuccess = bucket.successTotal / float32(bucket.total)
		}
		out = append(out, contract.Rollup{
			AccountID:         accountID,
			ProviderID:        bucket.providerID,
			Date:              date,
			TotalSamples:      bucket.total,
			HealthySamples:    bucket.healthy,
			AvailabilityRatio: ratio,
			AvgSuccessRate:    avgSuccess,
			ComputedAt:        computedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// RefreshAccount computes daily rollups from samples and persists them.
func (s *Service) RefreshAccount(ctx context.Context, accountID int, samples []contract.Sample, now time.Time) ([]contract.Rollup, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	rollups := Compute(accountID, samples, now)
	persisted := make([]contract.Rollup, 0, len(rollups))
	for _, rollup := range rollups {
		saved, err := s.store.UpsertRollup(ctx, rollup)
		if err != nil {
			return nil, err
		}
		persisted = append(persisted, saved)
	}
	return persisted, nil
}

// ListByAccount returns persisted rollups for an account within the trailing
// window of days (inclusive of today, UTC).
func (s *Service) ListByAccount(ctx context.Context, accountID, days int, now time.Time) ([]contract.Rollup, error) {
	if accountID <= 0 {
		return nil, ErrInvalidInput
	}
	if days <= 0 {
		days = 7
	}
	since := now.UTC().AddDate(0, 0, -(days - 1)).Format(bucketDateLayout)
	return s.store.ListRollupsByAccount(ctx, accountID, since)
}

// ListRecent returns all persisted rollups within the trailing window of days,
// grouped by the metrics/reporting caller rather than by account.
func (s *Service) ListRecent(ctx context.Context, days int, now time.Time) ([]contract.Rollup, error) {
	if days <= 0 {
		days = 7
	}
	since := now.UTC().AddDate(0, 0, -(days - 1)).Format(bucketDateLayout)
	return s.store.ListRollupsSince(ctx, since)
}
