package userplatformquotas

import (
	"context"
	"errors"
	"time"

	"github.com/srapi/srapi/apps/api/ent"
	entupq "github.com/srapi/srapi/apps/api/ent/userplatformquota"
	"github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
)

// ErrInvalidStore is returned when the store is constructed or called with
// invalid arguments.
var ErrInvalidStore = errors.New("invalid user platform quota store")

type Store struct {
	client *ent.Client
}

func New(client *ent.Client) (*Store, error) {
	if client == nil {
		return nil, ErrInvalidStore
	}
	return &Store{client: client}, nil
}

func (s *Store) UpsertQuota(ctx context.Context, input contract.UpsertQuota) (contract.Quota, error) {
	if input.UserID <= 0 || input.Platform == "" {
		return contract.Quota{}, ErrInvalidStore
	}
	now := time.Now().UTC()
	update := s.client.UserPlatformQuota.Update().
		Where(entupq.UserIDEQ(input.UserID), entupq.PlatformEQ(input.Platform)).
		SetCurrency(input.Currency).
		SetEnabled(input.Enabled).
		SetUpdatedAt(now)
	applyLimitUpdate(update, input)
	affected, err := update.Save(ctx)
	if err != nil {
		return contract.Quota{}, err
	}
	if affected == 0 {
		row, err := s.client.UserPlatformQuota.Create().
			SetUserID(input.UserID).
			SetPlatform(input.Platform).
			SetNillableDailyLimit(input.DailyLimit).
			SetNillableWeeklyLimit(input.WeeklyLimit).
			SetNillableMonthlyLimit(input.MonthlyLimit).
			SetCurrency(input.Currency).
			SetEnabled(input.Enabled).
			SetCreatedAt(now).
			SetUpdatedAt(now).
			Save(ctx)
		if err != nil {
			if ent.IsConstraintError(err) {
				return s.UpsertQuota(ctx, input)
			}
			return contract.Quota{}, err
		}
		return toQuota(row), nil
	}
	return s.FindByUserPlatform(ctx, input.UserID, input.Platform)
}

func (s *Store) DeleteByUserPlatform(ctx context.Context, userID int, platform string) error {
	if userID <= 0 || platform == "" {
		return ErrInvalidStore
	}
	affected, err := s.client.UserPlatformQuota.Delete().
		Where(entupq.UserIDEQ(userID), entupq.PlatformEQ(platform)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return contract.ErrNotFound
	}
	return nil
}

func (s *Store) FindByUserPlatform(ctx context.Context, userID int, platform string) (contract.Quota, error) {
	if userID <= 0 || platform == "" {
		return contract.Quota{}, ErrInvalidStore
	}
	row, err := s.client.UserPlatformQuota.Query().
		Where(entupq.UserIDEQ(userID), entupq.PlatformEQ(platform)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return contract.Quota{}, contract.ErrNotFound
		}
		return contract.Quota{}, err
	}
	return toQuota(row), nil
}

func (s *Store) ListByUser(ctx context.Context, userID int) ([]contract.Quota, error) {
	if userID <= 0 {
		return nil, ErrInvalidStore
	}
	rows, err := s.client.UserPlatformQuota.Query().
		Where(entupq.UserIDEQ(userID)).
		Order(ent.Asc(entupq.FieldPlatform)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Quota, 0, len(rows))
	for _, row := range rows {
		out = append(out, toQuota(row))
	}
	return out, nil
}

// applyLimitUpdate sets a window limit when provided, or clears it when nil, so
// an upsert fully replaces the stored caps (rather than leaving stale ones).
func applyLimitUpdate(update *ent.UserPlatformQuotaUpdate, input contract.UpsertQuota) {
	if input.DailyLimit != nil {
		update.SetDailyLimit(*input.DailyLimit)
	} else {
		update.ClearDailyLimit()
	}
	if input.WeeklyLimit != nil {
		update.SetWeeklyLimit(*input.WeeklyLimit)
	} else {
		update.ClearWeeklyLimit()
	}
	if input.MonthlyLimit != nil {
		update.SetMonthlyLimit(*input.MonthlyLimit)
	} else {
		update.ClearMonthlyLimit()
	}
}

func toQuota(row *ent.UserPlatformQuota) contract.Quota {
	return contract.Quota{
		ID:           row.ID,
		UserID:       row.UserID,
		Platform:     row.Platform,
		DailyLimit:   row.DailyLimit,
		WeeklyLimit:  row.WeeklyLimit,
		MonthlyLimit: row.MonthlyLimit,
		Currency:     row.Currency,
		Enabled:      row.Enabled,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
