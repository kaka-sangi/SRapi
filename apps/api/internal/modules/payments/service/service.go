package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	stripeprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/stripe"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	configVersionV1       = 1
	configCiphertextV1    = "v1"
	defaultOrderExpiresIn = 30 * time.Minute
)

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

type BillingRecorder interface {
	Record(ctx context.Context, req billingcontract.RecordRequest) (billingcontract.LedgerEntry, error)
}

type SubscriptionActivator interface {
	CreateUserSubscription(ctx context.Context, req subscriptioncontract.CreateSubscriptionRequest) (subscriptioncontract.UserSubscription, error)
}

type AuditRecorder interface {
	Record(ctx context.Context, req auditcontract.RecordRequest) (auditcontract.Log, error)
}

type EventEnqueuer interface {
	Enqueue(ctx context.Context, req eventscontract.EnqueueRequest) (eventscontract.OutboxEvent, error)
}

// BalanceAdjuster moves a user's spendable balance. A paid balance_credit order
// credits it; a refund of one debits it. Without this dependency a top-up would
// record a ledger entry but never make the funds spendable (and a refund would
// never claw them back).
type BalanceAdjuster interface {
	CreditBalance(ctx context.Context, userID int, amount, currency string) error
	DebitBalance(ctx context.Context, userID int, amount, currency string) error
}

type Dependencies struct {
	Billing       BillingRecorder
	Subscriptions SubscriptionActivator
	Audit         AuditRecorder
	Events        EventEnqueuer
	Balance       BalanceAdjuster
	Checkout      checkoutprovider.Registry
	Stripe        stripeprovider.CheckoutCreator
}

type Service struct {
	store         contract.Store
	masterKey     []byte
	deps          Dependencies
	clock         Clock
	selectorMu    sync.Mutex
	selectorState map[string][]int
}

func New(store contract.Store, masterKey string, deps Dependencies, clock Clock) (*Service, error) {
	if store == nil || len(masterKey) < 32 {
		return nil, ErrInvalidInput
	}
	derivedKey, err := platformcrypto.DeriveAESKey(masterKey)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if clock == nil {
		clock = SystemClock{}
	}
	if deps.Stripe == nil {
		deps.Stripe = stripeprovider.New()
	}
	if deps.Checkout == nil {
		deps.Checkout = defaultCheckoutRegistry(deps.Stripe)
	}
	return &Service{store: store, masterKey: derivedKey, deps: deps, clock: clock, selectorState: map[string][]int{}}, nil
}

func (s *Service) CreateProviderInstance(ctx context.Context, req contract.CreateProviderInstanceRequest) (contract.PaymentProviderInstance, error) {
	provider := strings.TrimSpace(req.Provider)
	name := strings.TrimSpace(req.Name)
	if provider == "" || name == "" || len(req.Config) == 0 {
		return contract.PaymentProviderInstance{}, ErrInvalidInput
	}
	status := contract.ProviderStatusActive
	if req.Status != nil {
		if !validProviderStatus(*req.Status) {
			return contract.PaymentProviderInstance{}, ErrInvalidInput
		}
		status = *req.Status
	}
	sortOrder := 0
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	feeRate, err := normalizeRatePtr(req.FeeRate)
	if err != nil {
		return contract.PaymentProviderInstance{}, err
	}
	if req.Weight != nil && *req.Weight <= 0 {
		return contract.PaymentProviderInstance{}, ErrInvalidInput
	}
	weight := defaultProviderWeight(req.Weight)
	methods := normalizeMethods(req.SupportedMethods)
	if len(methods) == 0 {
		methods = []string{provider}
	}
	ciphertext, err := s.encryptConfig(provider, name, req.Config)
	if err != nil {
		return contract.PaymentProviderInstance{}, err
	}
	return s.store.CreateProviderInstance(ctx, contract.CreateStoredProviderInstance{
		Provider:         provider,
		Name:             name,
		Status:           status,
		ConfigCiphertext: ciphertext,
		ConfigVersion:    configVersionV1,
		SupportedMethods: methods,
		Limits:           cloneMap(req.Limits),
		SortOrder:        sortOrder,
		FeeRate:          feeRate,
		Weight:           weight,
		Metadata:         cloneMap(req.Metadata),
	})
}

func (s *Service) ListProviderInstances(ctx context.Context) ([]contract.PaymentProviderInstance, error) {
	return s.store.ListProviderInstances(ctx)
}

// FindProviderInstanceByID returns a non-deleted payment provider instance.
func (s *Service) FindProviderInstanceByID(ctx context.Context, id int) (contract.PaymentProviderInstance, error) {
	if id <= 0 {
		return contract.PaymentProviderInstance{}, ErrInvalidInput
	}
	return s.store.FindProviderInstanceByID(ctx, id)
}

// UpdateProviderInstance patches mutable payment provider instance fields.
func (s *Service) UpdateProviderInstance(ctx context.Context, id int, req contract.UpdateProviderInstanceRequest) (contract.PaymentProviderInstance, error) {
	if id <= 0 {
		return contract.PaymentProviderInstance{}, ErrInvalidInput
	}
	provider, err := s.store.FindProviderInstanceByID(ctx, id)
	if err != nil {
		return contract.PaymentProviderInstance{}, err
	}
	originalProvider := provider

	config := map[string]any(nil)
	needsEncrypt := req.Config != nil
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return contract.PaymentProviderInstance{}, ErrInvalidInput
		}
		if name != provider.Name {
			if req.Config == nil {
				config, err = s.decryptConfig(provider, provider.ConfigCiphertext)
				if err != nil {
					return contract.PaymentProviderInstance{}, err
				}
			}
			provider.Name = name
			needsEncrypt = true
		}
	}
	if req.Status != nil {
		if !validProviderStatus(*req.Status) {
			return contract.PaymentProviderInstance{}, ErrInvalidInput
		}
		provider.Status = *req.Status
	}
	if req.SupportedMethods != nil {
		methods := normalizeMethods(*req.SupportedMethods)
		if len(methods) == 0 {
			methods = []string{provider.Provider}
		}
		provider.SupportedMethods = methods
	}
	if req.Limits != nil {
		provider.Limits = cloneMap(*req.Limits)
	}
	if req.SortOrder != nil {
		provider.SortOrder = *req.SortOrder
	}
	if req.FeeRate != nil {
		feeRate, err := normalizeRatePtr(req.FeeRate)
		if err != nil {
			return contract.PaymentProviderInstance{}, err
		}
		provider.FeeRate = feeRate
	}
	if req.Weight != nil {
		if *req.Weight <= 0 {
			return contract.PaymentProviderInstance{}, ErrInvalidInput
		}
		provider.Weight = *req.Weight
	}
	if req.Metadata != nil {
		provider.Metadata = cloneMap(*req.Metadata)
	}
	if req.Config != nil {
		if len(*req.Config) == 0 {
			return contract.PaymentProviderInstance{}, ErrInvalidInput
		}
		config = cloneMap(*req.Config)
	}
	if err := s.validateProviderUpdateInProgressOrderSafety(ctx, originalProvider, provider, req.Config != nil); err != nil {
		return contract.PaymentProviderInstance{}, err
	}
	if needsEncrypt {
		ciphertext, err := s.encryptConfig(provider.Provider, provider.Name, config)
		if err != nil {
			return contract.PaymentProviderInstance{}, err
		}
		provider.ConfigCiphertext = ciphertext
		provider.ConfigVersion = configVersionV1
	}
	provider.UpdatedAt = s.clock.Now()
	return s.store.UpdateProviderInstance(ctx, provider)
}

// DeleteProviderInstance soft-deletes a payment provider instance. The row is
// retained (its orders and audit logs still reference it) but excluded from
// listings and lookups. Refused when in-progress orders still depend on it.
func (s *Service) DeleteProviderInstance(ctx context.Context, id int) error {
	if id <= 0 {
		return ErrInvalidInput
	}
	if _, err := s.store.FindProviderInstanceByID(ctx, id); err != nil {
		return err
	}
	count, err := s.store.CountInProgressOrdersByProviderInstance(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return contract.ErrConflict
	}
	return s.store.DeleteProviderInstance(ctx, id)
}

