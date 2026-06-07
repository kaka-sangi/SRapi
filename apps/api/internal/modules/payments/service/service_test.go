package service

import (
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	alipaysdk "github.com/smartwalle/alipay/v3"

	auditservice "github.com/srapi/srapi/apps/api/internal/modules/audit/service"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
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

// Regression for B2: a paid balance_credit top-up must credit the user's
// spendable balance, and refunding it must claw that balance back.
func TestPaidBalanceCreditCreditsThenRefundDebits(t *testing.T) {
	h := newHarness(t)
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           easypayTestConfig("provider-signing-secret"),
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD"},
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "alipay",
		Amount:      "25.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	payload := map[string]any{
		"order_no":        order.OrderNo,
		"amount":          "25.00000000",
		"currency":        "USD",
		"status":          "paid",
		"transaction_id":  "txn_balance",
		"idempotency_key": "evt_balance_paid",
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
		t.Fatalf("expected fulfilled balance_credit order, got %+v", result)
	}
	if got := h.balance.net(7); got != "25.00000000" {
		t.Fatalf("balance after top-up = %s, want 25.00000000", got)
	}

	if _, err := h.payments.RequestRefund(t.Context(), contract.RefundRequest{OrderID: order.ID, ActorUserID: 1, Reason: "test refund"}); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got := h.balance.net(7); got != "0.00000000" {
		t.Fatalf("balance after refund = %s, want 0.00000000 (credit must be clawed back)", got)
	}
}

// A refund is one-shot: because the order carries no cumulative-refunded total,
// a second refund would claw back more balance than was ever paid. Verify the
// second attempt is rejected and the balance is not debited again.
func TestRefundIsOneShotPreventsDoubleDebit(t *testing.T) {
	h := newHarness(t)
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           easypayTestConfig("provider-signing-secret"),
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD"},
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "alipay",
		Amount:      "25.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	payload := map[string]any{
		"order_no":        order.OrderNo,
		"amount":          "25.00000000",
		"currency":        "USD",
		"status":          "paid",
		"transaction_id":  "txn_balance",
		"idempotency_key": "evt_balance_paid",
	}
	if _, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "easypay",
		Headers:  map[string]string{"X-SRapi-Payment-Signature": signWebhookPayload("provider-signing-secret", payload)},
		Payload:  payload,
	}); err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	// First refund: a partial 10.00 of the 25.00 order.
	if _, err := h.payments.RequestRefund(t.Context(), contract.RefundRequest{OrderID: order.ID, ActorUserID: 1, Amount: "10.00", Reason: "partial"}); err != nil {
		t.Fatalf("first (partial) refund: %v", err)
	}
	if got := h.balance.net(7); got != "15.00000000" {
		t.Fatalf("balance after partial refund = %s, want 15.00000000", got)
	}

	// Second refund must be rejected, leaving the balance untouched.
	if _, err := h.payments.RequestRefund(t.Context(), contract.RefundRequest{OrderID: order.ID, ActorUserID: 1, Reason: "double"}); err == nil {
		t.Fatal("expected the second refund to be rejected, got nil error")
	}
	if got := h.balance.net(7); got != "15.00000000" {
		t.Fatalf("balance after rejected second refund = %s, want 15.00000000 (no double debit)", got)
	}
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

