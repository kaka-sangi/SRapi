package service

import (
	"context"
	"errors"

	"github.com/srapi/srapi/apps/api/internal/modules/model_rate_limits/contract"
)

// ErrInvalidInput is returned for malformed input.
var ErrInvalidInput = errors.New("invalid model rate limit")

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
	if input.ModelID <= 0 || input.RPMLimit < 0 || input.MaxConcurrency < 0 {
		return contract.Limit{}, ErrInvalidInput
	}
	return s.store.UpsertLimit(ctx, input)
}

func (s *Service) DeleteLimit(ctx context.Context, modelID int) error {
	if modelID <= 0 {
		return ErrInvalidInput
	}
	return s.store.DeleteByModel(ctx, modelID)
}

// RPMForModel returns the active RPM ceiling for a model, or 0 when none applies
// (no rule, disabled, or non-positive limit). Errors are treated as "no limit"
// so rate-limit lookups never block traffic.
func (s *Service) RPMForModel(ctx context.Context, modelID int) int {
	if modelID <= 0 {
		return 0
	}
	limit, err := s.store.FindByModel(ctx, modelID)
	if err != nil || !limit.Enabled || limit.RPMLimit <= 0 {
		return 0
	}
	return limit.RPMLimit
}

// ConcurrencyForModel returns the active max-concurrency ceiling for a model, or
// 0 when none applies (fail-open).
func (s *Service) ConcurrencyForModel(ctx context.Context, modelID int) int {
	if modelID <= 0 {
		return 0
	}
	limit, err := s.store.FindByModel(ctx, modelID)
	if err != nil || !limit.Enabled || limit.MaxConcurrency <= 0 {
		return 0
	}
	return limit.MaxConcurrency
}