func (s *Service) validateProviderUpdateInProgressOrderSafety(ctx context.Context, current contract.PaymentProviderInstance, next contract.PaymentProviderInstance, configReplaced bool) error {
	if !providerUpdateNeedsInProgressOrderGuard(current, next, configReplaced) {
		return nil
	}
	count, err := s.store.CountInProgressOrdersByProviderInstance(ctx, current.ID)
	if err != nil {
		return err
	}
	if count > 0 {
		return contract.ErrConflict
	}
	return nil
}

func providerUpdateNeedsInProgressOrderGuard(current contract.PaymentProviderInstance, next contract.PaymentProviderInstance, configReplaced bool) bool {
	if configReplaced {
		return true
	}
	if current.Status != next.Status && next.Status != contract.ProviderStatusActive {
		return true
	}
	return removesSupportedMethods(current.SupportedMethods, next.SupportedMethods)
}

func removesSupportedMethods(current []string, next []string) bool {
	nextSet := map[string]struct{}{}
	for _, method := range normalizeMethods(next) {
		nextSet[method] = struct{}{}
	}
	for _, method := range normalizeMethods(current) {
		if _, ok := nextSet[method]; !ok {
			return true
		}
	}
	return false
}

// TestProviderInstance validates locally stored payment provider configuration without calling upstream payment APIs.
func (s *Service) TestProviderInstance(ctx context.Context, id int) (contract.ProviderInstanceTestResult, error) {
	start := contract.ProviderInstanceTestResult{Status: "failed"}
	instance, err := s.FindProviderInstanceByID(ctx, id)
	if err != nil {
		return start, err
	}
	checks := map[string]any{
		"payment_provider_instance_id": instance.ID,
		"provider":                     instance.Provider,
		"status":                       string(instance.Status),
		"active":                       instance.Status == contract.ProviderStatusActive,
		"supported_methods":            append([]string(nil), instance.SupportedMethods...),
		"config_decrypts":              false,
	}
	config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
	if err != nil {
		checks["config_error"] = "decrypt_failed"
		return contract.ProviderInstanceTestResult{
			ProviderInstance: instance,
			OK:               false,
			Status:           "failed",
			Message:          "payment provider config could not be decrypted",
			Checks:           checks,
		}, nil
	}
	checks["config_decrypts"] = true
	missing := missingProviderConfigFields(instance.Provider, config)
	if len(missing) > 0 {
		checks["missing_requirements"] = missing
		return contract.ProviderInstanceTestResult{
			ProviderInstance: instance,
			OK:               false,
			Status:           "failed",
			Message:          "payment provider config is incomplete",
			Checks:           checks,
		}, nil
	}
	if instance.Status != contract.ProviderStatusActive {
		return contract.ProviderInstanceTestResult{
			ProviderInstance: instance,
			OK:               false,
			Status:           "failed",
			Message:          "payment provider instance is not active",
			Checks:           checks,
		}, nil
	}
	return contract.ProviderInstanceTestResult{
		ProviderInstance: instance,
		OK:               true,
		Status:           "ok",
		Message:          "payment provider instance is configured",
		Checks:           checks,
	}, nil
}