func TestWechatWebhookFulfillsSignedNotification(t *testing.T) {
	h := newHarness(t)
	keys := newWechatTestKeys(t)
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "wechat",
		Name:     "wechat-primary",
		Config: map[string]any{
			"app_id":                  "wx_app_123",
			"mch_id":                  "mch_123",
			"api_v3_key":              keys.apiV3Key,
			"serial_no":               "merchant_serial_123",
			"private_key":             keys.merchantPrivateKey,
			"notify_url":              "https://api.example/api/v1/webhooks/payments/wechat",
			"wechatpay_public_key":    keys.platformPublicKey,
			"wechatpay_public_key_id": keys.platformPublicKeyID,
		},
		SupportedMethods: []string{"wechat"},
		Limits:           map[string]any{"currency": "CNY"},
	})
	if err != nil {
		t.Fatalf("create wechat provider: %v", err)
	}
	order, err := h.store.CreateOrder(t.Context(), contract.CreateStoredOrder{
		UserID:             1,
		OrderNo:            "pay_wechat_123",
		ProviderInstanceID: provider.ID,
		Amount:             "12.34000000",
		Currency:           "CNY",
		Status:             contract.OrderStatusPending,
		ProductType:        contract.ProductTypeBalanceCredit,
		ProviderSnapshot: map[string]any{
			"provider": "wechat",
			"method":   "wechat",
		},
	})
	if err != nil {
		t.Fatalf("create stored wechat order: %v", err)
	}
	payload, headers := signedWechatNotification(t, keys, map[string]any{
		"appid":          "wx_app_123",
		"mchid":          "mch_123",
		"out_trade_no":   order.OrderNo,
		"transaction_id": "4200000000202605221234567890",
		"trade_state":    "SUCCESS",
		"trade_type":     "NATIVE",
		"amount": map[string]any{
			"total":    1234,
			"currency": "CNY",
		},
	})
	result, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "wechat",
		Headers:  headers,
		Payload:  map[string]any{"raw_body": payload},
	})
	if err != nil {
		t.Fatalf("handle wechat webhook: %v", err)
	}
	if !result.Handled || result.Order.Status != contract.OrderStatusFulfilled || stringValue(result.Order.ProviderTransactionID) != "4200000000202605221234567890" {
		t.Fatalf("expected fulfilled wechat order, got %+v", result)
	}
	ledger, err := h.billing.List(t.Context())
	if err != nil {
		t.Fatalf("list billing ledger: %v", err)
	}
	if len(ledger) != 1 || ledger[0].Type != billingcontract.LedgerTypePaymentCredit || ledger[0].ReferenceID != order.OrderNo {
		t.Fatalf("expected payment credit ledger, got %+v", ledger)
	}
	duplicate, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "wechat",
		Headers:  headers,
		Payload:  map[string]any{"raw_body": payload},
	})
	if err != nil {
		t.Fatalf("handle duplicate wechat webhook: %v", err)
	}
	if duplicate.Handled {
		t.Fatalf("duplicate wechat webhook should be idempotent, got %+v", duplicate)
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

func TestCreateOrderAppliesPromoCodeBeforeCheckout(t *testing.T) {
	store := &promoPreviewStore{
		Store: paymentmemory.New(),
		preview: contract.PromoCodeApplication{
			UserID:         7,
			PromoCodeID:    44,
			OriginalAmount: "20.00000000",
			DiscountAmount: "5.00000000",
			FinalAmount:    "15.00000000",
			Currency:       "USD",
			DiscountType:   "amount",
		},
	}
	paymentsSvc, err := New(store, testMasterKey, Dependencies{}, fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new payment service: %v", err)
	}
	if _, err := paymentsSvc.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "promo-provider",
		Config:           easypayTestConfig("promo-secret"),
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD", "min_amount": "1.00", "max_amount": "100.00"},
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	order, err := paymentsSvc.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "alipay",
		Amount:      "20",
		Currency:    "usd",
		ProductType: contract.ProductTypeBalanceCredit,
		PromoCode:   " save5 ",
	})
	if err != nil {
		t.Fatalf("create order with promo: %v", err)
	}
	if store.previewInput.Code != "SAVE5" || store.previewInput.Amount != "20.00000000" || store.previewInput.Currency != "USD" {
		t.Fatalf("unexpected promo preview input: %+v", store.previewInput)
	}
	if order.OriginalAmount != "20.00000000" || order.DiscountAmount != "5.00000000" || order.Amount != "15.00000000" {
		t.Fatalf("unexpected discounted order amounts: %+v", order)
	}
	if order.PromoCodeID == nil || *order.PromoCodeID != 44 {
		t.Fatalf("expected promo code id on order, got %+v", order.PromoCodeID)
	}
	if store.createInput.PromoCode != "SAVE5" || store.createInput.PromoCodeID == nil || *store.createInput.PromoCodeID != 44 {
		t.Fatalf("expected stored promo details, got %+v", store.createInput)
	}
}

