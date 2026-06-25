package service

import (
	"context"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

type ManualPauseRequest struct {
	Until  time.Time
	Reason string
}

func (s *Service) ApplyManualPause(ctx context.Context, id int, req ManualPauseRequest) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	until := req.Until.UTC()
	now := s.clock.Now().UTC()
	if !until.After(now) {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 200 {
		reason = reason[:200]
	}
	account.TempUnschedulableUntil = &until
	account.TempUnschedulableReason = reason
	account.UpdatedAt = s.clock.Now()
	return s.persistAccount(ctx, account)
}

func (s *Service) ClearManualPause(ctx context.Context, id int) (contract.ProviderAccount, error) {
	if id <= 0 {
		return contract.ProviderAccount{}, ErrInvalidInput
	}
	account, err := s.store.FindByID(ctx, id)
	if err != nil {
		return contract.ProviderAccount{}, err
	}
	if account.TempUnschedulableUntil == nil {
		return account, nil
	}
	account.TempUnschedulableUntil = nil
	account.TempUnschedulableReason = ""
	account.UpdatedAt = s.clock.Now()
	return s.persistAccount(ctx, account)
}
