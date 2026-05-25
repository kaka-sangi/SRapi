package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	stripeprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/stripe"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	"github.com/srapi/srapi/apps/api/internal/testsupport/oteltest"
	"github.com/stripe/stripe-go/v78/webhook"
	"go.opentelemetry.io/otel/codes"
)

const testMasterKey = "payment_master_key_32_bytes_minimum_value"

func TestPaymentWebhookFulfillsSubscriptionOrderIdempotently(t *testing.T) {
	h := newHarness(t)
	plan, err := h.subscriptions.CreatePlan(t.Context(), subscriptioncontract.CreatePlanRequest{
		Name:         "commercial-pro",
		Price:        "19.99",
		Currency:     "usd",
		ValidityDays: 30,
		Entitlements: map[string]any{"allowed_models": []any{"commercial-model"}},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           easypayTestConfig("provider-signing-secret"),
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD", "min_amount": "1.00", "max_amount": "100.00"},
		Metadata:         map[string]any{"display_name": "AliPay"},
	})
	if err != nil {
		t.Fatalf("create provider instance: %v", err)
	}
	if provider.ConfigCiphertext == "" || strings.Contains(provider.ConfigCiphertext, "provider-signing-secret") {
		t.Fatalf("payment config must be encrypted without plaintext leak: %+v", provider)
	}

	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "alipay",
		Amount:      "19.990",
		Currency:    "usd",
		ProductType: contract.ProductTypeSubscriptionPlan,
		ProductID:   strconv.Itoa(plan.ID),
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if order.Amount != "19.99000000" || order.Currency != "USD" || order.ProviderInstanceID != provider.ID || order.ProductID != strconv.Itoa(plan.ID) {
		t.Fatalf("unexpected normalized order: %+v", order)
	}

	payload := map[string]any{
		"order_no":        order.OrderNo,
		"amount":          "19.99000000",
		"currency":        "USD",
		"status":          "paid",
		"transaction_id":  "txn_123",
		"idempotency_key": "evt_paid_1",
	}
	result, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "easypay",
		Headers:  map[string]string{"X-SRapi-Payment-Signature": signWebhookPayload("provider-signing-secret", payload)},
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("handle webhook: %v", err)
	}
	if !result.Handled || result.Order.Status != contract.OrderStatusFulfilled {
		t.Fatalf("expected fulfilled order from first webhook, got %+v", result)
	}

	ledger, err := h.billing.List(t.Context())
	if err != nil {
		t.Fatalf("list billing ledger: %v", err)
	}
	if len(ledger) != 1 || ledger[0].Type != billingcontract.LedgerTypePaymentCredit || ledger[0].ReferenceID != order.OrderNo {
		t.Fatalf("expected one payment credit ledger entry, got %+v", ledger)
	}
	subs, err := h.subscriptions.ListUserSubscriptionsByUser(t.Context(), 1)
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != 1 || subs[0].SourceType != "payment_order" || subs[0].SourceID != order.OrderNo {
		t.Fatalf("expected subscription fulfillment linked to order, got %+v", subs)
	}
	outbox, err := h.events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != 1 || outbox[0].EventType != "PaymentOrderPaid" || outbox[0].ProducerModule != "payments" {
		t.Fatalf("expected payment paid outbox event, got %+v", outbox)
	}
	audits, err := h.store.ListAuditLogsByOrder(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("list payment audit logs: %v", err)
	}
	if len(audits) != 1 || !audits[0].SignatureValid || strings.Contains(mustJSON(t, audits[0].Payload), "provider-signing-secret") {
		t.Fatalf("expected signed secret-free payment audit log, got %+v", audits)
	}

	duplicate, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "easypay",
		Headers:  map[string]string{"X-SRapi-Payment-Signature": signWebhookPayload("provider-signing-secret", payload)},
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("handle duplicate webhook: %v", err)
	}
	if duplicate.Handled {
		t.Fatalf("duplicate webhook should be idempotent, got %+v", duplicate)
	}
	assertCounts(t, h, 1, 1, 1, 1)
}