func TestWechatOrderCreatesPrepayCheckoutMetadata(t *testing.T) {
	h := newHarnessWithDeps(t, Dependencies{
		Checkout: checkoutprovider.Registry{
			"wechat": fakeCheckoutProvider{session: checkoutprovider.Session{
				ID:  "prepay_id=test",
				URL: "weixin://wxpay/bizpayurl?pr=test",
				Metadata: map[string]any{
					"wechat_pay_mode": "native",
					"wechat_code_url": "weixin://wxpay/bizpayurl?pr=test",
				},
			}},
		},
	})
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "wechat",
		Name:     "wechat-primary",
		Config: map[string]any{
			"app_id":      "wx_app_123",
			"mch_id":      "mch_123",
			"api_v3_key":  "0123456789abcdef0123456789abcdef",
			"serial_no":   "merchant_serial_123",
			"private_key": "merchant-private-key",
			"notify_url":  "https://api.example/api/v1/webhooks/payments/wechat",
		},
		SupportedMethods: []string{"wechat"},
		Limits:           map[string]any{"currency": "CNY"},
	})
	if err != nil {
		t.Fatalf("create wechat provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "wechat",
		Amount:      "12.34000000",
		Currency:    "CNY",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create wechat order: %v", err)
	}
	if order.ProviderInstanceID != provider.ID {
		t.Fatalf("expected wechat provider instance %d, got %+v", provider.ID, order)
	}
	if order.Metadata["checkout_session_id"] != "prepay_id=test" || order.Metadata["checkout_url"] != "weixin://wxpay/bizpayurl?pr=test" {
		t.Fatalf("expected wechat checkout identifiers, got %+v", order.Metadata)
	}
	if order.Metadata["wechat_pay_mode"] != "native" || order.Metadata["wechat_code_url"] != "weixin://wxpay/bizpayurl?pr=test" {
		t.Fatalf("expected wechat checkout metadata, got %+v", order.Metadata)
	}
}

func TestAlipayOrderCreatesSignedCheckoutURL(t *testing.T) {
	keys := newAlipayTestKeys(t)
	h := newHarness(t)
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "alipay",
		Name:     "alipay-primary",
		Config: map[string]any{
			"app_id":            "app_test_123",
			"private_key":       keys.merchantPrivateKey,
			"alipay_public_key": keys.alipayPublicKey,
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
			"gateway_url":       "https://openapi.alipay.test/gateway.do",
			"subject":           "SRapi balance top-up",
		},
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "CNY"},
	})
	if err != nil {
		t.Fatalf("create alipay provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "alipay",
		Amount:      "12.34",
		Currency:    "CNY",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create alipay order: %v", err)
	}
	if order.ProviderInstanceID != provider.ID {
		t.Fatalf("expected alipay provider instance %d, got %+v", provider.ID, order)
	}
	checkoutURL := stringValueFromMap(order.Metadata, "checkout_url")
	if checkoutURL == "" || !strings.Contains(checkoutURL, "method=alipay.trade.page.pay") || !strings.Contains(checkoutURL, "sign=") {
		t.Fatalf("expected signed alipay checkout url, got %+v", order.Metadata)
	}
	if order.Metadata["alipay_pay_url"] != checkoutURL || order.Metadata["alipay_mode"] != "page" {
		t.Fatalf("expected alipay metadata, got %+v", order.Metadata)
	}
}

