package payments

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entpaymentorder "github.com/srapi/srapi/apps/api/ent/paymentorder"
	entsetting "github.com/srapi/srapi/apps/api/ent/setting"
	entuserpromocodeapplication "github.com/srapi/srapi/apps/api/ent/userpromocodeapplication"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
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
	provider.Name = "renamed"
	provider.Status = contract.ProviderStatusDisabled
	provider.ConfigCiphertext = "v1:nonce:updated"
	provider.SupportedMethods = []string{"wechat", "alipay"}
	provider.Metadata = map[string]any{"display_name": "Renamed"}
	provider.UpdatedAt = time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	provider, err = store.UpdateProviderInstance(ctx, provider)
	if err != nil {
		t.Fatalf("update provider: %v", err)
	}
	if provider.Name != "renamed" || provider.Status != contract.ProviderStatusDisabled || provider.ConfigCiphertext != "v1:nonce:updated" || len(provider.SupportedMethods) != 2 {
		t.Fatalf("unexpected updated provider: %+v", provider)
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

func TestStoreCountsInProgressOrdersByProviderInstance(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	provider, err := store.CreateProviderInstance(ctx, contract.CreateStoredProviderInstance{
		Provider:         "manual",
		Name:             "primary",
		Status:           contract.ProviderStatusActive,
		ConfigCiphertext: "v1:nonce:ciphertext",
		ConfigVersion:    1,
		SupportedMethods: []string{"manual"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	statuses := []contract.OrderStatus{
		contract.OrderStatusPending,
		contract.OrderStatusPaid,
		contract.OrderStatusFulfilled,
		contract.OrderStatusCanceled,
		contract.OrderStatusExpired,
		contract.OrderStatusFailed,
	}
	for idx, status := range statuses {
		if _, err := store.CreateOrder(ctx, contract.CreateStoredOrder{
			UserID:             7,
			OrderNo:            "pay_count_" + strconv.Itoa(idx+1),
			ProviderInstanceID: provider.ID,
			Amount:             "5.00000000",
			Currency:           "USD",
			Status:             status,
			ProductType:        contract.ProductTypeBalanceCredit,
			ProviderSnapshot:   map[string]any{"provider": "manual"},
		}); err != nil {
			t.Fatalf("create %s order: %v", status, err)
		}
	}
	count, err := store.CountInProgressOrdersByProviderInstance(ctx, provider.ID)
	if err != nil {
		t.Fatalf("count in-progress orders: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected pending and paid orders to count as in-progress, got %d", count)
	}
}

func TestStoreCreatesDiscountedOrderAndPromoApplicationAtomically(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	provider, err := store.CreateProviderInstance(ctx, contract.CreateStoredProviderInstance{
		Provider:         "easypay",
		Name:             "primary",
		Status:           contract.ProviderStatusActive,
		ConfigCiphertext: "v1:nonce:ciphertext",
		ConfigVersion:    1,
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	seedPromoCodes(t, ctx, client, map[string]any{
		"next_id": 2,
		"items": []map[string]any{
			{
				"id":             1,
				"code":           "SAVE5",
				"status":         string(admincontrolcontract.PromoCodeStatusActive),
				"discount_type":  string(admincontrolcontract.PromoDiscountTypeAmount),
				"discount_value": "5.00",
				"currency":       "USD",
				"max_uses":       1,
				"used_count":     0,
				"created_at":     now.Format(time.RFC3339Nano),
				"updated_at":     now.Format(time.RFC3339Nano),
			},
		},
	})
	preview, err := store.PreviewPromoCode(ctx, contract.PromoCodePreviewInput{
		UserID:   7,
		Code:     "save5",
		Amount:   "20.00000000",
		Currency: "usd",
		Now:      now,
	})
	if err != nil {
		t.Fatalf("preview promo: %v", err)
	}
	if preview.DiscountAmount != "5.00000000" || preview.FinalAmount != "15.00000000" || preview.PromoCodeID != 1 {
		t.Fatalf("unexpected promo preview: %+v", preview)
	}
	order, err := store.CreateOrder(ctx, contract.CreateStoredOrder{
		UserID:             7,
		OrderNo:            "pay_promo_1",
		ProviderInstanceID: provider.ID,
		OriginalAmount:     "20.00000000",
		DiscountAmount:     "5.00000000",
		PromoCodeID:        intPtr(1),
		PromoCode:          "SAVE5",
		Amount:             "15.00000000",
		Currency:           "USD",
		Status:             contract.OrderStatusPending,
		ProductType:        contract.ProductTypeBalanceCredit,
		ProviderSnapshot:   map[string]any{"provider": "easypay"},
	})
	if err != nil {
		t.Fatalf("create discounted order: %v", err)
	}
	if order.OriginalAmount != "20.00000000" || order.DiscountAmount != "5.00000000" || order.Amount != "15.00000000" || order.PromoCodeID == nil || *order.PromoCodeID != 1 {
		t.Fatalf("unexpected persisted order: %+v", order)
	}
	app, err := client.UserPromoCodeApplication.Query().
		Where(entuserpromocodeapplication.PaymentOrderIDEQ(order.ID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("find promo application: %v", err)
	}
	if app.OrderNo != order.OrderNo || app.DiscountAmount != "5.00000000" || app.FinalAmount != "15.00000000" {
		t.Fatalf("unexpected promo application: %+v", app)
	}
	setting, err := client.Setting.Query().Where(entsetting.KeyEQ("admin_control.promo_codes")).Only(ctx)
	if err != nil {
		t.Fatalf("find promo setting: %v", err)
	}
	usedCount, status := promoUsageState(t, setting.ValueJSON)
	if usedCount != 1 || status != string(admincontrolcontract.PromoCodeStatusExpired) {
		t.Fatalf("expected promo usage to be exhausted, used=%d status=%s value=%+v", usedCount, status, setting.ValueJSON)
	}
	if _, err := store.CreateOrder(ctx, contract.CreateStoredOrder{
		UserID:             7,
		OrderNo:            "pay_promo_2",
		ProviderInstanceID: provider.ID,
		OriginalAmount:     "20.00000000",
		DiscountAmount:     "5.00000000",
		PromoCodeID:        intPtr(1),
		PromoCode:          "SAVE5",
		Amount:             "15.00000000",
		Currency:           "USD",
		Status:             contract.OrderStatusPending,
		ProductType:        contract.ProductTypeBalanceCredit,
		ProviderSnapshot:   map[string]any{"provider": "easypay"},
	}); err == nil {
		t.Fatalf("expected exhausted promo to reject second order")
	}
	count, err := client.PaymentOrder.Query().
		Where(entpaymentorder.OrderNoEQ("pay_promo_2")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count rejected order: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected rejected discounted order to rollback, found %d rows", count)
	}
}

func TestStorePromoLimitsAndRelease(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	provider, err := store.CreateProviderInstance(ctx, contract.CreateStoredProviderInstance{
		Provider:         "easypay",
		Name:             "primary",
		Status:           contract.ProviderStatusActive,
		ConfigCiphertext: "v1:nonce:ciphertext",
		ConfigVersion:    1,
		SupportedMethods: []string{"alipay"},
		Limits:           map[string]any{"currency": "USD"},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	seedPromoCodes(t, ctx, client, map[string]any{
		"next_id": 2,
		"items": []map[string]any{
			{
				"id":               1,
				"code":             "SAVE10",
				"status":           string(admincontrolcontract.PromoCodeStatusActive),
				"discount_type":    string(admincontrolcontract.PromoDiscountTypeAmount),
				"discount_value":   "10.00",
				"currency":         "USD",
				"max_uses":         3,
				"per_user_limit":   1,
				"min_order_amount": "50.00",
				"used_count":       0,
				"created_at":       now.Format(time.RFC3339Nano),
				"updated_at":       now.Format(time.RFC3339Nano),
			},
		},
	})
	if _, err := store.PreviewPromoCode(ctx, contract.PromoCodePreviewInput{
		UserID:   7,
		Code:     "SAVE10",
		Amount:   "40.00000000",
		Currency: "USD",
		Now:      now,
	}); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected min order conflict, got %v", err)
	}

	order, err := store.CreateOrder(ctx, contract.CreateStoredOrder{
		UserID:             7,
		OrderNo:            "pay_promo_limit_1",
		ProviderInstanceID: provider.ID,
		OriginalAmount:     "60.00000000",
		DiscountAmount:     "10.00000000",
		PromoCodeID:        intPtr(1),
		PromoCode:          "SAVE10",
		Amount:             "50.00000000",
		Currency:           "USD",
		Status:             contract.OrderStatusPending,
		ProductType:        contract.ProductTypeBalanceCredit,
		ProviderSnapshot:   map[string]any{"provider": "easypay"},
	})
	if err != nil {
		t.Fatalf("create discounted order: %v", err)
	}
	if _, err := store.PreviewPromoCode(ctx, contract.PromoCodePreviewInput{
		UserID:   7,
		Code:     "SAVE10",
		Amount:   "60.00000000",
		Currency: "USD",
		Now:      now,
	}); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("expected per-user limit conflict, got %v", err)
	}

	released, ok, err := store.ReleasePromoCode(ctx, contract.PromoCodeReleaseInput{PaymentOrderID: order.ID, ReleasedAt: now.Add(time.Minute), Reason: "order_canceled"})
	if err != nil || !ok {
		t.Fatalf("release promo: released=%+v ok=%v err=%v", released, ok, err)
	}
	if released.Metadata["released"] != true {
		t.Fatalf("expected released metadata, got %+v", released.Metadata)
	}
	setting, err := client.Setting.Query().Where(entsetting.KeyEQ("admin_control.promo_codes")).Only(ctx)
	if err != nil {
		t.Fatalf("find promo setting: %v", err)
	}
	usedCount, status := promoUsageState(t, setting.ValueJSON)
	if usedCount != 0 || status != string(admincontrolcontract.PromoCodeStatusActive) {
		t.Fatalf("expected released promo usage, used=%d status=%s value=%+v", usedCount, status, setting.ValueJSON)
	}
	if _, err := store.PreviewPromoCode(ctx, contract.PromoCodePreviewInput{
		UserID:   7,
		Code:     "SAVE10",
		Amount:   "60.00000000",
		Currency: "USD",
		Now:      now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatalf("expected promo reusable after release: %v", err)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "payments.db") + "?_fk=1"
}

func seedPromoCodes(t *testing.T, ctx context.Context, client *ent.Client, value map[string]any) {
	t.Helper()
	_, err := client.Setting.Create().
		SetKey("admin_control.promo_codes").
		SetValueJSON(value).
		SetDescription("promo test setting").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed promo setting: %v", err)
	}
}

func promoUsageState(t *testing.T, value map[string]any) (int, string) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal promo setting: %v", err)
	}
	var collection struct {
		Items []struct {
			UsedCount int    `json:"used_count"`
			Status    string `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &collection); err != nil {
		t.Fatalf("unmarshal promo setting: %v", err)
	}
	if len(collection.Items) != 1 {
		t.Fatalf("expected one promo code, got %+v", collection.Items)
	}
	return collection.Items[0].UsedCount, collection.Items[0].Status
}

func intPtr(value int) *int {
	return &value
}
