package healthrollups

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/ent"
	entrollup "github.com/srapi/srapi/apps/api/ent/accountavailabilityrollup"
	"github.com/srapi/srapi/apps/api/internal/modules/health_rollups/contract"
)

var ErrInvalidStore = errors.New("invalid health rollup ent store")

// Store is the Ent-backed implementation of the availability rollup store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) UpsertRollup(ctx context.Context, rollup contract.Rollup) (contract.Rollup, error) {
	if rollup.AccountID <= 0 || rollup.Date == "" {
		return contract.Rollup{}, ErrInvalidStore
	}
	affected, err := s.client.AccountAvailabilityRollup.Update().
		Where(
			entrollup.AccountIDEQ(rollup.AccountID),
			entrollup.BucketDateEQ(rollup.Date),
		).
		SetProviderID(rollup.ProviderID).
		SetTotalSamples(rollup.TotalSamples).
		SetHealthySamples(rollup.HealthySamples).
		SetAvailabilityRatio(float64(rollup.AvailabilityRatio)).
		SetAvgSuccessRate(float64(rollup.AvgSuccessRate)).
		SetComputedAt(rollup.ComputedAt.UTC()).
		SetUpdatedAt(rollup.ComputedAt.UTC()).
		Save(ctx)
	if err != nil {
		return contract.Rollup{}, err
	}
	if affected == 0 {
		row, err := s.client.AccountAvailabilityRollup.Create().
			SetAccountID(rollup.AccountID).
			SetProviderID(rollup.ProviderID).
			SetBucketDate(rollup.Date).
			SetTotalSamples(rollup.TotalSamples).
			SetHealthySamples(rollup.HealthySamples).
			SetAvailabilityRatio(float64(rollup.AvailabilityRatio)).
			SetAvgSuccessRate(float64(rollup.AvgSuccessRate)).
			SetComputedAt(rollup.ComputedAt.UTC()).
			SetCreatedAt(rollup.ComputedAt.UTC()).
			SetUpdatedAt(rollup.ComputedAt.UTC()).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				return s.UpsertRollup(ctx, rollup)
			}
			return contract.Rollup{}, err
		}
		return toRollup(row), nil
	}
	row, err := s.client.AccountAvailabilityRollup.Query().
		Where(
			entrollup.AccountIDEQ(rollup.AccountID),
			entrollup.BucketDateEQ(rollup.Date),
		).
		Only(ctx)
	if err != nil {
		return contract.Rollup{}, err
	}
	return toRollup(row), nil
}

func (s *Store) ListRollupsByAccount(ctx context.Context, accountID int, sinceDate string) ([]contract.Rollup, error) {
	if accountID <= 0 {
		return nil, ErrInvalidStore
	}
	query := s.client.AccountAvailabilityRollup.Query().
		Where(entrollup.AccountIDEQ(accountID))
	if sinceDate != "" {
		query = query.Where(entrollup.BucketDateGTE(sinceDate))
	}
	rows, err := query.Order(ent.Asc(entrollup.FieldBucketDate)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Rollup, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRollup(row))
	}
	return out, nil
}

func (s *Store) ListRollupsSince(ctx context.Context, sinceDate string) ([]contract.Rollup, error) {
	query := s.client.AccountAvailabilityRollup.Query()
	if sinceDate != "" {
		query = query.Where(entrollup.BucketDateGTE(sinceDate))
	}
	rows, err := query.Order(ent.Asc(entrollup.FieldBucketDate), ent.Asc(entrollup.FieldAccountID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Rollup, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRollup(row))
	}
	return out, nil
}

func toRollup(row *ent.AccountAvailabilityRollup) contract.Rollup {
	return contract.Rollup{
		ID:                row.ID,
		AccountID:         row.AccountID,
		ProviderID:        row.ProviderID,
		Date:              row.BucketDate,
		TotalSamples:      row.TotalSamples,
		HealthySamples:    row.HealthySamples,
		AvailabilityRatio: float32(row.AvailabilityRatio),
		AvgSuccessRate:    float32(row.AvgSuccessRate),
		ComputedAt:        row.ComputedAt,
	}
}