func TestAlipayWebhookFulfillsSignedNotification(t *testing.T) {
	keys := newAlipayTestKeys(t)
	h := newHarness(t)
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "alipay",
		Name:     "alipay-primary",
		Config: map[string]any{
			"app_id":            "app_test_123",
			"private_key":       keys.merchantPrivateKey,
			"alipay_public_key": keys.alipayPublicKey,
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
		},
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "CNY"},
	}); err != nil {
		t.Fatalf("create alipay provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "alipay",
		Amount:      "12.34",
		Currency:    "CNY",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create alipay order: %v", err)
	}
	payload := signedAlipayNotification(t, keys, map[string]string{
		"notify_id":    "notify_paid_1",
		"notify_type":  alipaysdk.NotifyTypeTradeStatusSync,
		"out_trade_no": order.OrderNo,
		"trade_no":     "2026052522001400000001",
		"trade_status": string(alipaysdk.TradeStatusSuccess),
		"total_amount": "12.34",
		"app_id":       "app_test_123",
		"charset":      "utf-8",
		"version":      "1.0",
	})
	result, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{
		Provider: "alipay",
		Payload:  payload,
	})
	if err != nil {
		t.Fatalf("handle alipay webhook: %v", err)
	}
	if !result.Handled || result.Order.Status != contract.OrderStatusFulfilled || stringValue(result.Order.ProviderTransactionID) != "2026052522001400000001" {
		t.Fatalf("expected fulfilled alipay order, got %+v", result)
	}
	audits, err := h.store.ListAuditLogsByOrder(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("list payment audit logs: %v", err)
	}
	if len(audits) != 1 || !audits[0].SignatureValid || strings.Contains(mustJSON(t, audits[0].Payload), keys.merchantPrivateKey) {
		t.Fatalf("expected signed secret-free alipay audit log, got %+v", audits)
	}
	duplicate, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{Provider: "alipay", Payload: payload})
	if err != nil {
		t.Fatalf("handle duplicate alipay webhook: %v", err)
	}
	if duplicate.Handled {
		t.Fatalf("duplicate alipay webhook should be idempotent, got %+v", duplicate)
	}
}

func TestAlipayWebhookUsesOrderProviderInstance(t *testing.T) {
	keys := newAlipayTestKeys(t)
	h := newHarness(t)
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "alipay",
		Name:     "alipay-shadow",
		Config: map[string]any{
			"app_id":            "app_test_123",
			"private_key":       keys.merchantPrivateKey,
			"alipay_public_key": keys.alipayPublicKey,
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
		},
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "JPY"},
	}); err != nil {
		t.Fatalf("create shadow alipay provider: %v", err)
	}
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "alipay",
		Name:     "alipay-primary",
		Config: map[string]any{
			"app_id":            "app_test_123",
			"private_key":       keys.merchantPrivateKey,
			"alipay_public_key": keys.alipayPublicKey,
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
		},
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "CNY"},
	})
	if err != nil {
		t.Fatalf("create primary alipay provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "alipay",
		Amount:      "7.00",
		Currency:    "CNY",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create alipay order: %v", err)
	}
	if order.ProviderInstanceID != provider.ID {
		t.Fatalf("expected order on primary provider %d, got %+v", provider.ID, order)
	}
	payload := signedAlipayNotification(t, keys, map[string]string{
		"notify_id":    "notify_paid_primary",
		"notify_type":  alipaysdk.NotifyTypeTradeStatusSync,
		"out_trade_no": order.OrderNo,
		"trade_no":     "2026052522001400000003",
		"trade_status": string(alipaysdk.TradeStatusSuccess),
		"total_amount": "7.00",
		"app_id":       "app_test_123",
		"charset":      "utf-8",
		"version":      "1.0",
	})
	result, err := h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{Provider: "alipay", Payload: payload})
	if err != nil {
		t.Fatalf("handle alipay webhook: %v", err)
	}
	if !result.Handled || result.Order.ProviderInstanceID != provider.ID || result.Order.Status != contract.OrderStatusFulfilled {
		t.Fatalf("expected webhook to use order provider instance, got %+v", result)
	}
}

