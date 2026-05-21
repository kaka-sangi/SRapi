package service

import (
	"context"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store contract.Store
	clock Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{store: store, clock: clock}, nil
}

func (s *Service) Record(ctx context.Context, req contract.RecordRequest) (contract.UsageLog, error) {
	if strings.TrimSpace(req.RequestID) == "" || req.UserID <= 0 || req.APIKeyID <= 0 || strings.TrimSpace(req.SourceEndpoint) == "" || strings.TrimSpace(req.Model) == "" {
		return contract.UsageLog{}, ErrInvalidInput
	}
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "USD"
	}
	cost := strings.TrimSpace(req.Cost)
	if cost == "" {
		cost = "0.00000000"
	}
	sourceProtocol := strings.TrimSpace(req.SourceProtocol)
	if sourceProtocol == "" {
		sourceProtocol = "openai-compatible"
	}
	totalTokens := req.InputTokens + req.OutputTokens + req.CachedTokens
	return s.store.Create(ctx, contract.UsageLog{
		RequestID:             strings.TrimSpace(req.RequestID),
		UserID:                req.UserID,
		APIKeyID:              req.APIKeyID,
		ProviderID:            req.ProviderID,
		AccountID:             req.AccountID,
		SourceProtocol:        sourceProtocol,
		SourceEndpoint:        strings.TrimSpace(req.SourceEndpoint),
		TargetProtocol:        strings.TrimSpace(req.TargetProtocol),
		Model:                 strings.TrimSpace(req.Model),
		InputTokens:           req.InputTokens,
		OutputTokens:          req.OutputTokens,
		CachedTokens:          req.CachedTokens,
		TotalTokens:           totalTokens,
		UsageEstimated:        req.UsageEstimated,
		LatencyMS:             req.LatencyMS,
		Success:               req.Success,
		ErrorClass:            req.ErrorClass,
		Cost:                  cost,
		Currency:              currency,
		CompatibilityWarnings: cloneStrings(req.CompatibilityWarnings),
		CreatedAt:             s.clock.Now(),
	})
}

func (s *Service) List(ctx context.Context) ([]contract.UsageLog, error) {
	return s.store.List(ctx)
}

func (s *Service) ListByUser(ctx context.Context, userID int) ([]contract.UsageLog, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListByUser(ctx, userID)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}
