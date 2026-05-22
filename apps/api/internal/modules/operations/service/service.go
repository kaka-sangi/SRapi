package service

import (
	"context"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store contract.RetentionStore
	clock Clock
}

func New(store contract.RetentionStore, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) CleanupRetention(ctx context.Context, policy contract.RetentionPolicy) (contract.CleanupResult, error) {
	if s == nil || s.store == nil {
		return contract.CleanupResult{}, ErrInvalidInput
	}
	now := s.clock.Now()
	return s.store.Cleanup(ctx, contract.RetentionCutoffs{
		UsageLogs:              cutoff(now, policy.UsageLogs),
		SchedulerDecisions:     cutoff(now, policy.SchedulerDecisions),
		SchedulerFeedbacks:     cutoff(now, policy.SchedulerFeedbacks),
		AuditLogs:              cutoff(now, policy.AuditLogs),
		AccountHealthSnapshots: cutoff(now, policy.AccountHealthSnapshots),
	})
}

func cutoff(now time.Time, retention time.Duration) *time.Time {
	if retention <= 0 {
		return nil
	}
	cutoff := now.Add(-retention)
	return &cutoff
}