func TestAlipayWebhookRejectsInvalidSignature(t *testing.T) {
	keys := newAlipayTestKeys(t)
	h := newHarness(t)
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider: "alipay",
		Name:     "alipay-primary",
		Config: map[string]any{
			"app_id":            "app_test_123",
			"private_key":       keys.merchantPrivateKey,
			"alipay_public_key": keys.alipayPublicKey,
			"notify_url":        "https://api.example/api/v1/webhooks/payments/alipay",
			"return_url":        "https://app.example/payments/return",
		},
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "CNY"},
	}); err != nil {
		t.Fatalf("create alipay provider: %v", err)
	}
	order, err := h.payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      1,
		Method:      "alipay",
		Amount:      "12.34",
		Currency:    "CNY",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create alipay order: %v", err)
	}
	payload := signedAlipayNotification(t, keys, map[string]string{
		"notify_id":    "notify_bad_1",
		"notify_type":  alipaysdk.NotifyTypeTradeStatusSync,
		"out_trade_no": order.OrderNo,
		"trade_no":     "2026052522001400000002",
		"trade_status": string(alipaysdk.TradeStatusSuccess),
		"total_amount": "12.34",
		"app_id":       "app_test_123",
		"charset":      "utf-8",
		"version":      "1.0",
	})
	payload["sign"] = "invalid-signature"
	_, err = h.payments.HandleWebhook(t.Context(), contract.WebhookRequest{Provider: "alipay", Payload: payload})
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("expected invalid alipay signature rejection, got %v", err)
	}
}

func TestUpdateProviderInstanceReencryptsConfigAfterRename(t *testing.T) {
	h := newHarness(t)
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "easypay-primary",
		Config:           easypayTestConfig("easypay-signing-secret"),
		SupportedMethods: []string{"alipay"},
	})
	if err != nil {
		t.Fatalf("create easypay provider: %v", err)
	}
	beforeCiphertext := provider.ConfigCiphertext
	name := "easypay-renamed"
	status := contract.ProviderStatusDisabled
	methods := []string{"wechat", "alipay", "wechat"}
	metadata := map[string]any{"display_name": "EasyPay Renamed"}
	updated, err := h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{
		Name:             &name,
		Status:           &status,
		SupportedMethods: &methods,
		Metadata:         &metadata,
	})
	if err != nil {
		t.Fatalf("update easypay provider: %v", err)
	}
	if updated.Name != name || updated.Status != status || strings.Join(updated.SupportedMethods, ",") != "alipay,wechat" {
		t.Fatalf("unexpected updated provider: %+v", updated)
	}
	if updated.ConfigCiphertext == "" || updated.ConfigCiphertext == beforeCiphertext {
		t.Fatalf("expected provider config to be re-encrypted on rename")
	}
	test, err := h.payments.TestProviderInstance(t.Context(), provider.ID)
	if err != nil {
		t.Fatalf("test updated provider: %v", err)
	}
	if test.OK || test.Message != "payment provider instance is not active" || test.Checks["config_decrypts"] != true {
		t.Fatalf("expected disabled provider to decrypt but fail active check, got %+v", test)
	}
	status = contract.ProviderStatusActive
	updated, err = h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{Status: &status})
	if err != nil {
		t.Fatalf("reactivate provider: %v", err)
	}
	test, err = h.payments.TestProviderInstance(t.Context(), updated.ID)
	if err != nil {
		t.Fatalf("test reactivated provider: %v", err)
	}
	if !test.OK || test.Status != "ok" {
		t.Fatalf("expected reactivated provider test to pass, got %+v", test)
	}
}

func TestPaymentProviderTestReportsMissingConfigRequirements(t *testing.T) {
	h := newHarness(t)
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "stripe",
		Name:             "stripe-incomplete",
		Config:           map[string]any{"secret_key": "stripe_test_api_key", "webhook_secret": "stripe_test_webhook_secret"},
		SupportedMethods: []string{"card"},
	})
	if err != nil {
		t.Fatalf("create stripe provider: %v", err)
	}
	result, err := h.payments.TestProviderInstance(t.Context(), provider.ID)
	if err != nil {
		t.Fatalf("test stripe provider: %v", err)
	}
	missing, ok := result.Checks["missing_requirements"].([]string)
	if result.OK || !ok || len(missing) != 2 || missing[0] != "config.success_url" || missing[1] != "config.cancel_url" {
		t.Fatalf("expected missing stripe checkout URLs, got %+v", result)
	}

	wechat, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "wechat",
		Name:             "wechat-incomplete",
		Config:           map[string]any{"app_id": "wx_app_123", "api_v3_key": "0123456789abcdef0123456789abcdef"},
		SupportedMethods: []string{"wechat"},
	})
	if err != nil {
		t.Fatalf("create incomplete wechat provider: %v", err)
	}
	result, err = h.payments.TestProviderInstance(t.Context(), wechat.ID)
	if err != nil {
		t.Fatalf("test wechat provider: %v", err)
	}
	missing, ok = result.Checks["missing_requirements"].([]string)
	if result.OK || !ok || len(missing) != 4 || missing[0] != "config.mch_id" || missing[1] != "config.serial_no" || missing[2] != "config.private_key" || missing[3] != "config.notify_url" {
		t.Fatalf("expected missing wechat APIv3 requirements, got %+v", result)
	}
}