func (s *Service) ListMethods(ctx context.Context) ([]contract.PaymentMethod, error) {
	instances, err := s.store.ListProviderInstances(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]contract.PaymentMethod, 0)
	for _, instance := range instances {
		if instance.Status != contract.ProviderStatusActive {
			continue
		}
		metadata := publicProviderMetadata(instance.Metadata)
		metadata["fee_rate"] = defaultMoney(instance.FeeRate)
		for _, method := range normalizeMethods(instance.SupportedMethods) {
			out = append(out, contract.PaymentMethod{
				Method:             method,
				Provider:           instance.Provider,
				ProviderInstanceID: instance.ID,
				Name:               instance.Name,
				Metadata:           cloneMap(metadata),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Method == out[j].Method {
			return out[i].ProviderInstanceID < out[j].ProviderInstanceID
		}
		return out[i].Method < out[j].Method
	})
	return out, nil
}

func (s *Service) CreateOrder(ctx context.Context, req contract.CreateOrderRequest) (contract.PaymentOrder, error) {
	if req.UserID <= 0 || strings.TrimSpace(req.Method) == "" {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	amount, ok := normalizeMoney(req.Amount)
	if !ok || compareMoney(amount, "0.00000000") <= 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	currency := normalizeCurrency(req.Currency)
	originalAmount := amount
	discountAmount := "0.00000000"
	var promoCodeID *int
	promoCode := normalizePromoCode(req.PromoCode)
	if promoCode != "" {
		preview, err := s.store.PreviewPromoCode(ctx, contract.PromoCodePreviewInput{
			UserID:   req.UserID,
			Code:     promoCode,
			Amount:   amount,
			Currency: currency,
			Now:      s.clock.Now(),
		})
		if err != nil {
			return contract.PaymentOrder{}, err
		}
		if preview.PromoCodeID <= 0 || compareMoney(preview.FinalAmount, "0.00000000") <= 0 || compareMoney(preview.OriginalAmount, amount) != 0 {
			return contract.PaymentOrder{}, ErrInvalidInput
		}
		previewFinal, ok := normalizeMoney(preview.FinalAmount)
		if !ok {
			return contract.PaymentOrder{}, ErrInvalidInput
		}
		previewDiscount, ok := normalizeMoney(preview.DiscountAmount)
		if !ok {
			return contract.PaymentOrder{}, ErrInvalidInput
		}
		amount = previewFinal
		discountAmount = previewDiscount
		promoCodeID = &preview.PromoCodeID
	}
	if !validProduct(req.ProductType, req.ProductID) {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	instance, err := s.selectProviderInstance(ctx, req.Method, amount, currency)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	feeAmount, payableAmount, err := applyFeeRate(amount, instance.FeeRate)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	expiresAt := s.clock.Now().Add(defaultOrderExpiresIn)
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt.UTC()
	}
	if !expiresAt.After(s.clock.Now()) {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	orderNo := newOrderNo()
	orderMetadata := cloneMap(req.Metadata)
	if strings.TrimSpace(req.PayerOpenID) != "" {
		orderMetadata["payer_openid"] = strings.TrimSpace(req.PayerOpenID)
	}
	if strings.TrimSpace(req.PayerClientIP) != "" {
		orderMetadata["payer_client_ip"] = strings.TrimSpace(req.PayerClientIP)
	}
	order, err := s.store.CreateOrder(ctx, contract.CreateStoredOrder{
		UserID:             req.UserID,
		OrderNo:            orderNo,
		ProviderInstanceID: instance.ID,
		OriginalAmount:     originalAmount,
		DiscountAmount:     discountAmount,
		FeeAmount:          feeAmount,
		PayableAmount:      payableAmount,
		PromoCodeID:        cloneInt(promoCodeID),
		PromoCode:          promoCode,
		Amount:             amount,
		Currency:           currency,
		Status:             contract.OrderStatusPending,
		ProductType:        req.ProductType,
		ProductID:          strings.TrimSpace(req.ProductID),
		ProviderSnapshot: map[string]any{
			"provider":             instance.Provider,
			"provider_instance_id": instance.ID,
			"name":                 instance.Name,
			"method":               strings.TrimSpace(req.Method),
			"fee_rate":             defaultMoney(instance.FeeRate),
			"fee_amount":           feeAmount,
			"payable_amount":       payableAmount,
			"metadata":             publicProviderMetadata(instance.Metadata),
		},
		ExpiresAt: &expiresAt,
		Metadata:  orderMetadata,
	})
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	return s.attachProviderCheckout(ctx, order, instance)
}

func (s *Service) ListOrders(ctx context.Context) ([]contract.PaymentOrder, error) {
	return s.store.ListOrders(ctx)
}

// ListOrdersPage delegates to OrderPageReader when supported so admin/payment-
// order reads avoid the legacy whole-table load. Falls back to ListOrders + in-
// memory filter+slice when the store omits the capability.
func (s *Service) ListOrdersPage(ctx context.Context, filter contract.OrderListFilter, limit, offset int) (contract.OrderListPageResult, error) {
	if reader, ok := s.store.(contract.OrderPageReader); ok {
		return reader.ListOrdersPage(ctx, filter, limit, offset)
	}
	all, err := s.store.ListOrders(ctx)
	if err != nil {
		return contract.OrderListPageResult{}, err
	}
	wantStatus := strings.TrimSpace(filter.Status)
	matched := make([]contract.PaymentOrder, 0, len(all))
	for _, order := range all {
		if filter.UserID != nil && order.UserID != *filter.UserID {
			continue
		}
		if wantStatus != "" && string(order.Status) != wantStatus {
			continue
		}
		matched = append(matched, order)
	}
	for i, j := 0, len(matched)-1; i < j; i, j = i+1, j-1 {
		matched[i], matched[j] = matched[j], matched[i]
	}
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

// AggregatePaymentDashboard computes the totals + payment-method breakdown + top
// spenders for paid orders inside the last `days` days. days <= 0 falls back to 30,
// values > 365 clamp to 365.
//
// The aggregation is done in Go after a full ListOrders fetch — payment volumes are
// usually tractable, and skipping a dedicated time-windowed store query keeps this
// adoption surface small. If volumes grow problematic, a windowed store method is
// the obvious next step.
func (s *Service) AggregatePaymentDashboard(ctx context.Context, days int) (contract.PaymentDashboardSnapshot, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	now := time.Now().UTC()
	since := now.AddDate(0, 0, -days)

	orders, err := s.store.ListOrders(ctx)
	if err != nil {
		return contract.PaymentDashboardSnapshot{}, err
	}
	instances, err := s.store.ListProviderInstances(ctx)
	if err != nil {
		return contract.PaymentDashboardSnapshot{}, err
	}
	providerByInstance := make(map[int]string, len(instances))
	for _, inst := range instances {
		providerByInstance[inst.ID] = inst.Provider
	}

	paidStatuses := map[contract.OrderStatus]struct{}{
		contract.OrderStatusPaid:              {},
		contract.OrderStatusFulfilled:         {},
		contract.OrderStatusPartiallyRefunded: {},
		contract.OrderStatusRefunding:         {},
		contract.OrderStatusRefunded:          {},
	}

	currencyCounts := make(map[string]int)
	for _, o := range orders {
		if o.PaidAt != nil && !o.PaidAt.Before(since) {
			if _, ok := paidStatuses[o.Status]; ok && o.Currency != "" {
				currencyCounts[o.Currency]++
			}
		}
	}
	reportCurrency := pickReportCurrency(currencyCounts)

	totalsAmount := new(big.Rat)
	providerAmounts := make(map[string]*big.Rat)
	providerCounts := make(map[string]int)
	userAmounts := make(map[int]*big.Rat)
	userCounts := make(map[int]int)
	var orderCount, paidCount int

	for _, o := range orders {
		// Window check is on CreatedAt for the "all orders in window" count, and
		// on PaidAt for paid revenue — sub2api uses paid_at for everything, but
		// counting unpaid pendings as "orders in window" gives a useful denominator.
		if !o.CreatedAt.Before(since) {
			orderCount++
		}
		if o.PaidAt == nil || o.PaidAt.Before(since) {
			continue
		}
		if _, ok := paidStatuses[o.Status]; !ok {
			continue
		}
		if o.Currency != reportCurrency {
			continue
		}

		amount, ok := parseMoneyRat(o.PayableAmount)
		if !ok {
			continue
		}
		paidCount++
		totalsAmount.Add(totalsAmount, amount)

		provider := providerByInstance[o.ProviderInstanceID]
		if provider == "" {
			provider = "unknown"
		}
		if _, present := providerAmounts[provider]; !present {
			providerAmounts[provider] = new(big.Rat)
		}
		providerAmounts[provider].Add(providerAmounts[provider], amount)
		providerCounts[provider]++

		if _, present := userAmounts[o.UserID]; !present {
			userAmounts[o.UserID] = new(big.Rat)
		}
		userAmounts[o.UserID].Add(userAmounts[o.UserID], amount)
		userCounts[o.UserID]++
	}

	methods := make([]contract.PaymentMethodBreakdown, 0, len(providerAmounts))
	for provider, amount := range providerAmounts {
		methods = append(methods, contract.PaymentMethodBreakdown{
			Provider: provider,
			Count:    providerCounts[provider],
			Amount:   money.FormatRatFixed(amount, 8),
		})
	}
	sort.Slice(methods, func(i, j int) bool {
		// Largest amount first; tie-break by provider name for stability.
		cmp := compareRats(providerAmounts[methods[i].Provider], providerAmounts[methods[j].Provider])
		if cmp != 0 {
			return cmp > 0
		}
		return methods[i].Provider < methods[j].Provider
	})

	type userRow struct {
		userID int
		amount *big.Rat
	}
	userRows := make([]userRow, 0, len(userAmounts))
	for userID, amount := range userAmounts {
		userRows = append(userRows, userRow{userID: userID, amount: amount})
	}
	sort.Slice(userRows, func(i, j int) bool {
		cmp := compareRats(userRows[i].amount, userRows[j].amount)
		if cmp != 0 {
			return cmp > 0
		}
		return userRows[i].userID < userRows[j].userID
	})
	const topUsersLimit = 10
	if len(userRows) > topUsersLimit {
		userRows = userRows[:topUsersLimit]
	}
	topUsers := make([]contract.PaymentTopUser, 0, len(userRows))
	for _, row := range userRows {
		topUsers = append(topUsers, contract.PaymentTopUser{
			UserID:     row.userID,
			Amount:     money.FormatRatFixed(row.amount, 8),
			OrderCount: userCounts[row.userID],
		})
	}

	return contract.PaymentDashboardSnapshot{
		DayRange: days,
		Currency: reportCurrency,
		Totals: contract.PaymentDashboardTotals{
			OrderCount: orderCount,
			PaidCount:  paidCount,
			PaidAmount: money.FormatRatFixed(totalsAmount, 8),
		},
		PaymentMethods: methods,
		TopUsers:       topUsers,
	}, nil
}

func pickReportCurrency(counts map[string]int) string {
	if len(counts) == 0 {
		return "USD"
	}
	bestCount := -1
	best := ""
	for cur, n := range counts {
		if n > bestCount || (n == bestCount && cur < best) {
			bestCount = n
			best = cur
		}
	}
	return best
}

func parseMoneyRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return new(big.Rat), true
	}
	r := new(big.Rat)
	if _, ok := r.SetString(value); !ok {
		return nil, false
	}
	return r, true
}

func compareRats(a, b *big.Rat) int {
	return a.Cmp(b)
}

func (s *Service) ListOrdersByUser(ctx context.Context, userID int) ([]contract.PaymentOrder, error) {
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	return s.store.ListOrdersByUser(ctx, userID)
}

func (s *Service) FindOrderByID(ctx context.Context, id int) (contract.PaymentOrder, error) {
	if id <= 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	return s.store.FindOrderByID(ctx, id)
}

func (s *Service) FindOrderByOrderNo(ctx context.Context, orderNo string) (contract.PaymentOrder, error) {
	orderNo = strings.TrimSpace(orderNo)
	if orderNo == "" {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	return s.store.FindOrderByOrderNo(ctx, orderNo)
}

func (s *Service) ListAuditLogsByOrder(ctx context.Context, orderID int) ([]contract.PaymentAuditLog, error) {
	if orderID <= 0 {
		return nil, ErrInvalidInput
	}
	if _, err := s.store.FindOrderByID(ctx, orderID); err != nil {
		return nil, err
	}
	return s.store.ListAuditLogsByOrder(ctx, orderID)
}

func (s *Service) CancelOrder(ctx context.Context, userID int, orderID int) (contract.PaymentOrder, error) {
	if userID <= 0 || orderID <= 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	order, err := s.store.FindOrderByID(ctx, orderID)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	if order.UserID != userID {
		// Report a foreign order as not-found (404), mirroring handleGetPaymentOrder,
		// so a caller can't distinguish "exists but not yours" (was 400) from
		// "doesn't exist" (404) and enumerate other users' order IDs.
		return contract.PaymentOrder{}, contract.ErrNotFound
	}
	if err := validateTransition(order.Status, contract.OrderStatusCanceled); err != nil {
		return contract.PaymentOrder{}, err
	}
	now := s.clock.Now()
	order.Status = contract.OrderStatusCanceled
	order.ClosedAt = &now
	order.UpdatedAt = now
	updated, err := s.store.UpdateOrder(ctx, order)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	if updated.PromoCodeID != nil {
		if _, _, err := s.store.ReleasePromoCode(ctx, contract.PromoCodeReleaseInput{PaymentOrderID: updated.ID, ReleasedAt: now, Reason: "order_canceled"}); err != nil && !errors.Is(err, contract.ErrNotFound) {
			return contract.PaymentOrder{}, err
		}
	}
	return updated, nil
}

func (s *Service) ExpirePendingOrders(ctx context.Context, now time.Time) (contract.ExpireOrdersResult, error) {
	if now.IsZero() {
		now = s.clock.Now()
	}
	now = now.UTC()
	orders, err := s.store.ListExpiredPendingOrders(ctx, now)
	if err != nil {
		return contract.ExpireOrdersResult{}, err
	}
	result := contract.ExpireOrdersResult{Selected: len(orders)}
	for _, order := range orders {
		before := order
		// Skip an order that can't be expired rather than aborting the whole pass —
		// one bad order must not block every other expired order in the batch.
		if err := validateTransition(order.Status, contract.OrderStatusExpired); err != nil {
			continue
		}
		updated, expired, err := s.store.ExpireOrder(ctx, order.ID, now)
		if err != nil {
			continue
		}
		if !expired {
			continue
		}
		if updated.PromoCodeID != nil {
			if _, _, err := s.store.ReleasePromoCode(ctx, contract.PromoCodeReleaseInput{PaymentOrderID: updated.ID, ReleasedAt: now, Reason: "order_expired"}); err != nil && !errors.Is(err, contract.ErrNotFound) {
				continue
			}
		}
		_, _, err = s.store.CreateAuditLog(ctx, contract.PaymentAuditLog{
			OrderID:            updated.ID,
			ProviderInstanceID: updated.ProviderInstanceID,
			EventType:          "order.expired",
			IdempotencyKey:     "order_expired:" + updated.OrderNo,
			Payload: map[string]any{
				"order_id":   updated.ID,
				"order_no":   updated.OrderNo,
				"expired_at": now.Format(time.RFC3339Nano),
			},
			SignatureValid: true,
			CreatedAt:      now,
		})
		if err != nil {
			return result, err
		}
		s.recordAudit(ctx, nil, "payment_order.expire", "payment_order", strconv.Itoa(updated.ID), paymentOrderAuditSnapshot(before), paymentOrderAuditSnapshot(updated))
		result.Expired++
	}
	return result, nil
}

func (s *Service) HandleWebhook(ctx context.Context, req contract.WebhookRequest) (result contract.WebhookResult, err error) {
	provider := strings.TrimSpace(req.Provider)
	ctx, span := platformotel.StartSpan(ctx, "payments.HandleWebhook",
		attribute.String("srapi.payment.provider", provider),
	)
	defer func() {
		platformotel.EndSpan(span, err, paymentWebhookTraceErrorType(err), paymentWebhookTraceAttrs(result, err)...)
	}()

	normalized, err := s.normalizeWebhook(ctx, provider, req)
	if err != nil {
		return contract.WebhookResult{}, err
	}
	orderNo := normalized.OrderNo
	transactionID := normalized.TransactionID
	status := normalized.Status
	idempotencyKey := normalized.IdempotencyKey
	if provider == "" || orderNo == "" || status == "" {
		return contract.WebhookResult{}, ErrInvalidInput
	}
	if idempotencyKey == "" {
		idempotencyKey = strings.Join([]string{provider, orderNo, transactionID, status}, ":")
	}
	order, err := s.store.FindOrderByOrderNo(ctx, orderNo)
	if err != nil {
		return contract.WebhookResult{}, err
	}
	instance, err := s.store.FindProviderInstanceByID(ctx, order.ProviderInstanceID)
	if err != nil {
		return contract.WebhookResult{}, err
	}
	if instance.Provider != provider {
		return contract.WebhookResult{}, ErrOrderMismatch
	}
	if normalized.ProviderInstanceID > 0 && normalized.ProviderInstanceID != instance.ID {
		return contract.WebhookResult{}, ErrOrderMismatch
	}
	config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
	if err != nil {
		return contract.WebhookResult{}, err
	}
	signatureValid := normalized.SignatureValid
	if !signatureValid {
		signatureValid = verifyWebhookSignature(config, req.Headers, req.Payload)
	}
	auditLog, created, err := s.store.CreateAuditLog(ctx, contract.PaymentAuditLog{
		OrderID:            order.ID,
		ProviderInstanceID: instance.ID,
		EventType:          "webhook." + status,
		IdempotencyKey:     idempotencyKey,
		Payload:            sanitizePayload(normalized.Payload),
		SignatureValid:     signatureValid,
		CreatedAt:          s.clock.Now(),
	})
	if err != nil {
		return contract.WebhookResult{}, err
	}
	if !created {
		// Idempotent replay: reject if the STORED log was unsigned OR the CURRENT
		// request's signature does not verify. Trusting only the stored flag would
		// let an attacker replay a known idempotency key with a forged/absent
		// signature and get a 200 (no-op) back instead of a rejection.
		if !auditLog.SignatureValid || !signatureValid {
			return contract.WebhookResult{}, ErrSignatureInvalid
		}
		current, findErr := s.store.FindOrderByID(ctx, order.ID)
		if findErr != nil {
			return contract.WebhookResult{}, findErr
		}
		return contract.WebhookResult{Order: current, Handled: false}, nil
	}
	if !signatureValid {
		return contract.WebhookResult{}, ErrSignatureInvalid
	}
	if err := verifyWebhookOrder(order, normalized.Payload); err != nil {
		return contract.WebhookResult{}, err
	}
	if status == "failed" {
		updated, err := s.markFailed(ctx, order)
		if err != nil {
			return contract.WebhookResult{}, err
		}
		return contract.WebhookResult{Order: updated, Handled: true}, nil
	}
	if status == "refunded" {
		updated, err := s.completePendingRefund(ctx, order, true, "refund.webhook")
		if err != nil {
			return contract.WebhookResult{}, err
		}
		return contract.WebhookResult{Order: updated, Handled: true}, nil
	}
	if status == "refund_failed" {
		updated, err := s.completePendingRefund(ctx, order, false, "refund.webhook")
		if err != nil {
			return contract.WebhookResult{}, err
		}
		return contract.WebhookResult{Order: updated, Handled: true}, nil
	}
	if status != "paid" {
		return contract.WebhookResult{Order: order, Handled: false}, nil
	}
	if order.Status == contract.OrderStatusFulfilled || order.Status == contract.OrderStatusPaid {
		return contract.WebhookResult{Order: order, Handled: false}, nil
	}
	fulfilled, err := s.markPaidAndFulfill(ctx, order, instance, transactionID)
	if err != nil {
		return contract.WebhookResult{}, err
	}
	return contract.WebhookResult{Order: fulfilled, Handled: true}, nil
}

func paymentWebhookTraceErrorType(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrInvalidInput):
		return "invalid_input"
	case errors.Is(err, ErrSignatureInvalid):
		return "signature_invalid"
	case errors.Is(err, ErrOrderMismatch):
		return "order_mismatch"
	case errors.Is(err, ErrInvalidTransition):
		return "invalid_transition"
	default:
		return "payment_webhook_error"
	}
}

func paymentWebhookTraceAttrs(result contract.WebhookResult, err error) []attribute.KeyValue {
	outcome := "error"
	if err == nil {
		outcome = "ignored"
		if result.Handled {
			outcome = "handled"
		}
	}
	attrs := []attribute.KeyValue{attribute.String("srapi.payment.webhook_outcome", outcome)}
	if result.Order.ID > 0 {
		attrs = append(attrs,
			attribute.Int("srapi.payment.order_id", result.Order.ID),
			attribute.Int("srapi.payment.provider_instance_id", result.Order.ProviderInstanceID),
			attribute.String("srapi.payment.order_status", string(result.Order.Status)),
			attribute.String("srapi.payment.product_type", string(result.Order.ProductType)),
		)
	}
	return attrs
}

func (s *Service) attachProviderCheckout(ctx context.Context, order contract.PaymentOrder, instance contract.PaymentProviderInstance) (contract.PaymentOrder, error) {
	provider, ok := s.deps.Checkout[instance.Provider]
	if !ok {
		return order, nil
	}
	config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
	if err != nil {
		return contract.PaymentOrder{}, fmt.Errorf("%w: failed to decrypt config for %s provider %q", ErrProviderConfigInvalid, instance.Provider, instance.Name)
	}
	session, err := provider.CreateSession(checkoutprovider.Request{
		Provider:      instance.Provider,
		Config:        config,
		OrderNo:       order.OrderNo,
		UserID:        order.UserID,
		Amount:        payableAmount(order),
		Currency:      order.Currency,
		PayerOpenID:   payloadString(order.Metadata, "payer_openid"),
		PayerClientIP: payloadString(order.Metadata, "payer_client_ip"),
		Product: checkoutprovider.Product{
			Type: string(order.ProductType),
			ID:   order.ProductID,
		},
		Metadata: map[string]any{
			"method": payloadString(order.ProviderSnapshot, "method"),
		},
	})
	if err != nil {
		if errors.Is(err, checkoutprovider.ErrUnavailable) {
			return contract.PaymentOrder{}, ErrProviderUnavailable
		}
		if errors.Is(err, checkoutprovider.ErrInvalidConfig) {
			return contract.PaymentOrder{}, fmt.Errorf("%w: %s provider %q: %v", ErrProviderConfigInvalid, instance.Provider, instance.Name, err)
		}
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	order.Metadata = cloneMap(order.Metadata)
	if session.ID != "" {
		order.Metadata["checkout_session_id"] = session.ID
	}
	if session.URL != "" {
		order.Metadata["checkout_url"] = session.URL
	}
	for key, value := range session.Metadata {
		order.Metadata[key] = value
	}
	order.UpdatedAt = s.clock.Now()
	return s.store.UpdateOrder(ctx, order)
}

type normalizedWebhook struct {
	OrderNo            string
	TransactionID      string
	Status             string
	IdempotencyKey     string
	Payload            map[string]any
	SignatureValid     bool
	ProviderInstanceID int
}

func (s *Service) normalizeWebhook(ctx context.Context, provider string, req contract.WebhookRequest) (normalizedWebhook, error) {
	if provider == "stripe" {
		return s.normalizeStripeWebhook(ctx, req)
	}
	if provider == "alipay" {
		return s.normalizeAlipayWebhook(ctx, req)
	}
	if provider == "wechat" {
		return s.normalizeWechatWebhook(ctx, req)
	}
	if provider == "linuxdo" || provider == "easypay" {
		return s.normalizeEpayWebhook(ctx, req)
	}
	return normalizedWebhook{
		OrderNo:        payloadString(req.Payload, "order_no"),
		TransactionID:  payloadString(req.Payload, "transaction_id", "trade_no", "provider_transaction_id"),
		Status:         normalizeProviderStatus(payloadString(req.Payload, "status", "trade_status")),
		IdempotencyKey: payloadString(req.Payload, "idempotency_key", "event_id"),
		Payload:        req.Payload,
	}, nil
}

func (s *Service) RequestRefund(ctx context.Context, req contract.RefundRequest) (contract.PaymentOrder, error) {
	if req.OrderID <= 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	order, err := s.store.FindOrderByID(ctx, req.OrderID)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	// One-shot refund: the order carries no cumulative-refunded accounting, so a
	// second refund (PartiallyRefunded -> Refunded, or another partial) would claw
	// back more balance than was ever paid. Reject once any refund has happened.
	if order.Status == contract.OrderStatusRefunded || order.Status == contract.OrderStatusPartiallyRefunded || order.Status == contract.OrderStatusRefunding {
		return contract.PaymentOrder{}, ErrInvalidTransition
	}
	refundAmount, ok := normalizeMoney(req.Amount)
	if strings.TrimSpace(req.Amount) == "" {
		refundAmount = order.Amount
		ok = true
	}
	if !ok || compareMoney(refundAmount, "0.00000000") <= 0 || compareMoney(refundAmount, order.Amount) > 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	if err := validateTransition(order.Status, contract.OrderStatusRefunding); err != nil {
		return contract.PaymentOrder{}, err
	}
	instance, err := s.store.FindProviderInstanceByID(ctx, order.ProviderInstanceID)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	provider, ok := s.deps.Checkout[instance.Provider]
	if !ok {
		return contract.PaymentOrder{}, ErrProviderUnavailable
	}
	config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	now := s.clock.Now()
	original := order
	order.Status = contract.OrderStatusRefunding
	order.Metadata = cloneMap(order.Metadata)
	order.Metadata["pending_refund_amount"] = refundAmount
	order.Metadata["pending_refund_reason"] = strings.TrimSpace(req.Reason)
	order.Metadata["pending_refund_provider_amount"] = payableRefundAmount(order, refundAmount)
	order.UpdatedAt = now
	refunding, err := s.store.UpdateOrder(ctx, order)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	s.recordPaymentAudit(ctx, refunding, "refund.requested", "refund_requested:"+refunding.OrderNo+":"+refundAmount, map[string]any{
		"order_no":               refunding.OrderNo,
		"refund_amount":          refundAmount,
		"provider_refund_amount": payloadString(refunding.Metadata, "pending_refund_provider_amount"),
		"reason":                 strings.TrimSpace(req.Reason),
	})
	refundResult, err := provider.Refund(checkoutprovider.RefundRequest{
		Provider:              instance.Provider,
		Config:                config,
		OrderNo:               refunding.OrderNo,
		ProviderTransactionID: stringValue(refunding.ProviderTransactionID),
		Amount:                payloadString(refunding.Metadata, "pending_refund_provider_amount"),
		OriginalAmount:        payableAmount(refunding),
		Currency:              refunding.Currency,
		Reason:                strings.TrimSpace(req.Reason),
		IdempotencyKey:        "refund:" + refunding.OrderNo + ":" + refundAmount,
		Metadata: map[string]any{
			"local_refund_amount": refundAmount,
			"payment_order_id":    refunding.ID,
		},
	})
	if err != nil {
		original.UpdatedAt = s.clock.Now()
		_, _ = s.store.UpdateOrder(ctx, original)
		s.recordPaymentAudit(ctx, original, "refund.failed", "refund_failed:"+original.OrderNo+":"+refundAmount, map[string]any{
			"order_no":      original.OrderNo,
			"refund_amount": refundAmount,
			"error":         err.Error(),
		})
		return contract.PaymentOrder{}, err
	}
	refunding.Metadata = cloneMap(refunding.Metadata)
	if refundResult.ID != "" {
		refunding.Metadata["pending_refund_id"] = refundResult.ID
	}
	for key, value := range refundResult.Metadata {
		refunding.Metadata[key] = value
	}
	refunding.UpdatedAt = s.clock.Now()
	refunding, err = s.store.UpdateOrder(ctx, refunding)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	switch refundResult.Status {
	case checkoutprovider.RefundStatusProcessing:
		return refunding, nil
	case checkoutprovider.RefundStatusFailed:
		return s.completePendingRefund(ctx, refunding, false, strings.TrimSpace(req.Reason))
	default:
		return s.completePendingRefund(ctx, refunding, true, strings.TrimSpace(req.Reason))
	}
}

func (s *Service) completePendingRefund(ctx context.Context, order contract.PaymentOrder, succeeded bool, reason string) (contract.PaymentOrder, error) {
	if order.Status != contract.OrderStatusRefunding {
		return contract.PaymentOrder{}, ErrInvalidTransition
	}
	refundAmount, ok := normalizeMoney(payloadString(order.Metadata, "pending_refund_amount"))
	if !ok || compareMoney(refundAmount, "0.00000000") <= 0 || compareMoney(refundAmount, order.Amount) > 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	now := s.clock.Now()
	if !succeeded {
		if err := validateTransition(order.Status, contract.OrderStatusRefundFailed); err != nil {
			return contract.PaymentOrder{}, err
		}
		order.Status = contract.OrderStatusRefundFailed
		order.UpdatedAt = now
		updated, err := s.store.UpdateOrder(ctx, order)
		if err != nil {
			return contract.PaymentOrder{}, err
		}
		s.recordPaymentAudit(ctx, updated, "refund.failed", "refund_failed:"+updated.OrderNo+":"+refundAmount, map[string]any{
			"order_no":      updated.OrderNo,
			"refund_amount": refundAmount,
			"reason":        strings.TrimSpace(reason),
		})
		return updated, nil
	}
	nextStatus := contract.OrderStatusRefunded
	if compareMoney(refundAmount, order.Amount) < 0 {
		nextStatus = contract.OrderStatusPartiallyRefunded
	}
	if err := validateTransition(order.Status, nextStatus); err != nil {
		return contract.PaymentOrder{}, err
	}
	// Claw back the spendable balance BEFORE persisting the refunded status. Only
	// balance_credit orders ever credited balance, so only those are debited. If
	// the debit fails (e.g. the user already spent the funds), the order remains
	// refunding for operator follow-up instead of being marked refunded.
	if order.ProductType == contract.ProductTypeBalanceCredit && s.deps.Balance != nil {
		if err := s.deps.Balance.DebitBalance(ctx, order.UserID, refundAmount, order.Currency); err != nil {
			return contract.PaymentOrder{}, err
		}
	}
	order.Status = nextStatus
	order.ClosedAt = &now
	order.UpdatedAt = now
	updated, err := s.store.UpdateOrder(ctx, order)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	negativeAmount := "-" + refundAmount
	_, _ = s.recordBilling(ctx, billingcontract.RecordRequest{
		UserID:        order.UserID,
		Type:          billingcontract.LedgerTypeRefund,
		Amount:        negativeAmount,
		Currency:      order.Currency,
		ReferenceType: "payment_order",
		ReferenceID:   order.OrderNo,
		Metadata: map[string]any{
			"payment_order_id": order.ID,
			"refund_reason":    strings.TrimSpace(reason),
			"refund_amount":    refundAmount,
		},
	})
	s.recordAudit(ctx, nil, "payment_order.refund", "payment_order", strconv.Itoa(order.ID), paymentOrderAuditSnapshot(order), paymentOrderAuditSnapshot(updated))
	s.recordPaymentAudit(ctx, updated, "refund.succeeded", "refund_succeeded:"+updated.OrderNo+":"+refundAmount, map[string]any{
		"order_no":      updated.OrderNo,
		"refund_amount": refundAmount,
		"reason":        strings.TrimSpace(reason),
	})
	s.enqueueRefunded(ctx, updated, refundAmount, reason)
	return updated, nil
}

func (s *Service) markPaidAndFulfill(ctx context.Context, order contract.PaymentOrder, instance contract.PaymentProviderInstance, transactionID string) (contract.PaymentOrder, error) {
	now := s.clock.Now()
	if err := validateTransition(order.Status, contract.OrderStatusPaid); err != nil {
		return contract.PaymentOrder{}, err
	}
	order.Status = contract.OrderStatusPaid
	order.ProviderTransactionID = stringPtr(strings.TrimSpace(transactionID))
	order.PaidAt = &now
	order.UpdatedAt = now
	paid, err := s.store.UpdateOrder(ctx, order)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	if err := s.fulfill(ctx, paid, instance); err != nil {
		return paid, err
	}
	if err := validateTransition(paid.Status, contract.OrderStatusFulfilled); err != nil {
		return contract.PaymentOrder{}, err
	}
	paid.Status = contract.OrderStatusFulfilled
	paid.UpdatedAt = s.clock.Now()
	fulfilled, err := s.store.UpdateOrder(ctx, paid)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	s.recordAudit(ctx, nil, "payment_order.fulfill", "payment_order", strconv.Itoa(order.ID), paymentOrderAuditSnapshot(order), paymentOrderAuditSnapshot(fulfilled))
	s.enqueuePaid(ctx, fulfilled, instance)
	return fulfilled, nil
}

func (s *Service) markFailed(ctx context.Context, order contract.PaymentOrder) (contract.PaymentOrder, error) {
	if order.Status != contract.OrderStatusPending {
		return order, nil
	}
	if err := validateTransition(order.Status, contract.OrderStatusFailed); err != nil {
		return contract.PaymentOrder{}, err
	}
	now := s.clock.Now()
	order.Status = contract.OrderStatusFailed
	order.ClosedAt = &now
	order.UpdatedAt = now
	updated, err := s.store.UpdateOrder(ctx, order)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	if updated.PromoCodeID != nil {
		if _, _, err := s.store.ReleasePromoCode(ctx, contract.PromoCodeReleaseInput{PaymentOrderID: updated.ID, ReleasedAt: now, Reason: "order_failed"}); err != nil && !errors.Is(err, contract.ErrNotFound) {
			return contract.PaymentOrder{}, err
		}
	}
	s.recordPaymentAudit(ctx, updated, "order.failed", "order_failed:"+updated.OrderNo, map[string]any{
		"order_id":  updated.ID,
		"order_no":  updated.OrderNo,
		"failed_at": now.Format(time.RFC3339Nano),
	})
	return updated, nil
}

func (s *Service) ReconcilePendingOrders(ctx context.Context, now time.Time) (contract.ReconcileOrdersResult, error) {
	if now.IsZero() {
		now = s.clock.Now()
	}
	orders, err := s.store.ListPendingOrders(ctx, now)
	if err != nil {
		return contract.ReconcileOrdersResult{}, err
	}
	result := contract.ReconcileOrdersResult{Selected: len(orders)}
	for _, order := range orders {
		instance, err := s.store.FindProviderInstanceByID(ctx, order.ProviderInstanceID)
		if err != nil {
			continue
		}
		provider, ok := s.deps.Checkout[instance.Provider]
		if !ok {
			continue
		}
		config, err := s.decryptConfig(instance, instance.ConfigCiphertext)
		if err != nil {
			continue
		}
		query, err := provider.QueryOrder(checkoutprovider.QueryRequest{
			Provider:              instance.Provider,
			Config:                config,
			OrderNo:               order.OrderNo,
			ProviderTransactionID: stringValue(order.ProviderTransactionID),
			Amount:                payableAmount(order),
			Currency:              order.Currency,
			Metadata:              cloneMap(order.Metadata),
		})
		if err != nil {
			continue
		}
		s.recordPaymentAudit(ctx, order, "reconcile.query", "reconcile:"+order.OrderNo+":"+string(query.Status), map[string]any{
			"order_no":                order.OrderNo,
			"status":                  string(query.Status),
			"provider_transaction_id": query.ProviderTransactionID,
			"amount":                  query.Amount,
			"currency":                query.Currency,
		})
		switch query.Status {
		case checkoutprovider.QueryStatusPaid:
			fulfilled, err := s.markPaidAndFulfill(ctx, order, instance, query.ProviderTransactionID)
			if err != nil {
				continue
			}
			s.recordPaymentAudit(ctx, fulfilled, "reconcile.fulfilled", "reconcile_fulfilled:"+fulfilled.OrderNo, map[string]any{
				"order_no":                fulfilled.OrderNo,
				"provider_transaction_id": stringValue(fulfilled.ProviderTransactionID),
			})
			result.Paid++
		case checkoutprovider.QueryStatusFailed, checkoutprovider.QueryStatusCanceled:
			if _, err := s.markFailed(ctx, order); err != nil {
				continue
			}
			result.Failed++
		}
	}
	return result, nil
}

func (s *Service) fulfill(ctx context.Context, order contract.PaymentOrder, instance contract.PaymentProviderInstance) error {
	_, err := s.recordBilling(ctx, billingcontract.RecordRequest{
		UserID:        order.UserID,
		Type:          billingcontract.LedgerTypePaymentCredit,
		Amount:        order.Amount,
		Currency:      order.Currency,
		ReferenceType: "payment_order",
		ReferenceID:   order.OrderNo,
		Metadata: map[string]any{
			"payment_order_id":          order.ID,
			"payment_provider":          instance.Provider,
			"payment_provider_instance": instance.ID,
			"product_type":              string(order.ProductType),
			"product_id":                order.ProductID,
		},
	})
	if err != nil {
		return err
	}
	if order.ProductType == contract.ProductTypeBalanceCredit {
		// Make the paid amount spendable. Idempotent per order: fulfill only runs
		// on the Paid->Fulfilled transition, which validateTransition gates.
		if s.deps.Balance == nil {
			return ErrInvalidInput
		}
		return s.deps.Balance.CreditBalance(ctx, order.UserID, order.Amount, order.Currency)
	}
	if order.ProductType != contract.ProductTypeSubscriptionPlan {
		return nil
	}
	planID, err := strconv.Atoi(strings.TrimSpace(order.ProductID))
	if err != nil || planID <= 0 {
		return ErrInvalidInput
	}
	if s.deps.Subscriptions == nil {
		return ErrInvalidInput
	}
	_, err = s.deps.Subscriptions.CreateUserSubscription(ctx, subscriptioncontract.CreateSubscriptionRequest{
		UserID:     order.UserID,
		PlanID:     planID,
		SourceType: "payment_order",
		SourceID:   order.OrderNo,
	})
	return err
}

func (s *Service) recordBilling(ctx context.Context, req billingcontract.RecordRequest) (billingcontract.LedgerEntry, error) {
	if s.deps.Billing == nil {
		return billingcontract.LedgerEntry{}, nil
	}
	return s.deps.Billing.Record(ctx, req)
}

func (s *Service) recordAudit(ctx context.Context, actorUserID *int, action string, resourceType string, resourceID string, before map[string]any, after map[string]any) {
	if s.deps.Audit == nil {
		return
	}
	_, _ = s.deps.Audit.Record(ctx, auditcontract.RecordRequest{
		ActorUserID:  actorUserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Before:       before,
		After:        after,
	})
}

func (s *Service) recordPaymentAudit(ctx context.Context, order contract.PaymentOrder, eventType string, idempotencyKey string, payload map[string]any) {
	_, _, _ = s.store.CreateAuditLog(ctx, contract.PaymentAuditLog{
		OrderID:            order.ID,
		ProviderInstanceID: order.ProviderInstanceID,
		EventType:          eventType,
		IdempotencyKey:     idempotencyKey,
		Payload:            sanitizePayload(payload),
		SignatureValid:     true,
		CreatedAt:          s.clock.Now(),
	})
}

func (s *Service) enqueuePaid(ctx context.Context, order contract.PaymentOrder, instance contract.PaymentProviderInstance) {
	if s.deps.Events == nil {
		return
	}
	_, _ = s.deps.Events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "PaymentOrderPaid",
		EventVersion:   "v1",
		ProducerModule: "payments",
		AggregateType:  "payment_order",
		AggregateID:    order.OrderNo,
		IdempotencyKey: "payment_paid:" + order.OrderNo,
		Payload: map[string]any{
			"order_id":                order.ID,
			"order_no":                order.OrderNo,
			"user_id":                 order.UserID,
			"provider":                instance.Provider,
			"provider_instance_id":    instance.ID,
			"amount":                  order.Amount,
			"currency":                order.Currency,
			"product_type":            string(order.ProductType),
			"product_id":              order.ProductID,
			"paid_at":                 timeValue(order.PaidAt),
			"provider_transaction_id": stringValue(order.ProviderTransactionID),
		},
	})
}

