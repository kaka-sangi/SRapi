package orderexpirer

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
)

const testMasterKey = "payment_master_key_32_bytes_minimum_value"

func TestRunOnceExpiresPendingPaymentOrders(t *testing.T) {
	store := paymentmemory.New()
	now := time.Date(2026, 5, 22, 12, 5, 0, 0, time.UTC)
	payments, err := paymentservice.New(store, testMasterKey, paymentservice.Dependencies{}, fixedClock{now: now.Add(-time.Hour)})
	if err != nil {
		t.Fatalf("new payment service: %v", err)
	}
	_, err = payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "manual",
		Name:             "manual-credit",
		Config:           map[string]any{"webhook_secret": "manual-secret"},
		SupportedMethods: []string{"manual"},
	})
	if err != nil {
		t.Fatalf("create provider instance: %v", err)
	}
	order, err := payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "manual",
		Amount:      "5.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	past := now.Add(-time.Minute)
	order.ExpiresAt = &past
	if _, err := store.UpdateOrder(t.Context(), order); err != nil {
		t.Fatalf("backdate order expiry: %v", err)
	}

	worker, err := New(store, testMasterKey, paymentservice.Dependencies{}, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		Clock: fixedClock{now: now},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Expired != 1 {
		t.Fatalf("expected one expired order, got %+v", result)
	}
	updated, err := payments.FindOrderByID(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("find order: %v", err)
	}
	if updated.Status != contract.OrderStatusExpired || updated.ClosedAt == nil || !updated.ClosedAt.Equal(now) {
		t.Fatalf("expected expired order closed at worker time, got %+v", updated)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
