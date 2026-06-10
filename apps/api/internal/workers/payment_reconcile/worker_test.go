package paymentreconcile

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"
	checkoutprovider "github.com/srapi/srapi/apps/api/internal/modules/payments/providers/checkout"
	paymentservice "github.com/srapi/srapi/apps/api/internal/modules/payments/service"
	paymentmemory "github.com/srapi/srapi/apps/api/internal/modules/payments/store/memory"
)

const testMasterKey = "payment_master_key_32_bytes_minimum_value"

func TestRunOnceReconcilesPaidPendingPaymentOrder(t *testing.T) {
	store := paymentmemory.New()
	provider := &fakeCheckoutProvider{
		query: checkoutprovider.QueryResult{
			Status:                checkoutprovider.QueryStatusPaid,
			ProviderTransactionID: "txn_reconciled",
			Amount:                "12.00000000",
			Currency:              "USD",
		},
	}
	payments, err := paymentservice.New(store, testMasterKey, paymentservice.Dependencies{
		Checkout: checkoutprovider.Registry{"manual": provider},
		Balance:  stubBalance{},
	}, fixedClock{now: time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("new payment service: %v", err)
	}
	if _, err := payments.CreateProviderInstance(t.Context(), contract.CreateProviderInstanceRequest{
		Provider:         "manual",
		Name:             "manual-credit",
		Config:           map[string]any{"webhook_secret": "manual-secret"},
		SupportedMethods: []string{"manual"},
	}); err != nil {
		t.Fatalf("create provider instance: %v", err)
	}
	order, err := payments.CreateOrder(t.Context(), contract.CreateOrderRequest{
		UserID:      7,
		Method:      "manual",
		Amount:      "12.00",
		Currency:    "USD",
		ProductType: contract.ProductTypeBalanceCredit,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}

	worker, err := New(store, testMasterKey, paymentservice.Dependencies{
		Checkout: checkoutprovider.Registry{"manual": provider},
		Balance:  stubBalance{},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{Clock: fixedClock{now: time.Date(2026, 6, 9, 12, 1, 0, 0, time.UTC)}})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Selected != 1 || result.Paid != 1 {
		t.Fatalf("unexpected reconcile result: %+v", result)
	}
	updated, err := payments.FindOrderByID(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("find order: %v", err)
	}
	if updated.Status != contract.OrderStatusFulfilled || updated.ProviderTransactionID == nil || *updated.ProviderTransactionID != "txn_reconciled" {
		t.Fatalf("expected fulfilled reconciled order, got %+v", updated)
	}
	audits, err := store.ListAuditLogsByOrder(t.Context(), order.ID)
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	if len(audits) < 2 || audits[0].EventType != "reconcile.query" {
		t.Fatalf("expected reconcile audit timeline, got %+v", audits)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type fakeCheckoutProvider struct {
	query checkoutprovider.QueryResult
}

func (f *fakeCheckoutProvider) CreateSession(checkoutprovider.Request) (checkoutprovider.Session, error) {
	return checkoutprovider.Session{}, nil
}

func (f *fakeCheckoutProvider) Refund(checkoutprovider.RefundRequest) (checkoutprovider.RefundResult, error) {
	return checkoutprovider.RefundResult{Status: checkoutprovider.RefundStatusSucceeded}, nil
}

func (f *fakeCheckoutProvider) QueryOrder(checkoutprovider.QueryRequest) (checkoutprovider.QueryResult, error) {
	return f.query, nil
}

type stubBalance struct{}

func (stubBalance) CreditBalance(context.Context, int, string, string) error { return nil }

func (stubBalance) DebitBalance(context.Context, int, string, string) error { return nil }
