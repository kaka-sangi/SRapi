package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type Service struct {
	store        contract.Store
	usageCharges contract.UsageChargeStore
	pricing      contract.PricingStore
	clock        Clock
}

func New(store contract.Store, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	svc := &Service{store: store, clock: clock}
	if pricing, ok := store.(contract.PricingStore); ok {
		svc.pricing = pricing
	}
	return svc, nil
}

func NewUsageCharger(store contract.UsageChargeStore, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{usageCharges: store, clock: clock}, nil
}

func NewPricing(store contract.PricingStore, clock Clock) (*Service, error) {
	if store == nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	return &Service{pricing: store, clock: clock}, nil
}

func (s *Service) WithPricingStore(store contract.PricingStore) (*Service, error) {
	if s == nil || store == nil {
		return nil, ErrInvalidInput
	}
	s.pricing = store
	return s, nil
}

func (s *Service) Record(ctx context.Context, req contract.RecordRequest) (contract.LedgerEntry, error) {
	if s == nil || s.store == nil {
		return contract.LedgerEntry{}, ErrInvalidInput
	}
	if req.UserID <= 0 || strings.TrimSpace(string(req.Type)) == "" {
		return contract.LedgerEntry{}, ErrInvalidInput
	}
	return s.store.Create(ctx, contract.LedgerEntry{
		UserID:        req.UserID,
		Type:          req.Type,
		Amount:        money.NormalizeAmount(req.Amount),
		Currency:      money.NormalizeCurrency(req.Currency),
		BalanceBefore: money.NormalizeAmount(req.BalanceBefore),
		BalanceAfter:  money.NormalizeAmount(req.BalanceAfter),
		ReferenceType: strings.TrimSpace(req.ReferenceType),
		ReferenceID:   strings.TrimSpace(req.ReferenceID),
		Metadata:      cloneMap(req.Metadata),
		CreatedAt:     s.clock.Now(),
	})
}

func (s *Service) List(ctx context.Context) ([]contract.LedgerEntry, error) {
	if s == nil || s.store == nil {
		return nil, ErrInvalidInput
	}
	return s.store.List(ctx)
}

func (s *Service) ChargePendingUsage(ctx context.Context, req contract.ChargePendingUsageRequest) (contract.ChargePendingUsageResult, error) {
	if s == nil || s.usageCharges == nil {
		return contract.ChargePendingUsageResult{}, ErrInvalidInput
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	chargedAt := req.ChargedAt
	if chargedAt.IsZero() {
		chargedAt = s.clock.Now()
	}
	pending, err := s.usageCharges.ListPendingUsageCharges(ctx, limit)
	if err != nil {
		return contract.ChargePendingUsageResult{}, err
	}
	result := contract.ChargePendingUsageResult{Selected: len(pending)}
	if len(pending) == 0 {
		return result, nil
	}
	var firstErr error
	sort.Slice(pending, func(i, j int) bool {
		if pending[i].UserID != pending[j].UserID {
			return pending[i].UserID < pending[j].UserID
		}
		if pending[i].Currency != pending[j].Currency {
			return pending[i].Currency < pending[j].Currency
		}
		return pending[i].UsageLogID < pending[j].UsageLogID
	})
	for start := 0; start < len(pending); {
		end := start + 1
		for end < len(pending) && pending[end].UserID == pending[start].UserID && pending[end].Currency == pending[start].Currency {
			end++
		}
		usageLogIDs := make([]int, 0, end-start)
		for _, item := range pending[start:end] {
			usageLogIDs = append(usageLogIDs, item.UsageLogID)
		}
		chargeResult, err := s.ChargeUsage(ctx, contract.ChargeUsageRequest{
			UserID:        pending[start].UserID,
			Currency:      pending[start].Currency,
			UsageLogIDs:   usageLogIDs,
			ChargedAt:     chargedAt,
			ReferenceType: "usage_log_batch",
			ReferenceID:   usageChargeReferenceID(usageLogIDs),
		})
		if err != nil {
			firstErr = errors.Join(firstErr, err)
			start = end
			continue
		}
		if chargeResult.LedgerEntry.ID > 0 {
			result.Batches = append(result.Batches, chargeResult)
			result.Charged += len(chargeResult.ChargedUsageLogIDs)
		}
		start = end
	}
	return result, firstErr
}

func (s *Service) ChargeUsage(ctx context.Context, req contract.ChargeUsageRequest) (contract.ChargeUsageResult, error) {
	if s == nil || s.usageCharges == nil {
		return contract.ChargeUsageResult{}, ErrInvalidInput
	}
	if req.UserID <= 0 || len(req.UsageLogIDs) == 0 {
		return contract.ChargeUsageResult{}, ErrInvalidInput
	}
	req.Currency = money.NormalizeCurrency(req.Currency)
	if strings.TrimSpace(req.ReferenceType) == "" {
		req.ReferenceType = "usage_log_batch"
	}
	if strings.TrimSpace(req.ReferenceID) == "" {
		req.ReferenceID = usageChargeReferenceID(req.UsageLogIDs)
	}
	if req.ChargedAt.IsZero() {
		req.ChargedAt = s.clock.Now()
	}
	return s.usageCharges.ChargeUsage(ctx, req)
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

func usageChargeReferenceID(ids []int) string {
	if len(ids) == 0 {
		return ""
	}
	if len(ids) == 1 {
		return strconv.Itoa(ids[0])
	}
	return strconv.Itoa(ids[0]) + "-" + strconv.Itoa(ids[len(ids)-1])
}
