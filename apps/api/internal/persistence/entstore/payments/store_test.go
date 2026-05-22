package payments

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/payments/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStorePersistsProvidersOrdersAndIdempotentAuditLogs(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	provider, err := store.CreateProviderInstance(ctx, contract.CreateStoredProviderInstance{
		Provider:         "easypay",
		Name:             "primary",
		Status:           contract.ProviderStatusActive,
		ConfigCiphertext: "v1:nonce:ciphertext",
		ConfigVersion:    1,
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD"},
		SortOrder:        10,
		Metadata:         map[string]any{"display_name": "AliPay"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	if provider.ConfigCiphertext != "v1:nonce:ciphertext" || provider.SupportedMethods[0] != "alipay" {
		t.Fatalf("unexpected provider: %+v", provider)
	}

	expiresAt := time.Date(2026, 5, 22, 12, 30, 0, 0, time.UTC)
	order, err := store.CreateOrder(ctx, contract.CreateStoredOrder{
		UserID:             7,
		OrderNo:            "pay_test_1",
		ProviderInstanceID: provider.ID,
		Amount:             "19.99000000",
		Currency:           "USD",
		Status:             contract.OrderStatusPending,
		ProductType:        contract.ProductTypeSubscriptionPlan,
		ProductID:          "3",
		ProviderSnapshot:   map[string]any{"provider": "easypay"},
		ExpiresAt:          &expiresAt,
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	transactionID := "txn_1"
	paidAt := time.Date(2026, 5, 22, 12, 1, 0, 0, time.UTC)
	order.Status = contract.OrderStatusFulfilled
	order.ProviderTransactionID = &transactionID
	order.PaidAt = &paidAt
	updated, err := store.UpdateOrder(ctx, order)
	if err != nil {
		t.Fatalf("update order: %v", err)
	}
	if updated.Status != contract.OrderStatusFulfilled || updated.ProviderTransactionID == nil || *updated.ProviderTransactionID != transactionID {
		t.Fatalf("unexpected updated order: %+v", updated)
	}
	byNo, err := store.FindOrderByOrderNo(ctx, order.OrderNo)
	if err != nil {
		t.Fatalf("find order by no: %v", err)
	}
	if byNo.ID != order.ID || byNo.ProviderSnapshot["provider"] != "easypay" {
		t.Fatalf("unexpected order lookup: %+v", byNo)
	}

	audit, created, err := store.CreateAuditLog(ctx, contract.PaymentAuditLog{
		OrderID:            order.ID,
		ProviderInstanceID: provider.ID,
		EventType:          "webhook.paid",
		IdempotencyKey:     "evt_1",
		Payload:            map[string]any{"order_no": order.OrderNo},
		SignatureValid:     true,
	})
	if err != nil || !created {
		t.Fatalf("create audit log: audit=%+v created=%v err=%v", audit, created, err)
	}
	duplicate, created, err := store.CreateAuditLog(ctx, contract.PaymentAuditLog{
		OrderID:            order.ID,
		ProviderInstanceID: provider.ID,
		EventType:          "webhook.paid",
		IdempotencyKey:     "evt_1",
		Payload:            map[string]any{"order_no": order.OrderNo},
		SignatureValid:     true,
	})
	if err != nil || created || duplicate.ID != audit.ID {
		t.Fatalf("expected idempotent duplicate audit log: audit=%+v duplicate=%+v created=%v err=%v", audit, duplicate, created, err)
	}
	logs, err := store.ListAuditLogsByOrder(ctx, order.ID)
	if err != nil {
		t.Fatalf("list audit logs: %v", err)
	}
	if len(logs) != 1 || logs[0].Payload["order_no"] != order.OrderNo {
		t.Fatalf("expected one persisted audit log, got %+v", logs)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "payments.db") + "?_fk=1"
}
