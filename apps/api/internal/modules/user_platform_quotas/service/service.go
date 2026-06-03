package service

import (
	"context"
	"errors"
	"strings"

	"github.com/srapi/srapi/apps/api/internal/modules/user_platform_quotas/contract"
)

// ErrInvalidInput is returned for malformed quota input.
var ErrInvalidInput = errors.New("invalid user platform quota input")

type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

func (s *Service) ListByUser(ctx context.Context, userID int) ([]contract.Quota, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListByUser(ctx, userID)
}

func (s *Service) UpsertQuota(ctx context.Context, input contract.UpsertQuota) (contract.Quota, error) {
	input.Platform = strings.TrimSpace(input.Platform)
	if input.UserID <= 0 || input.Platform == "" {
		return contract.Quota{}, ErrInvalidInput
	}
	if strings.TrimSpace(input.Currency) == "" {
		input.Currency = "USD"
	}
	return s.store.UpsertQuota(ctx, input)
}

func (s *Service) DeleteQuota(ctx context.Context, userID int, platform string) error {
	platform = strings.TrimSpace(platform)
	if userID <= 0 || platform == "" {
		return ErrInvalidInput
	}
	return s.store.DeleteByUserPlatform(ctx, userID, platform)
}

// EffectiveQuota returns the enabled per-user quota for a platform, or
// (Quota{}, false) when none is configured or it is disabled. Fail-open: any
// store error yields no quota so a lookup problem never blocks traffic — the
// gateway falls back to the plan default in that case.
func (s *Service) EffectiveQuota(ctx context.Context, userID int, platform string) (contract.Quota, bool) {
	platform = strings.TrimSpace(platform)
	if userID <= 0 || platform == "" {
		return contract.Quota{}, false
	}
	quota, err := s.store.FindByUserPlatform(ctx, userID, platform)
	if err != nil || !quota.Enabled {
		return contract.Quota{}, false
	}
	return quota, true
}
