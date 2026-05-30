package service

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/internal/modules/group_rate_limits/contract"
)

// ErrInvalidInput is returned for malformed input.
var ErrInvalidInput = errors.New("invalid account group rate limit")

type Service struct {
	store contract.Store
}

func New(store contract.Store) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	return &Service{store: store}, nil
}

func (s *Service) ListLimits(ctx context.Context) ([]contract.Limit, error) {
	return s.store.ListLimits(ctx)
}

func (s *Service) UpsertLimit(ctx context.Context, input contract.UpsertLimit) (contract.Limit, error) {
	if input.GroupID <= 0 || input.RPMLimit < 0 || input.TPMLimit < 0 || input.MaxConcurrency < 0 {
		return contract.Limit{}, ErrInvalidInput
	}
	return s.store.UpsertLimit(ctx, input)
}

func (s *Service) DeleteLimit(ctx context.Context, groupID int) error {
	if groupID <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteByGroup(ctx, groupID)
}

// RPMForGroup returns the active RPM ceiling for a group, or 0 when none applies
// (no rule, disabled, non-positive limit, or error — fail-open so rate-limit
// lookups never block traffic).
func (s *Service) RPMForGroup(ctx context.Context, groupID int) int {
	if groupID <= 0 {
		return 0
	}
	limit, err := s.store.FindByGroup(ctx, groupID)
	if err != nil || !limit.Enabled || limit.RPMLimit <= 0 {
		return 0
	}
	return limit.RPMLimit
}

// TPMForGroup returns the active tokens-per-minute ceiling for a group, or 0
// when none applies (fail-open).
func (s *Service) TPMForGroup(ctx context.Context, groupID int) int {
	if groupID <= 0 {
		return 0
	}
	limit, err := s.store.FindByGroup(ctx, groupID)
	if err != nil || !limit.Enabled || limit.TPMLimit <= 0 {
		return 0
	}
	return limit.TPMLimit
}

// ConcurrencyForGroup returns the active max-concurrency ceiling for a group, or
// 0 when none applies (fail-open).
func (s *Service) ConcurrencyForGroup(ctx context.Context, groupID int) int {
	if groupID <= 0 {
		return 0
	}
	limit, err := s.store.FindByGroup(ctx, groupID)
	if err != nil || !limit.Enabled || limit.MaxConcurrency <= 0 {
		return 0
	}
	return limit.MaxConcurrency
}