func TestPaymentWebhookRejectsInvalidSignatureFailClosed(t *testing.T) {
	exporter := oteltest.NewExporter(t)
	h := newHarness(t)
	_, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           easypayTestConfig("provider-signing-secret"),
		SupportedMethods: []string{"alipay"},
	})
	if err != nil {
		t.Fatalf("create provider instance: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "alipay",
		Amount:      "10.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	payload := map[string]any{
		"order_no":        order.OrderNo,
		"amount":          "10.00000000",
		"currency":        "USD",
		"status":          "paid",
		"transaction_id":  "txn_bad",
		"idempotency_key": "evt_bad_signature",
	}
	if _, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "easypay",
		Headers:  map[string]string{"X-SRapi-Payment-Signature": "bad-signature"},
		Payload:  payload,
	}); !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("expected invalid signature rejection, got %v", err)
	}
	orders, err := h.payments.ListOrdersByUser(t.Context(), 1)
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}
	if len(orders) != 1 || orders[0].Status != contract.OrderStatusPending {
		t.Fatalf("invalid signature must not mutate order, got %+v", orders)
	}
	span := oteltest.FindSpan(t, exporter.GetSpans(), "payments.HandleWebhook")
	if span.Status.Code != codes.Error {
		t.Fatalf("expected payment webhook span error status, got %+v", span.Status)
	}
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.payment.provider", "easypay")
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.payment.webhook_outcome", "error")
	oteltest.AssertStringAttr(t, span.Attributes, "error.type", "signature_invalid")
	assertCounts(t, h, 0, 0, 0, 1)
}

func TestStripeOrderCreatesCheckoutSessionMetadata(t *testing.T) {
	h := newHarness(t)
	h.stripe.session = stripeprovider.CheckoutSession{ID: "cs_test_123", URL: "https://checkout.stripe.test/session/cs_test_123"}
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "stripe",
		Name:             "stripe-primary",
		Config:           map[string]any{"secret_key": "stripe_test_api_key", "webhook_secret": "stripe_test_webhook_secret", "success_url": "https://app.example/success", "cancel_url": "https://app.example/cancel"},
		SupportedMethods: []string{"card"},
		Limits:           map[string]any{"currency": "USD"},
	})
	if err != nil {
		t.Fatalf("create stripe provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "card",
		Amount:      "12.34",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create stripe order: %v", err)
	}
	if order.ProviderInstanceID != provider.ID {
		t.Fatalf("expected stripe provider instance %d, got %+v", provider.ID, order)
	}
	if order.Metadata["stripe_checkout_session_id"] != "cs_test_123" || order.Metadata["stripe_checkout_url"] != h.stripe.session.URL {
		t.Fatalf("expected checkout metadata, got %+v", order.Metadata)
	}
	if h.stripe.last.APIKey != "stripe_test_api_key" || h.stripe.last.Amount != 1234 || h.stripe.last.Currency != "usd" || h.stripe.last.OrderNo != order.OrderNo {
		t.Fatalf("unexpected stripe checkout request: %+v", h.stripe.last)
	}
	if h.stripe.last.Metadata["order_no"] != order.OrderNo || h.stripe.last.Metadata["user_id"] != "1" {
		t.Fatalf("expected order metadata in stripe checkout request, got %+v", h.stripe.last.Metadata)
	}
}

func TestStripeWebhookFulfillsCheckoutSession(t *testing.T) {
	h := newHarness(t)
	h.stripe.session = stripeprovider.CheckoutSession{ID: "cs_test_order", URL: "https://checkout.stripe.test/session/cs_test_order"}
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "stripe",
		Name:             "stripe-primary",
		Config:           map[string]any{"secret_key": "stripe_test_api_key", "webhook_secret": "stripe_test_webhook_secret", "success_url": "https://app.example/success", "cancel_url": "https://app.example/cancel"},
		SupportedMethods: []string{"card"},
		Limits:           map[string]any{"currency": "USD"},
	}); err != nil {
		t.Fatalf("create stripe provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "card",
		Amount:      "12.34",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create stripe order: %v", err)
	}
	payload := []byte(`{"id":"evt_test_paid","type":"checkout.session.completed","data":{"object":{"id":"cs_test_order","object":"checkout.session","client_reference_id":"` + order.OrderNo + `","amount_total":1234,"currency":"usd","metadata":{"order_no":"` + order.OrderNo + `"}}}}`)
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload:   payload,
		Secret:    "stripe_test_webhook_secret",
		Timestamp: time.Now(),
	})
	result, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "stripe",
		Headers:  map[string]string{"Stripe-Signature": signed.Header},
		Payload:  map[string]any{"raw_body": string(payload)},
	})
	if err != nil {
		t.Fatalf("handle stripe webhook: %v", err)
	}
	if !result.Handled || result.Order.Status != contract.OrderStatusFulfilled || stringValue(result.Order.ProviderTransactionID) != "cs_test_order" {
		t.Fatalf("expected fulfilled stripe order, got %+v", result)
	}
	ledger, err := h.billing.List(t.Context())
	if err != nil {
		t.Fatalf("list billing ledger: %v", err)
	}
	if len(ledger) != 1 || ledger[0].Type != billingcontract.LedgerTypePaymentCredit || ledger[0].ReferenceID != order.OrderNo {
		t.Fatalf("expected payment credit ledger, got %+v", ledger)
	}
}