func (s *Service) enqueueRefunded(ctx context.Context, order contract.PaymentOrder, refundAmount string, reason string) {
	if s.deps.Events == nil {
		return
	}
	refundID := "refund_" + order.OrderNo + "_" + strings.ReplaceAll(refundAmount, ".", "_")
	_, _ = s.deps.Events.Enqueue(ctx, eventscontract.EnqueueRequest{
		EventType:      "PaymentOrderRefunded",
		EventVersion:   "v1",
		ProducerModule: "payments",
		AggregateType:  "payment_order",
		AggregateID:    order.OrderNo,
		IdempotencyKey: "payment_refunded:" + order.OrderNo + ":" + refundAmount,
		Payload: map[string]any{
			"order_id":      order.ID,
			"refund_id":     refundID,
			"user_id":       order.UserID,
			"amount":        refundAmount,
			"currency":      order.Currency,
			"refund_reason": strings.TrimSpace(reason),
			"refunded_at":   timeValue(order.ClosedAt),
		},
	})
}

func (s *Service) selectProviderInstance(ctx context.Context, method string, amount string, currency string) (contract.PaymentProviderInstance, error) {
	instances, err := s.store.ListProviderInstances(ctx)
	if err != nil {
		return contract.PaymentProviderInstance{}, err
	}
	method = strings.TrimSpace(method)
	currency = normalizeCurrency(currency)
	var candidates []contract.PaymentProviderInstance
	for _, instance := range instances {
		if instance.Status != contract.ProviderStatusActive {
			continue
		}
		if !supportsMethod(instance, method) {
			continue
		}
		if !withinLimits(instance.Limits, amount, currency) {
			continue
		}
		candidates = append(candidates, instance)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SortOrder == candidates[j].SortOrder {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].SortOrder < candidates[j].SortOrder
	})
	if len(candidates) == 0 {
		return contract.PaymentProviderInstance{}, ErrProviderUnavailable
	}
	return s.nextProviderCandidate(method, candidates), nil
}

