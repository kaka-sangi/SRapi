package memory

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
)

type Store struct {
	mu                   sync.Mutex
	nextProviderID       int
	nextOrderID          int
	nextAuditID          int
	nextPromoUsageID     int
	providers            map[int]contract.PaymentProviderInstance
	orders               map[int]contract.PaymentOrder
	orderIDByNo          map[string]int
	auditLogs            map[int]contract.PaymentAuditLog
	auditIDByIdempotency map[string]int
	promoUsages          map[int]contract.PromoCodeApplication
	promoUsageByOrderNo  map[string]int
}

func New() *Store {
	return &Store{
		nextProviderID:       1,
		nextOrderID:          1,
		nextAuditID:          1,
		nextPromoUsageID:     1,
		providers:            map[int]contract.PaymentProviderInstance{},
		orders:               map[int]contract.PaymentOrder{},
		orderIDByNo:          map[string]int{},
		auditLogs:            map[int]contract.PaymentAuditLog{},
		auditIDByIdempotency: map[string]int{},
		promoUsages:          map[int]contract.PromoCodeApplication{},
		promoUsageByOrderNo:  map[string]int{},
	}
}

func (s *Store) CreateProviderInstance(_ context.Context, input contract.CreateStoredProviderInstance) (contract.PaymentProviderInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	provider := contract.PaymentProviderInstance{
		ID:               s.nextProviderID,
		Provider:         input.Provider,
		Name:             input.Name,
		Status:           input.Status,
		ConfigCiphertext: input.ConfigCiphertext,
		ConfigVersion:    input.ConfigVersion,
		SupportedMethods: cloneStringSlice(input.SupportedMethods),
		Limits:           cloneMap(input.Limits),
		SortOrder:        input.SortOrder,
		FeeRate:          defaultMoney(input.FeeRate, "0.00000000"),
		Weight:           defaultWeight(input.Weight),
		Metadata:         cloneMap(input.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	s.providers[provider.ID] = provider
	s.nextProviderID++
	return cloneProvider(provider), nil
}

func (s *Store) ListProviderInstances(_ context.Context) ([]contract.PaymentProviderInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.PaymentProviderInstance, 0, len(s.providers))
	for _, provider := range s.providers {
		out = append(out, cloneProvider(provider))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].ID < out[j].ID
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out, nil
}

func (s *Store) FindProviderInstanceByID(_ context.Context, id int) (contract.PaymentProviderInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	provider, ok := s.providers[id]
	if !ok {
		return contract.PaymentProviderInstance{}, contract.ErrNotFound
	}
	return cloneProvider(provider), nil
}

func (s *Store) DeleteProviderInstance(_ context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[id]; !ok {
		return contract.ErrNotFound
	}
	delete(s.providers, id)
	return nil
}

func (s *Store) UpdateProviderInstance(_ context.Context, input contract.PaymentProviderInstance) (contract.PaymentProviderInstance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.providers[input.ID]
	if !ok {
		return contract.PaymentProviderInstance{}, contract.ErrNotFound
	}
	for _, candidate := range s.providers {
		if candidate.ID != input.ID && candidate.Provider == input.Provider && candidate.Name == input.Name {
			return contract.PaymentProviderInstance{}, contract.ErrConflict
		}
	}
	provider := cloneProvider(input)
	provider.CreatedAt = current.CreatedAt
	if provider.UpdatedAt.IsZero() {
		provider.UpdatedAt = time.Now().UTC()
	}
	s.providers[provider.ID] = provider
	return cloneProvider(provider), nil
}

func (s *Store) PreviewPromoCode(_ context.Context, _ contract.PromoCodePreviewInput) (contract.PromoCodeApplication, error) {
	return contract.PromoCodeApplication{}, contract.ErrNotFound
}

func (s *Store) ReleasePromoCode(_ context.Context, input contract.PromoCodeReleaseInput) (contract.PromoCodeApplication, bool, error) {
	if input.PaymentOrderID <= 0 {
		return contract.PromoCodeApplication{}, false, contract.ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[input.PaymentOrderID]
	if !ok || order.PromoCodeID == nil {
		return contract.PromoCodeApplication{}, false, contract.ErrNotFound
	}
	usageID, ok := s.promoUsageByOrderNo[order.OrderNo]
	if !ok {
		return contract.PromoCodeApplication{}, false, contract.ErrNotFound
	}
	usage := s.promoUsages[usageID]
	if usage.UpdatedAt.After(usage.CreatedAt) {
		return usage, false, nil
	}
	releasedAt := input.ReleasedAt.UTC()
	if releasedAt.IsZero() {
		releasedAt = time.Now().UTC()
	}
	usage.UpdatedAt = releasedAt
	s.promoUsages[usageID] = usage
	return usage, true, nil
}

func (s *Store) CreateOrder(_ context.Context, input contract.CreateStoredOrder) (contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.providers[input.ProviderInstanceID]; !ok {
		return contract.PaymentOrder{}, contract.ErrNotFound
	}
	if _, ok := s.orderIDByNo[input.OrderNo]; ok {
		return contract.PaymentOrder{}, contract.ErrConflict
	}
	now := time.Now().UTC()
	order := contract.PaymentOrder{
		ID:                 s.nextOrderID,
		UserID:             input.UserID,
		OrderNo:            input.OrderNo,
		ProviderInstanceID: input.ProviderInstanceID,
		OriginalAmount:     defaultMoney(input.OriginalAmount, input.Amount),
		DiscountAmount:     defaultMoney(input.DiscountAmount, "0.00000000"),
		FeeAmount:          defaultMoney(input.FeeAmount, "0.00000000"),
		PayableAmount:      defaultMoney(input.PayableAmount, input.Amount),
		PromoCodeID:        cloneInt(input.PromoCodeID),
		Amount:             input.Amount,
		Currency:           input.Currency,
		Status:             input.Status,
		ProductType:        input.ProductType,
		ProductID:          input.ProductID,
		ProviderSnapshot:   cloneMap(input.ProviderSnapshot),
		ExpiresAt:          cloneTime(input.ExpiresAt),
		Metadata:           cloneMap(input.Metadata),
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.orders[order.ID] = order
	s.orderIDByNo[order.OrderNo] = order.ID
	s.nextOrderID++
	if input.PromoCodeID != nil {
		usage := contract.PromoCodeApplication{
			ID:             s.nextPromoUsageID,
			UserID:         input.UserID,
			PromoCodeID:    *input.PromoCodeID,
			PaymentOrderID: order.ID,
			OrderNo:        order.OrderNo,
			OriginalAmount: order.OriginalAmount,
			DiscountAmount: order.DiscountAmount,
			FinalAmount:    order.Amount,
			Currency:       order.Currency,
			AppliedAt:      now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		s.promoUsages[usage.ID] = usage
		s.promoUsageByOrderNo[order.OrderNo] = usage.ID
		s.nextPromoUsageID++
	}
	return cloneOrder(order), nil
}

func (s *Store) UpdateOrder(_ context.Context, input contract.PaymentOrder) (contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.orders[input.ID]; !ok {
		return contract.PaymentOrder{}, contract.ErrNotFound
	}
	order := cloneOrder(input)
	if order.UpdatedAt.IsZero() {
		order.UpdatedAt = time.Now().UTC()
	}
	s.orders[order.ID] = order
	s.orderIDByNo[order.OrderNo] = order.ID
	return cloneOrder(order), nil
}

func (s *Store) FindOrderByID(_ context.Context, id int) (contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[id]
	if !ok {
		return contract.PaymentOrder{}, contract.ErrNotFound
	}
	return cloneOrder(order), nil
}

func (s *Store) FindOrderByOrderNo(_ context.Context, orderNo string) (contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.orderIDByNo[orderNo]
	if !ok {
		return contract.PaymentOrder{}, contract.ErrNotFound
	}
	return cloneOrder(s.orders[id]), nil
}

func (s *Store) ListOrders(_ context.Context) ([]contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.PaymentOrder, 0, len(s.orders))
	for _, order := range s.orders {
		out = append(out, cloneOrder(order))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// ListOrdersPage mirrors the SQL store with newest-first ordering and
// offset/limit slicing — keeps the memory store and ent store interchangeable
// for tests against the OrderPageReader capability.
func (s *Store) ListOrdersPage(_ context.Context, filter contract.OrderListFilter, limit, offset int) (contract.OrderListPageResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wantStatus := strings.TrimSpace(filter.Status)
	matched := make([]contract.PaymentOrder, 0)
	for _, order := range s.orders {
		if filter.UserID != nil && order.UserID != *filter.UserID {
			continue
		}
		if wantStatus != "" && string(order.Status) != wantStatus {
			continue
		}
		matched = append(matched, cloneOrder(order))
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].ID > matched[j].ID })
	total := len(matched)
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return contract.OrderListPageResult{Items: []contract.PaymentOrder{}, Total: total}, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return contract.OrderListPageResult{Items: matched[offset:end], Total: total}, nil
}

func (s *Store) ListPendingOrders(_ context.Context, now time.Time) ([]contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	out := make([]contract.PaymentOrder, 0)
	for _, order := range s.orders {
		if order.Status != contract.OrderStatusPending {
			continue
		}
		if order.ExpiresAt != nil && !order.ExpiresAt.After(now) {
			continue
		}
		out = append(out, cloneOrder(order))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListExpiredPendingOrders(_ context.Context, now time.Time) ([]contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC()
	out := make([]contract.PaymentOrder, 0)
	for _, order := range s.orders {
		if order.Status != contract.OrderStatusPending || order.ExpiresAt == nil || !order.ExpiresAt.Before(now) {
			continue
		}
		out = append(out, cloneOrder(order))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) ListOrdersByUser(_ context.Context, userID int) ([]contract.PaymentOrder, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.PaymentOrder, 0)
	for _, order := range s.orders {
		if order.UserID == userID {
			out = append(out, cloneOrder(order))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Store) CountPendingOrdersByUser(_ context.Context, userID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, order := range s.orders {
		if order.UserID == userID && order.Status == contract.OrderStatusPending {
			count++
		}
	}
	return count, nil
}

func (s *Store) CountInProgressOrdersByProviderInstance(_ context.Context, providerInstanceID int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, order := range s.orders {
		if order.ProviderInstanceID == providerInstanceID && inProgressOrderStatus(order.Status) {
			count++
		}
	}
	return count, nil
}

func (s *Store) ExpireOrder(_ context.Context, orderID int, now time.Time) (contract.PaymentOrder, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order, ok := s.orders[orderID]
	if !ok {
		return contract.PaymentOrder{}, false, contract.ErrNotFound
	}
	now = now.UTC()
	if order.Status != contract.OrderStatusPending || order.ExpiresAt == nil || !order.ExpiresAt.Before(now) {
		return cloneOrder(order), false, nil
	}
	order.Status = contract.OrderStatusExpired
	order.ClosedAt = &now
	order.UpdatedAt = now
	s.orders[order.ID] = order
	return cloneOrder(order), true, nil
}

func inProgressOrderStatus(status contract.OrderStatus) bool {
	switch status {
	case contract.OrderStatusPending, contract.OrderStatusPaid, contract.OrderStatusRefunding:
		return true
	default:
		return false
	}
}

func (s *Store) CreateAuditLog(_ context.Context, input contract.PaymentAuditLog) (contract.PaymentAuditLog, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.auditIDByIdempotency[input.IdempotencyKey]; ok {
		return cloneAuditLog(s.auditLogs[id]), false, nil
	}
	now := time.Now().UTC()
	log := cloneAuditLog(input)
	log.ID = s.nextAuditID
	if log.CreatedAt.IsZero() {
		log.CreatedAt = now
	}
	s.auditLogs[log.ID] = log
	s.auditIDByIdempotency[log.IdempotencyKey] = log.ID
	s.nextAuditID++
	return cloneAuditLog(log), true, nil
}

func (s *Store) ListAuditLogsByOrder(_ context.Context, orderID int) ([]contract.PaymentAuditLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]contract.PaymentAuditLog, 0)
	for _, log := range s.auditLogs {
		if log.OrderID == orderID {
			out = append(out, cloneAuditLog(log))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func cloneProvider(value contract.PaymentProviderInstance) contract.PaymentProviderInstance {
	value.SupportedMethods = cloneStringSlice(value.SupportedMethods)
	value.Limits = cloneMap(value.Limits)
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func cloneOrder(value contract.PaymentOrder) contract.PaymentOrder {
	value.PromoCodeID = cloneInt(value.PromoCodeID)
	value.ProviderTransactionID = cloneString(value.ProviderTransactionID)
	value.ProviderSnapshot = cloneMap(value.ProviderSnapshot)
	value.ExpiresAt = cloneTime(value.ExpiresAt)
	value.PaidAt = cloneTime(value.PaidAt)
	value.ClosedAt = cloneTime(value.ClosedAt)
	value.Metadata = cloneMap(value.Metadata)
	return value
}

func cloneAuditLog(value contract.PaymentAuditLog) contract.PaymentAuditLog {
	value.Payload = cloneMap(value.Payload)
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

func cloneStringSlice(value []string) []string {
	if value == nil {
		return []string{}
	}
	out := make([]string, len(value))
	copy(out, value)
	return out
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func defaultMoney(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

func defaultWeight(value int) int {
	if value <= 0 {
		return 1
	}
	return value
}