func TestEasyPayOrderCreatesSignedCheckoutURL(t *testing.T) {
	h := newHarness(t)
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "easypay-primary",
		Config:           easypayTestConfig("easypay-signing-secret"),
		SupportedMethods: []string{"wechat"},
		Limits:           map[string]any{"currency": "USD"},
	})
	if err != nil {
		t.Fatalf("create easypay provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "wechat",
		Amount:      "12.34000000",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create easypay order: %v", err)
	}
	if order.ProviderInstanceID != provider.ID {
		t.Fatalf("expected easypay provider instance %d, got %+v", provider.ID, order)
	}
	checkoutURL := stringValueFromMap(order.Metadata, "checkout_url")
	if checkoutURL == "" || !strings.Contains(checkoutURL, "out_trade_no="+order.OrderNo) || !strings.Contains(checkoutURL, "type=wxpay") {
		t.Fatalf("expected signed easypay checkout url, got %+v", order.Metadata)
	}
	if order.Metadata["easypay_pay_url"] != checkoutURL || order.Metadata["easypay_sign"] == "" {
		t.Fatalf("expected easypay metadata, got %+v", order.Metadata)
	}
}

func TestStripeCheckoutFailureReturnsProviderUnavailable(t *testing.T) {
	h := newHarness(t)
	h.stripe.err = errors.New("stripe api unavailable")
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "stripe",
		Name:             "stripe-primary",
		Config:           map[string]any{"secret_key": "stripe_test_api_key", "webhook_secret": "stripe_test_webhook_secret", "success_url": "https://app.example/success", "cancel_url": "https://app.example/cancel"},
		SupportedMethods: []string{"card"},
		Limits:           map[string]any{"currency": "USD"},
	}); err != nil {
		t.Fatalf("create stripe provider: %v", err)
	}
	_, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "card",
		Amount:      "12.34",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected checkout failure to return provider unavailable, got %v", err)
	}
}

func TestPaymentOrderStatusMachineRejectsIllegalTransitions(t *testing.T) {
	h := newHarness(t)
	_, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "manual",
		Name:             "manual-credit",
		Config:           map[string]any{"webhook_secret": "manual-secret"},
		SupportedMethods: []string{"manual"},
	})
	if err != nil {
		t.Fatalf("create provider instance: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "manual",
		Amount:      "5.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	canceled, err := h.payments.CancelOrder(t.Context(), 7, order.ID)
	if err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	if canceled.Status != contract.OrderStatusCanceled || canceled.ClosedAt == nil {
		t.Fatalf("expected canceled order, got %+v", canceled)
	}
	payload := map[string]any{
		"order_no":        order.OrderNo,
		"amount":          "5.00000000",
		"currency":        "USD",
		"status":          "paid",
		"transaction_id":  "txn_after_cancel",
		"idempotency_key": "evt_after_cancel",
	}
	_, err = h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "manual",
		Headers:  map[string]string{"X-SRapi-Payment-Signature": signWebhookPayload("manual-secret", payload)},
		Payload:  payload,
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected canceled order to reject paid transition, got %v", err)
	}
}

func TestExpirePendingOrdersMarksExpiredOrdersAndWritesAudit(t *testing.T) {
	h := newHarness(t)
	_, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "manual",
		Name:             "manual-credit",
		Config:           map[string]any{"webhook_secret": "manual-secret"},
		SupportedMethods: []string{"manual"},
	})
	if err != nil {
		t.Fatalf("create provider instance: %v", err)
	}
	expiredOrder, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "manual",
		Amount:      "5.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create expired candidate order: %v", err)
	}
	futureOrder, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "manual",
		Amount:      "8.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create future order: %v", err)
	}
	now := time.Date(2026, 5, 22, 12, 5, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	expiredOrder.ExpiresAt = &past
	if _, err := h.store.UpdateOrder(t.Context(), expiredOrder); err != nil {
		t.Fatalf("backdate order expiry: %v", err)
	}

	result, err := h.payments.ExpirePendingOrders(t.Context(), now)
	if err != nil {
		t.Fatalf("expire pending orders: %v", err)
	}
	if result.Selected != 1 || result.Expired != 1 {
		t.Fatalf("unexpected expiration result: %+v", result)
	}
	updatedExpired, err := h.payments.FindOrderByID(t.Context(), expiredOrder.ID)
	if err != nil {
		t.Fatalf("find expired order: %v", err)
	}
	if updatedExpired.Status != contract.OrderStatusExpired || updatedExpired.ClosedAt == nil || !updatedExpired.ClosedAt.Equal(now) {
		t.Fatalf("expected expired order closed at worker time, got %+v", updatedExpired)
	}
	updatedFuture, err := h.payments.FindOrderByID(t.Context(), futureOrder.ID)
	if err != nil {
		t.Fatalf("find future order: %v", err)
	}
	if updatedFuture.Status != contract.OrderStatusPending {
		t.Fatalf("future order should remain pending, got %+v", updatedFuture)
	}
	audits, err := h.store.ListAuditLogsByOrder(t.Context(), expiredOrder.ID)
	if err != nil {
		t.Fatalf("list payment audit logs: %v", err)
	}
	if len(audits) != 1 || audits[0].EventType != "order.expired" || !audits[0].SignatureValid {
		t.Fatalf("expected signed expiration audit log, got %+v", audits)
	}

	second, err := h.payments.ExpirePendingOrders(t.Context(), now.Add(time.Minute))
	if err != nil {
		t.Fatalf("expire pending orders again: %v", err)
	}
	if second.Expired != 0 {
		t.Fatalf("expiration should be idempotent on second run, got %+v", second)
	}
	audits, err = h.store.ListAuditLogsByOrder(t.Context(), expiredOrder.ID)
	if err != nil {
		t.Fatalf("list payment audit logs again: %v", err)
	}
	if len(audits) != 1 {
		t.Fatalf("expected one expiration audit log after second run, got %+v", audits)
	}
}