func TestUpdateProviderInstanceRejectsDuplicateProviderName(t *testing.T) {
	h := newHarness(t)
	first, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           easypayTestConfig("secret-1"),
		SupportedMethods: []string{"alipay"},
	})
	if err != nil {
		t.Fatalf("create first provider: %v", err)
	}
	if _, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "secondary",
		Config:           easypayTestConfig("secret-2"),
		SupportedMethods: []string{"wechat"},
	}); err != nil {
		t.Fatalf("create second provider: %v", err)
	}
	duplicate := "secondary"
	_, err = h.payments.UpdateProviderInstance(t.Context(), first.ID, contract.UpdateProviderInstanceRequest{Name: &duplicate})
	if !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected duplicate provider name conflict, got %v", err)
	}
}

func TestUpdateProviderInstanceProtectsInProgressOrders(t *testing.T) {
	h := newHarness(t)
	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "manual",
		Name:             "manual-credit",
		Config:           map[string]any{"webhook_secret": "manual-secret"},
		SupportedMethods: []string{"manual", "bank"},
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
	renamed := "manual-renamed"
	metadata := map[string]any{"display_name": "Manual Renamed"}
	if _, err := h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{
		Name:     &renamed,
		Metadata: &metadata,
	}); err != nil {
		t.Fatalf("safe provider update with pending order: %v", err)
	}
	disabled := contract.ProviderStatusDisabled
	if _, err := h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{Status: &disabled}); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected disabling provider with pending order to conflict, got %v", err)
	}
	methods := []string{"bank"}
	if _, err := h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{SupportedMethods: &methods}); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected removing method with pending order to conflict, got %v", err)
	}
	config := map[string]any{"webhook_secret": "replacement-secret"}
	if _, err := h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{Config: &config}); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected replacing config with pending order to conflict, got %v", err)
	}
	if _, err := h.payments.CancelOrder(t.Context(), 7, order.ID); err != nil {
		t.Fatalf("cancel order: %v", err)
	}
	updated, err := h.payments.UpdateProviderInstance(t.Context(), provider.ID, contract.UpdateProviderInstanceRequest{Status: &disabled, SupportedMethods: &methods})
	if err != nil {
		t.Fatalf("expected disabling provider after order closes: %v", err)
	}
	if updated.Status != contract.ProviderStatusDisabled || strings.Join(updated.SupportedMethods, ",") != "bank" {
		t.Fatalf("unexpected provider after closed-order update: %+v", updated)
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

func TestDeleteProviderInstanceSoftDeletesAndGuardsInProgressOrders(t *testing.T) {
	h := newHarness(t)

	provider, err := h.payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "easypay",
		Name:             "primary",
		Config:           easypayTestConfig("provider-signing-secret"),
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD", "min_amount": "1.00", "max_amount": "100.00"},
	})
	if err != nil {
		t.Fatalf("create provider instance: %v", err)
	}

	order, err := h.store.CreateOrder(t.Context(), contract.CreateStoredOrder{
		UserID:             1,
		OrderNo:            "pay_pending_1",
		ProviderInstanceID: provider.ID,
		Amount:             "19.99000000",
		Currency:           "USD",
		Status:             contract.OrderStatusPending,
		ProductType:        contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create stored order: %v", err)
	}

	// An in-progress (pending) order must block deletion so an active payment
	// flow is never orphaned.
	if err := h.payments.DeleteProviderInstance(t.Context(), provider.ID); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected ErrConflict while a pending order references the provider, got %v", err)
	}

	// Settle the order to a terminal state; deletion then succeeds.
	order.Status = contract.OrderStatusFulfilled
	if _, err := h.store.UpdateOrder(t.Context(), order); err != nil {
		t.Fatalf("settle order: %v", err)
	}
	if err := h.payments.DeleteProviderInstance(t.Context(), provider.ID); err != nil {
		t.Fatalf("delete provider instance: %v", err)
	}

	// The soft-deleted provider is excluded from lookups and listings, but its
	// order/audit history (the row itself) is retained.
	if _, err := h.payments.FindProviderInstanceByID(t.Context(), provider.ID); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for deleted provider, got %v", err)
	}
	list, err := h.payments.ListProviderInstances(t.Context())
	if err != nil {
		t.Fatalf("list provider instances: %v", err)
	}
	for _, p := range list {
		if p.ID == provider.ID {
			t.Fatalf("deleted provider must not appear in listing: %+v", p)
		}
	}

	// Deleting again is a no-op not-found (idempotent), not a 500.
	if err := h.payments.DeleteProviderInstance(t.Context(), provider.ID); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected ErrNotFound deleting an already-deleted provider, got %v", err)
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
	balance       *stubBalance
}

