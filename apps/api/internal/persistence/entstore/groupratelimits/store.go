package groupratelimits

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entlimit "github.com/srapi/srapi/apps/api/ent/accountgroupratelimit"
	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
)

var ErrInvalidStore = errors.New("invalid account group rate limit ent store")

// Store is the Ent-backed implementation of the account group rate limit store.
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
	if input.GroupID <= 0 {
		return contract.Limit{}, ErrInvalidStore
	}
	now := time.Now().UTC()
	affected, err := s.client.AccountGroupRateLimit.Update().
		Where(entlimit.AccountGroupIDEQ(input.GroupID)).
		SetRpmLimit(input.RPMLimit).
		SetTpmLimit(input.TPMLimit).
		SetMaxConcurrency(input.MaxConcurrency).
		SetEnabled(input.Enabled).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return contract.Limit{}, err
	}
	if affected == 0 {
		row, err := s.client.AccountGroupRateLimit.Create().
			SetAccountGroupID(input.GroupID).
			SetRpmLimit(input.RPMLimit).
			SetTpmLimit(input.TPMLimit).
			SetMaxConcurrency(input.MaxConcurrency).
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
	return s.FindByGroup(ctx, input.GroupID)
}

func (s *Store) DeleteByGroup(ctx context.Context, groupID int) error {
	if groupID <= 0 {
		return ErrInvalidStore
	}
	affected, err := s.client.AccountGroupRateLimit.Delete().
		Where(entlimit.AccountGroupIDEQ(groupID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func (s *Store) FindByGroup(ctx context.Context, groupID int) (contract.Limit, error) {
	if groupID <= 0 {
		return contract.Limit{}, ErrInvalidStore
	}
	row, err := s.client.AccountGroupRateLimit.Query().
		Where(entlimit.AccountGroupIDEQ(groupID)).
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
	rows, err := s.client.AccountGroupRateLimit.Query().
		Order(ent.Asc(entlimit.FieldAccountGroupID)).
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

func toLimit(row *ent.AccountGroupRateLimit) contract.Limit {
	return contract.Limit{
		ID:             row.ID,
		GroupID:        row.AccountGroupID,
		RPMLimit:       row.RpmLimit,
		TPMLimit:       row.TpmLimit,
		MaxConcurrency: row.MaxConcurrency,
		Enabled:        row.Enabled,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}