type harness struct {
	store         *paymentmemory.Store
	payments      *Service
	billing       *billingservice.Service
	subscriptions *subscriptionservice.Service
	audit         *auditservice.Service
	events        *eventsservice.Service
	stripe        *fakeStripeCheckout
}

func newHarness(t *testing.T) harness {
	t.Helper()
	store := paymentmemory.New()
	billingSvc, err := billingservice.New(billingmemory.New(), fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}
	subSvc, err := subscriptionservice.New(subscriptionmemory.New(), fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	auditSvc, err := auditservice.New(auditmemory.New(), fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new audit service: %v", err)
	}
	eventsSvc, err := eventsservice.New(eventsmemory.New(), fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	stripeFake := &fakeStripeCheckout{}
	paymentsSvc, err := New(store, testMasterKey, Dependencies{
		Billing:       billingSvc,
		Subscriptions: subSvc,
		Audit:         auditSvc,
		Events:        eventsSvc,
		Stripe:        stripeFake,
	}, fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new payment service: %v", err)
	}
	return harness{store: store, payments: paymentsSvc, billing: billingSvc, subscriptions: subSvc, audit: auditSvc, events: eventsSvc, stripe: stripeFake}
}

func assertCounts(t *testing.T, h harness, wantLedger int, wantSubs int, wantOutbox int, wantAudit int) {
	t.Helper()
	ledger, err := h.billing.List(t.Context())
	if err != nil {
		t.Fatalf("list billing ledger: %v", err)
	}
	if len(ledger) != wantLedger {
		t.Fatalf("ledger count = %d, want %d: %+v", len(ledger), wantLedger, ledger)
	}
	subs, err := h.subscriptions.ListUserSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("list subscriptions: %v", err)
	}
	if len(subs) != wantSubs {
		t.Fatalf("subscription count = %d, want %d: %+v", len(subs), wantSubs, subs)
	}
	outbox, err := h.events.ListOutbox(t.Context())
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(outbox) != wantOutbox {
		t.Fatalf("outbox count = %d, want %d: %+v", len(outbox), wantOutbox, outbox)
	}
	orders, err := h.payments.ListOrders(t.Context())
	if err != nil || len(orders) == 0 {
		t.Fatalf("list orders: orders=%+v err=%v", orders, err)
	}
	audits, err := h.store.ListAuditLogsByOrder(t.Context(), orders[0].ID)
	if err != nil {
		t.Fatalf("list payment audits: %v", err)
	}
	if len(audits) != wantAudit {
		t.Fatalf("audit count = %d, want %d: %+v", len(audits), wantAudit, audits)
	}
}

func mustJSON(t *testing.T, value map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return string(raw)
}

func easypayTestConfig(secret string) map[string]any {
	return map[string]any{
		"gateway_url":    "https://pay.example/submit",
		"merchant_id":    "merchant-1",
		"webhook_secret": secret,
		"notify_url":     "https://api.example/api/v1/webhooks/payments/easypay",
		"return_url":     "https://app.example/payments/return",
	}
}

func stringValueFromMap(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return typed
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeStripeCheckout struct {
	last    stripeprovider.CheckoutSessionRequest
	session stripeprovider.CheckoutSession
	err     error
}

func (f *fakeStripeCheckout) CreateCheckoutSession(req stripeprovider.CheckoutSessionRequest) (stripeprovider.CheckoutSession, error) {
	f.last = req
	if f.err != nil {
		return stripeprovider.CheckoutSession{}, f.err
	}
	return f.session, nil
}