type promoPreviewStore struct {
	*paymentmemory.Store
	preview      contract.PromoCodeApplication
	previewErr   error
	previewInput contract.PromoCodePreviewInput
	createInput  contract.CreateStoredOrder
}

func (s *promoPreviewStore) PreviewPromoCode(_ context.Context, input contract.PromoCodePreviewInput) (contract.PromoCodeApplication, error) {
	s.previewInput = input
	return s.preview, s.previewErr
}

func (s *promoPreviewStore) CreateOrder(ctx context.Context, input contract.CreateStoredOrder) (contract.PaymentOrder, error) {
	s.createInput = input
	return s.Store.CreateOrder(ctx, input)
}

func newHarness(t *testing.T) harness {
	t.Helper()
	return newHarnessWithDeps(t, Dependencies{})
}

// stubBalance is an in-memory BalanceAdjuster that tracks each user's net
// spendable balance so tests can assert top-up credits and refund debits.
type stubBalance struct {
	balances map[int]*big.Rat
}

func newStubBalance() *stubBalance { return &stubBalance{balances: map[int]*big.Rat{}} }

func (s *stubBalance) adjust(userID int, amount string, sign int) error {
	delta, ok := new(big.Rat).SetString(amount)
	if !ok {
		return errors.New("stub balance: invalid amount " + amount)
	}
	cur := s.balances[userID]
	if cur == nil {
		cur = new(big.Rat)
	}
	if sign < 0 {
		cur = new(big.Rat).Sub(cur, delta)
	} else {
		cur = new(big.Rat).Add(cur, delta)
	}
	s.balances[userID] = cur
	return nil
}

func (s *stubBalance) CreditBalance(_ context.Context, userID int, amount, _ string) error {
	return s.adjust(userID, amount, 1)
}

func (s *stubBalance) DebitBalance(_ context.Context, userID int, amount, _ string) error {
	return s.adjust(userID, amount, -1)
}

func (s *stubBalance) net(userID int) string {
	cur := s.balances[userID]
	if cur == nil {
		return "0.00000000"
	}
	return cur.FloatString(8)
}

func newHarnessWithDeps(t *testing.T, deps Dependencies) harness {
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
	balanceStub := newStubBalance()
	deps.Billing = billingSvc
	deps.Subscriptions = subSvc
	deps.Audit = auditSvc
	deps.Events = eventsSvc
	deps.Stripe = stripeFake
	if deps.Balance == nil {
		deps.Balance = balanceStub
	}
	paymentsSvc, err := New(store, testMasterKey, deps, fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new payment service: %v", err)
	}
	return harness{store: store, payments: paymentsSvc, billing: billingSvc, subscriptions: subSvc, audit: auditSvc, events: eventsSvc, stripe: stripeFake, balance: balanceStub}
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

type alipayTestKeys struct {
	merchantPrivateKey string
	alipayPrivateKey   string
	alipayPublicKey    string
}

type wechatTestKeys struct {
	apiV3Key            string
	merchantPrivateKey  string
	platformPrivateKey  *rsa.PrivateKey
	platformPublicKey   string
	platformPublicKeyID string
}

func newAlipayTestKeys(t *testing.T) alipayTestKeys {
	t.Helper()
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate merchant key: %v", err)
	}
	alipayKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate alipay key: %v", err)
	}
	return alipayTestKeys{
		merchantPrivateKey: encodePrivateKey(merchantKey),
		alipayPrivateKey:   encodePrivateKey(alipayKey),
		alipayPublicKey:    encodePublicKey(&alipayKey.PublicKey),
	}
}

