package modelratelimits

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entlimit "github.com/srapi/srapi/apps/api/ent/modelratelimit"
	"github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
)

var ErrInvalidStore = errors.New("invalid model rate limit ent store")

// Store is the Ent-backed implementation of the model rate limit store.
type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) UpsertLimit(ctx context.Context, input contract.UpsertLimit) (contract.Limit, error) {
	if input.ModelID <= 0 {
		return contract.Limit{}, ErrInvalidStore
	}
	now := time.Now().UTC()
	affected, err := s.client.ModelRateLimit.Update().
		Where(entlimit.ModelIDEQ(input.ModelID)).
		SetRpmLimit(input.RPMLimit).
		SetEnabled(input.Enabled).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Limit{}, err
	}
	if affected == 0 {
		row, err := s.client.ModelRateLimit.Create().
			SetModelID(input.ModelID).
			SetRpmLimit(input.RPMLimit).
			SetEnabled(input.Enabled).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				return s.UpsertLimit(ctx, input)
			}
			return contract.Limit{}, err
		}
		return toLimit(row), nil
	}
	return s.FindByModel(ctx, input.ModelID)
}

func (s *Store) DeleteByModel(ctx context.Context, modelID int) error {
	if modelID <= 0 {
		return ErrInvalidStore
	}
	affected, err := s.client.ModelRateLimit.Delete().
		Where(entlimit.ModelIDEQ(modelID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func (s *Store) FindByModel(ctx context.Context, modelID int) (contract.Limit, error) {
	if modelID <= 0 {
		return contract.Limit{}, ErrInvalidStore
	}
	row, err := s.client.ModelRateLimit.Query().
		Where(entlimit.ModelIDEQ(modelID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Limit{}, contract.ErrNotFound
		}
		return contract.Limit{}, err
	}
	return toLimit(row), nil
}

func (s *Store) ListLimits(ctx context.Context) ([]contract.Limit, error) {
	rows, err := s.client.ModelRateLimit.Query().
		Order(ent.Asc(entlimit.FieldModelID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Limit, 0, len(rows))
	for _, row := range rows {
		out = append(out, toLimit(row))
	}
	return out, nil
}

func toLimit(row *ent.ModelRateLimit) contract.Limit {
	return contract.Limit{
		ID:        row.ID,
		ModelID:   row.ModelID,
		RPMLimit:  row.RpmLimit,
		Enabled:   row.Enabled,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}
