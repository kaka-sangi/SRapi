package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
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
	"time"

	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	eventscontract "github.com/srapi/srapi/apps/api/internal/modules/events/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	stripeprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/stripe"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	platformcrypto "github.com/srapi/srapi/apps/api/internal/platform/crypto"
	platformotel "github.com/srapi/srapi/apps/api/internal/platform/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	configVersionV1       = 1
	configCiphertextV1    = "v1"
	defaultCurrency       = "USD"
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

type Dependencies struct {
	Billing       BillingRecorder
	Subscriptions SubscriptionActivator
	Audit         AuditRecorder
	Events        EventEnqueuer
	Checkout      checkoutprovider.Registry
	Stripe        stripeprovider.CheckoutCreator
}

type Service struct {
	store     contract.Store
	masterKey []byte
	deps      Dependencies
	clock     Clock
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
	return &Service{store: store, masterKey: derivedKey, deps: deps, clock: clock}, nil
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
	if provider.DeletedAt != nil {
		return contract.PaymentProviderInstance{}, contract.ErrNotFound
	}

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
	if req.Metadata != nil {
		provider.Metadata = cloneMap(*req.Metadata)
	}
	if req.Config != nil {
		if len(*req.Config) == 0 {
			return contract.PaymentProviderInstance{}, ErrInvalidInput
		}
		config = cloneMap(*req.Config)
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
		if instance.Status != contract.ProviderStatusActive || instance.DeletedAt != nil {
			continue
		}
		for _, method := range normalizeMethods(instance.SupportedMethods) {
			out = append(out, contract.PaymentMethod{
				Method:             method,
				Provider:           instance.Provider,
				ProviderInstanceID: instance.ID,
				Name:               instance.Name,
				Metadata:           publicProviderMetadata(instance.Metadata),
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
	if !validProduct(req.ProductType, req.ProductID) {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	instance, err := s.selectProviderInstance(ctx, req.Method, amount, currency)
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
	order, err := s.store.CreateOrder(ctx, contract.CreateStoredOrder{
		UserID:             req.UserID,
		OrderNo:            orderNo,
		ProviderInstanceID: instance.ID,
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
			"metadata":             publicProviderMetadata(instance.Metadata),
		},
		ExpiresAt: &expiresAt,
		Metadata:  cloneMap(req.Metadata),
	})
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	return s.attachProviderCheckout(ctx, order, instance)
}

func (s *Service) ListOrders(ctx context.Context) ([]contract.PaymentOrder, error) {
	return s.store.ListOrders(ctx)
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

func (s *Service) CancelOrder(ctx context.Context, userID int, orderID int) (contract.PaymentOrder, error) {
	if userID <= 0 || orderID <= 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	order, err := s.store.FindOrderByID(ctx, orderID)
	if err != nil {
		return contract.PaymentOrder{}, err
	}
	if order.UserID != userID {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	if err := validateTransition(order.Status, contract.OrderStatusCanceled); err != nil {
		return contract.PaymentOrder{}, err
	}
	now := s.clock.Now()
	order.Status = contract.OrderStatusCanceled
	order.ClosedAt = &now
	order.UpdatedAt = now
	return s.store.UpdateOrder(ctx, order)
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
		if err := validateTransition(order.Status, contract.OrderStatusExpired); err != nil {
			return result, err
		}
		updated, expired, err := s.store.ExpireOrder(ctx, order.ID, now)
		if err != nil {
			return result, err
		}
		if !expired {
			continue
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
		if !auditLog.SignatureValid {
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
		return contract.PaymentOrder{}, err
	}
	session, err := provider.CreateSession(checkoutprovider.Request{
		Provider: instance.Provider,
		Config:   config,
		OrderNo:  order.OrderNo,
		UserID:   order.UserID,
		Amount:   order.Amount,
		Currency: order.Currency,
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
	refundAmount, ok := normalizeMoney(req.Amount)
	if strings.TrimSpace(req.Amount) == "" {
		refundAmount = order.Amount
		ok = true
	}
	if !ok || compareMoney(refundAmount, "0.00000000") <= 0 || compareMoney(refundAmount, order.Amount) > 0 {
		return contract.PaymentOrder{}, ErrInvalidInput
	}
	nextStatus := contract.OrderStatusRefunded
	if compareMoney(refundAmount, order.Amount) < 0 {
		nextStatus = contract.OrderStatusPartiallyRefunded
	}
	if err := validateTransition(order.Status, nextStatus); err != nil {
		return contract.PaymentOrder{}, err
	}
	now := s.clock.Now()
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
			"refund_reason":    strings.TrimSpace(req.Reason),
			"refund_amount":    refundAmount,
		},
	})
	s.recordAudit(ctx, &req.ActorUserID, "payment_order.refund", "payment_order", strconv.Itoa(order.ID), paymentOrderAuditSnapshot(order), paymentOrderAuditSnapshot(updated))
	s.enqueueRefunded(ctx, updated, refundAmount, req.Reason)
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
		if instance.Status != contract.ProviderStatusActive || instance.DeletedAt != nil {
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
	return candidates[0], nil
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
	secret := strings.TrimSpace(payloadString(config, "webhook_secret", "secret", "signing_secret"))
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
	expected := signWebhookPayload(secret, payload)
	return hmac.Equal([]byte(strings.ToLower(signature)), []byte(expected))
}

func signWebhookPayload(secret string, payload map[string]any) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonicalPayload(payload)))
	return hex.EncodeToString(mac.Sum(nil))
}

func canonicalPayload(payload map[string]any) string {
	keys := make([]string, 0, len(payload))
	for key := range payload {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" || normalized == "signature" || normalized == "sign" {
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
	amount, ok := normalizeMoney(payloadString(payload, "amount", "money"))
	if !ok || amount != order.Amount {
		return ErrOrderMismatch
	}
	currency := normalizeCurrency(payloadString(payload, "currency"))
	if currency != order.Currency {
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
		case contract.OrderStatusFulfilled, contract.OrderStatusPartiallyRefunded, contract.OrderStatusRefunded:
			return nil
		}
	case contract.OrderStatusFulfilled:
		switch to {
		case contract.OrderStatusPartiallyRefunded, contract.OrderStatusRefunded:
			return nil
		}
	case contract.OrderStatusPartiallyRefunded:
		if to == contract.OrderStatusRefunded {
			return nil
		}
	}
	return ErrInvalidTransition
}

func normalizeProviderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "trade_success", "finished":
		return "paid"
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

func normalizeMoney(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	rat, ok := decimalRat(value)
	if !ok {
		return "", false
	}
	if rat.Sign() < 0 {
		return "", false
	}
	return formatRatFixed(rat, 8), true
}

func defaultMoney(value string) string {
	normalized, ok := normalizeMoney(value)
	if !ok {
		return "0.00000000"
	}
	return normalized
}

func decimalRat(value string) (*big.Rat, bool) {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "eE") {
		return nil, false
	}
	rat := new(big.Rat)
	if _, ok := rat.SetString(value); !ok {
		return nil, false
	}
	return rat, true
}

func formatRatFixed(value *big.Rat, places int) string {
	if value == nil {
		value = new(big.Rat)
	}
	return value.FloatString(places)
}

func compareMoney(left string, right string) int {
	leftRat, ok := decimalRat(defaultMoney(left))
	if !ok {
		leftRat = new(big.Rat)
	}
	rightRat, ok := decimalRat(defaultMoney(right))
	if !ok {
		rightRat = new(big.Rat)
	}
	return leftRat.Cmp(rightRat)
}

func normalizeCurrency(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return defaultCurrency
	}
	return value
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