func newWechatTestKeys(t *testing.T) wechatTestKeys {
	t.Helper()
	merchantKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wechat merchant key: %v", err)
	}
	platformKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wechat platform key: %v", err)
	}
	return wechatTestKeys{
		apiV3Key:            "0123456789abcdef0123456789abcdef",
		merchantPrivateKey:  encodePrivateKey(merchantKey),
		platformPrivateKey:  platformKey,
		platformPublicKey:   encodePublicKey(&platformKey.PublicKey),
		platformPublicKeyID: "PUB_KEY_ID_123",
	}
}

func signedAlipayNotification(t *testing.T, keys alipayTestKeys, fields map[string]string) map[string]any {
	t.Helper()
	client, err := alipaysdk.New("app_test_123", keys.alipayPrivateKey, false)
	if err != nil {
		t.Fatalf("new alipay signer: %v", err)
	}
	values := url.Values{}
	for key, value := range fields {
		values.Set(key, value)
	}
	signature, err := client.SignValues(values)
	if err != nil {
		t.Fatalf("sign alipay notification: %v", err)
	}
	values.Set("sign_type", "RSA2")
	values.Set("sign", base64.StdEncoding.EncodeToString(signature))
	payload := make(map[string]any, len(values))
	for key, values := range values {
		if len(values) > 0 {
			payload[key] = values[0]
		}
	}
	return payload
}

func signedWechatNotification(t *testing.T, keys wechatTestKeys, transaction map[string]any) (string, map[string]string) {
	t.Helper()
	plaintext, err := json.Marshal(transaction)
	if err != nil {
		t.Fatalf("marshal wechat transaction: %v", err)
	}
	block, err := aes.NewCipher([]byte(keys.apiV3Key))
	if err != nil {
		t.Fatalf("new wechat aes cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("new wechat gcm: %v", err)
	}
	associatedData := "transaction"
	resourceNonce := "notify123456"
	ciphertext := aead.Seal(nil, []byte(resourceNonce), plaintext, []byte(associatedData))
	body := map[string]any{
		"id":            "evt_wechat_paid",
		"create_time":   time.Now().UTC().Format(time.RFC3339),
		"event_type":    "TRANSACTION.SUCCESS",
		"resource_type": "encrypt-resource",
		"summary":       "transaction success",
		"resource": map[string]any{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      base64.StdEncoding.EncodeToString(ciphertext),
			"associated_data": associatedData,
			"nonce":           resourceNonce,
			"original_type":   "transaction",
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal wechat notification: %v", err)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signNonce := "signnonce123"
	message := timestamp + "\n" + signNonce + "\n" + string(raw) + "\n"
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, keys.platformPrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign wechat notification: %v", err)
	}
	return string(raw), map[string]string{
		"Wechatpay-Nonce":     signNonce,
		"Wechatpay-Serial":    keys.platformPublicKeyID,
		"Wechatpay-Signature": base64.StdEncoding.EncodeToString(signature),
		"Wechatpay-Timestamp": timestamp,
	}
}

func encodePrivateKey(key *rsa.PrivateKey) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

func encodePublicKey(key *rsa.PublicKey) string {
	raw, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		panic(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: raw}))
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

type fakeCheckoutProvider struct {
	session checkoutprovider.Session
	err     error
}

func (f fakeCheckoutProvider) CreateSession(checkoutprovider.Request) (checkoutprovider.Session, error) {
	if f.err != nil {
		return checkoutprovider.Session{}, f.err
	}
	return f.session, nil
}