func (s *Service) nextProviderCandidate(method string, candidates []contract.PaymentProviderInstance) contract.PaymentProviderInstance {
	if len(candidates) == 1 {
		return candidates[0]
	}
	s.selectorMu.Lock()
	defer s.selectorMu.Unlock()
	key := strings.ToLower(strings.TrimSpace(method))

	state := s.selectorState[key]
	if len(state) != len(candidates) {
		state = make([]int, len(candidates))
		s.selectorState[key] = state
	}

	totalWeight := 0
	for _, c := range candidates {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	bestIdx := 0
	bestWeight := state[0]
	for i, c := range candidates {
		w := c.Weight
		if w <= 0 {
			w = 1
		}
		state[i] += w
		if state[i] > bestWeight {
			bestWeight = state[i]
			bestIdx = i
		}
	}
	state[bestIdx] -= totalWeight
	return candidates[bestIdx]
}

func (s *Service) encryptConfig(provider string, name string, payload map[string]any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	aad := configAAD(provider, name)
	ciphertext := gcm.Seal(nil, nonce, raw, aad)
	return fmt.Sprintf("%s:%s:%s", configCiphertextV1, base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(ciphertext)), nil
}

func (s *Service) decryptConfig(instance contract.PaymentProviderInstance, ciphertext string) (map[string]any, error) {
	parts := strings.Split(ciphertext, ":")
	if len(parts) != 3 || parts[0] != configCiphertextV1 {
		return nil, ErrInvalidInput
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(s.masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	raw, err := gcm.Open(nil, nonce, encrypted, configAAD(instance.Provider, instance.Name))
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func configAAD(provider string, name string) []byte {
	return []byte("resource_type=payment_provider_instance;resource_id=" + provider + "/" + name + ";field_name=config;key_version=v1")
}

func verifyWebhookSignature(config map[string]any, headers map[string]string, payload map[string]any) bool {
	secret := strings.TrimSpace(payloadString(config, "webhook_secret", "secret", "signing_secret", "key"))
	if secret == "" {
		return false
	}
	signature := strings.TrimSpace(headers["X-SRapi-Payment-Signature"])
	if signature == "" {
		signature = strings.TrimSpace(headers["X-EasyPay-Signature"])
	}
	if signature == "" {
		signature = payloadString(payload, "signature", "sign")
	}
	if signature == "" {
		return false
	}
	// Try HMAC-SHA256 first (SRapi native), then MD5 (EasyPay / LinuxDo Credit).
	expected := signWebhookPayload(secret, payload)
	if hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected)) {
		return true
	}
	expectedMD5 := signWebhookPayloadMD5(secret, payload)
	return strings.EqualFold(signature, expectedMD5)
}

func signWebhookPayload(secret string, payload map[string]any) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonicalPayload(payload)))
	return hex.EncodeToString(mac.Sum(nil))
}

