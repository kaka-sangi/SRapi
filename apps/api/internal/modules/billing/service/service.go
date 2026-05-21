package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
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

func (s *Service) Record(ctx context.Context, req contract.RecordRequest) (contract.LedgerEntry, error) {
	if req.UserID <= 0 || strings.TrimSpace(string(req.Type)) == "" {
		return contract.LedgerEntry{}, ErrInvalidInput
	}
	amount := defaultMoney(req.Amount)
	currency := strings.TrimSpace(req.Currency)
	if currency == "" {
		currency = "USD"
	}
	return s.store.Create(ctx, contract.LedgerEntry{
		UserID:        req.UserID,
		Type:          req.Type,
		Amount:        amount,
		Currency:      currency,
		BalanceBefore: defaultMoney(req.BalanceBefore),
		BalanceAfter:  defaultMoney(req.BalanceAfter),
		ReferenceType: strings.TrimSpace(req.ReferenceType),
		ReferenceID:   strings.TrimSpace(req.ReferenceID),
		Metadata:      cloneMap(req.Metadata),
		CreatedAt:     s.clock.Now(),
	})
}

func (s *Service) List(ctx context.Context) ([]contract.LedgerEntry, error) {
	return s.store.List(ctx)
}

func defaultMoney(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.00000000"
	}
	return value
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return map[string]any{}
	}
	return cloned
}
