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
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
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
		Config:           map[string]any{"webhook_secret": "provider-signing-secret", "merchant_id": "merchant-1"},
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
	h := newHarness(t)
	_, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           map[string]any{"webhook_secret": "provider-signing-secret"},
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
	assertCounts(t, h, 0, 0, 0, 1)
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

type harness struct {
	store         *paymentmemory.Store
	payments      *Service
	billing       *billingservice.Service
	subscriptions *subscriptionservice.Service
	audit         *auditservice.Service
	events        *eventsservice.Service
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
	paymentsSvc, err := New(store, testMasterKey, Dependencies{
		Billing:       billingSvc,
		Subscriptions: subSvc,
		Audit:         auditSvc,
		Events:        eventsSvc,
	}, fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new payment service: %v", err)
	}
	return harness{store: store, payments: paymentsSvc, billing: billingSvc, subscriptions: subSvc, audit: auditSvc, events: eventsSvc}
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

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