func signWebhookPayloadMD5(secret string, payload map[string]any) string {
	sum := md5.Sum([]byte(canonicalPayload(payload) + secret))
	return hex.EncodeToString(sum[:])
}

func canonicalPayload(payload map[string]any) string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" || normalized == "signature" || normalized == "sign" || normalized == "sign_type" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fmt.Sprint(payload[key]))
	}
	return strings.Join(parts, "&")
}

func verifyWebhookOrder(order contract.PaymentOrder, payload map[string]any) error {
	amountStr := payloadString(payload, "amount", "money")
	if amountStr != "" {
		amount, ok := normalizeMoney(amountStr)
		if !ok || amount != payableAmount(order) {
			return ErrOrderMismatch
		}
	}
	currency := normalizeCurrency(payloadString(payload, "currency"))
	if currency != "" && currency != order.Currency {
		return ErrOrderMismatch
	}
	return nil
}

func validateTransition(from contract.OrderStatus, to contract.OrderStatus) error {
	if from == to {
		return nil
	}
	switch from {
	case contract.OrderStatusPending:
		switch to {
		case contract.OrderStatusPaid, contract.OrderStatusExpired, contract.OrderStatusCanceled, contract.OrderStatusFailed:
			return nil
		}
	case contract.OrderStatusPaid:
		switch to {
		case contract.OrderStatusFulfilled, contract.OrderStatusPartiallyRefunded, contract.OrderStatusRefunding, contract.OrderStatusRefunded:
			return nil
		}
	case contract.OrderStatusFulfilled:
		switch to {
		case contract.OrderStatusPartiallyRefunded, contract.OrderStatusRefunding, contract.OrderStatusRefunded:
			return nil
		}
	case contract.OrderStatusRefunding:
		switch to {
		case contract.OrderStatusPartiallyRefunded, contract.OrderStatusRefunded, contract.OrderStatusRefundFailed:
			return nil
		}
	case contract.OrderStatusRefundFailed:
		if to == contract.OrderStatusRefunding {
			return nil
		}
	case contract.OrderStatusPartiallyRefunded:
		if to == contract.OrderStatusRefunding || to == contract.OrderStatusRefunded {
			return nil
		}
	}
	return ErrInvalidTransition
}

func normalizeProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "trade_success", "finished":
		return "paid"
	case "refunded", "refund_success", "refund_succeeded", "refund.success", "refund_successful":
		return "refunded"
	case "refund_failed", "refund.fail", "refund_failure":
		return "refund_failed"
	case "failed", "failure":
		return "failed"
	case "canceled", "cancelled", "closed":
		return "canceled"
	default:
		return ""
	}
}

func validProviderStatus(status contract.ProviderStatus) bool {
	switch status {
	case contract.ProviderStatusActive, contract.ProviderStatusDisabled, contract.ProviderStatusArchived:
		return true
	default:
		return false
	}
}

func validProduct(productType contract.ProductType, productID string) bool {
	switch productType {
	case contract.ProductTypeBalanceCredit:
		return true
	case contract.ProductTypeSubscriptionPlan:
		_, err := strconv.Atoi(strings.TrimSpace(productID))
		return err == nil
	default:
		return false
	}
}

func normalizeMethods(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func missingProviderConfigFields(provider string, config map[string]any) []string {
	var missing []string
	requireAny := func(label string, keys ...string) {
		if payloadString(config, keys...) == "" {
			missing = append(missing, label)
		}
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "stripe":
		requireAny("config.secret_key", "secret_key", "api_key")
		requireAny("config.success_url", "success_url")
		requireAny("config.cancel_url", "cancel_url")
		requireAny("config.webhook_secret", "webhook_secret", "signing_secret")
	case "alipay":
		requireAny("config.app_id", "app_id", "appId")
		requireAny("config.private_key", "private_key", "app_private_key")
		requireAny("config.alipay_public_key", "alipay_public_key", "public_key")
		requireAny("config.notify_url", "notify_url", "webhook_url")
		requireAny("config.return_url", "return_url", "success_url")
	case "easypay":
		requireAny("config.gateway_url", "gateway_url", "base_url", "payment_url")
		requireAny("config.merchant_id", "merchant_id", "pid")
		requireAny("config.signing_secret", "signing_secret", "webhook_secret", "key", "secret")
		requireAny("config.notify_url", "notify_url", "webhook_url")
		requireAny("config.return_url", "return_url", "success_url")
	case "wechat":
		requireAny("config.app_id", "app_id", "appid")
		requireAny("config.mch_id", "mch_id", "mchid", "merchant_id")
		requireAny("config.api_v3_key", "api_v3_key", "apiV3Key")
		requireAny("config.serial_no", "serial_no", "certificate_serial_no", "mch_certificate_serial_no")
		requireAny("config.private_key", "private_key", "merchant_private_key")
		requireAny("config.notify_url", "notify_url", "webhook_url")
	default:
		requireAny("config.webhook_secret", "webhook_secret", "signing_secret", "secret")
	}
	return missing
}

func supportsMethod(instance contract.PaymentProviderInstance, method string) bool {
	method = strings.ToLower(strings.TrimSpace(method))
	for _, candidate := range normalizeMethods(instance.SupportedMethods) {
		if candidate == method {
			return true
		}
	}
	return false
}

func withinLimits(limits map[string]any, amount string, currency string) bool {
	if len(limits) == 0 {
		return true
	}
	if limitCurrency := strings.TrimSpace(payloadString(limits, "currency")); limitCurrency != "" && normalizeCurrency(limitCurrency) != currency {
		return false
	}
	if minAmount := payloadString(limits, "min_amount"); minAmount != "" && compareMoney(amount, defaultMoney(minAmount)) < 0 {
		return false
	}
	if maxAmount := payloadString(limits, "max_amount"); maxAmount != "" && compareMoney(amount, defaultMoney(maxAmount)) > 0 {
		return false
	}
	return true
}

func applyFeeRate(amount string, feeRate string) (string, string, error) {
	amountRat, ok := money.DecimalRat(amount)
	if !ok || amountRat.Sign() < 0 {
		return "", "", ErrInvalidInput
	}
	rateRat, ok := money.DecimalRat(defaultMoney(feeRate))
	if !ok || rateRat.Sign() < 0 {
		return "", "", ErrInvalidInput
	}
	feeRat := new(big.Rat).Mul(amountRat, rateRat)
	feeAmount := money.FormatRatFixed(feeRat, 8)
	payableRat := new(big.Rat).Add(amountRat, feeRat)
	return feeAmount, money.FormatRatFixed(payableRat, 8), nil
}

func payableAmount(order contract.PaymentOrder) string {
	if strings.TrimSpace(order.PayableAmount) != "" {
		return defaultMoney(order.PayableAmount)
	}
	return defaultMoney(order.Amount)
}

func payableRefundAmount(order contract.PaymentOrder, refundAmount string) string {
	if compareMoney(refundAmount, order.Amount) == 0 {
		return payableAmount(order)
	}
	refundRat, ok := money.DecimalRat(defaultMoney(refundAmount))
	if !ok {
		return defaultMoney(refundAmount)
	}
	amountRat, ok := money.DecimalRat(defaultMoney(order.Amount))
	if !ok || amountRat.Sign() == 0 {
		return defaultMoney(refundAmount)
	}
	payableRat, ok := money.DecimalRat(payableAmount(order))
	if !ok {
		return defaultMoney(refundAmount)
	}
	ratio := new(big.Rat).Quo(refundRat, amountRat)
	return money.FormatRatFixed(new(big.Rat).Mul(payableRat, ratio), 8)
}

func normalizeRatePtr(value *string) (string, error) {
	if value == nil {
		return "0.00000000", nil
	}
	normalized, ok := normalizeMoney(*value)
	if !ok {
		return "", ErrInvalidInput
	}
	return normalized, nil
}

func defaultProviderWeight(value *int) int {
	if value == nil {
		return 1
	}
	if *value <= 0 {
		return 0
	}
	return *value
}

func normalizeMoney(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	rat, ok := money.RequiredDecimalRat(value)
	if !ok {
		return "", false
	}
	if rat.Sign() < 0 {
		return "", false
	}
	return money.FormatRatFixed(rat, 8), true
}

func normalizePromoCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func defaultMoney(value string) string {
	normalized, ok := normalizeMoney(value)
	if !ok {
		return "0.00000000"
	}
	return normalized
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func compareMoney(left string, right string) int {
	leftRat, ok := money.DecimalRat(defaultMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := money.DecimalRat(defaultMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Cmp(rightRat)
}

func normalizeCurrency(value string) string {
	return money.NormalizeCurrency(value)
}

func newOrderNo() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("pay_%d", time.Now().UnixNano())
	}
	return "pay_" + hex.EncodeToString(bytes[:])
}

func sanitizePayload(value map[string]any) map[string]any {
	payload := cloneMap(value)
	for _, key := range []string{"signature", "sign", "secret", "webhook_secret", "token", "password"} {
		if _, ok := payload[key]; ok {
			payload[key] = "[redacted]"
		}
	}
	return payload
}

func publicProviderMetadata(value map[string]any) map[string]any {
	metadata := sanitizePayload(value)
	delete(metadata, "config_ciphertext")
	return metadata
}

func paymentOrderAuditSnapshot(order contract.PaymentOrder) map[string]any {
	return map[string]any{
		"id":                      order.ID,
		"order_no":                order.OrderNo,
		"user_id":                 order.UserID,
		"provider_instance_id":    order.ProviderInstanceID,
		"amount":                  order.Amount,
		"currency":                order.Currency,
		"status":                  string(order.Status),
		"product_type":            string(order.ProductType),
		"product_id":              order.ProductID,
		"provider_transaction_id": stringValue(order.ProviderTransactionID),
	}
}

func payloadString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case json.Number:
			return typed.String()
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		default:
			return strings.TrimSpace(fmt.Sprint(typed))
		}
	}
	return ""
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

func stringPtr(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func timeValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
