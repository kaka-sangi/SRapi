package billing

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entusagelog "github.com/srapi/srapi/apps/api/ent/usagelog"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestChargeUsageCreatesLedgerUpdatesBalanceAndMarksUsageLogs(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new billing store: %v", err)
	}

	ctx := context.Background()
	userID := createUser(t, client, "billing@srapi.local", "1.00000000")
	firstID := createUsageLog(t, client, userID, "req_charge_first", "0.25000000", nil)
	secondID := createUsageLog(t, client, userID, "req_charge_second", "0.50000000", nil)
	alreadyCharged := time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC)
	createUsageLog(t, client, userID, "req_already_charged", "0.25000000", &alreadyCharged)

	pending, err := store.ListPendingUsageCharges(ctx, 10)
	if err != nil {
		t.Fatalf("list pending usage charges: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending usage charges, got %+v", pending)
	}

	chargedAt := time.Date(2026, 5, 24, 9, 30, 0, 0, time.UTC)
	result, err := store.ChargeUsage(ctx, contract.ChargeUsageRequest{
		UserID:      userID,
		Currency:    "usd",
		UsageLogIDs: []int{secondID, firstID, firstID},
		ChargedAt:   chargedAt,
	})
	if err != nil {
		t.Fatalf("charge usage: %v", err)
	}
	if result.LedgerEntry.Amount != "0.75000000" || result.BalanceBefore != "1.00000000" || result.BalanceAfter != "0.25000000" {
		t.Fatalf("unexpected charge result: %+v", result)
	}
	if result.LedgerEntry.Type != contract.LedgerTypeUsageCharge || result.LedgerEntry.Currency != "USD" || result.UserDisabled {
		t.Fatalf("unexpected ledger result: %+v", result)
	}

	user, err := client.User.Get(ctx, userID)
	if err != nil {
		t.Fatalf("load charged user: %v", err)
	}
	if user.Balance != "0.25000000" || user.Currency != "USD" {
		t.Fatalf("expected updated user balance, got %+v", user)
	}
	for _, id := range []int{firstID, secondID} {
		log, err := client.UsageLog.Get(ctx, id)
		if err != nil {
			t.Fatalf("load charged usage log %d: %v", id, err)
		}
		if log.ChargedAt == nil || !log.ChargedAt.Equal(chargedAt) {
			t.Fatalf("expected usage log %d charged at %s, got %v", id, chargedAt, log.ChargedAt)
		}
	}

	pending, err = store.ListPendingUsageCharges(ctx, 10)
	if err != nil {
		t.Fatalf("list pending usage charges after charge: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending usage charges, got %+v", pending)
	}
}

func TestListPendingUsageChargesOrdersByCreatedAtThenID(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new billing store: %v", err)
	}

	ctx := context.Background()
	userID := createUser(t, client, "ordered-pending@srapi.local", "1.00000000")
	older := time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC)
	newer := older.Add(time.Minute)
	newerID := createUsageLogAt(t, client, userID, "req_pending_newer", "0.10000000", nil, newer)
	olderID := createUsageLogAt(t, client, userID, "req_pending_older", "0.20000000", nil, older)

	pending, err := store.ListPendingUsageCharges(ctx, 10)
	if err != nil {
		t.Fatalf("list pending usage charges: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("expected two pending usage charges, got %+v", pending)
	}
	if pending[0].UsageLogID != olderID || pending[1].UsageLogID != newerID {
		t.Fatalf("expected oldest pending usage first, got %+v", pending)
	}
}

func TestChargeUsageFlagsNegativeBalance(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new billing store: %v", err)
	}

	ctx := context.Background()
	userID := createUser(t, client, "low-balance@srapi.local", "0.10000000")
	usageID := createUsageLog(t, client, userID, "req_negative_balance", "0.25000000", nil)

	result, err := store.ChargeUsage(ctx, contract.ChargeUsageRequest{
		UserID:      userID,
		Currency:    "USD",
		UsageLogIDs: []int{usageID},
	})
	if err != nil {
		t.Fatalf("charge usage: %v", err)
	}
	if !result.UserDisabled || result.BalanceAfter != "-0.15000000" {
		t.Fatalf("expected negative balance to flag user disabled, got %+v", result)
	}
}

func TestChargeUsageRejectsCurrencyMismatch(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new billing store: %v", err)
	}

	ctx := context.Background()
	userID := createUser(t, client, "currency@srapi.local", "1.00000000")
	usageID := createUsageLog(t, client, userID, "req_currency_mismatch", "0.25000000", nil)

	result, err := store.ChargeUsage(ctx, contract.ChargeUsageRequest{
		UserID:      userID,
		Currency:    "EUR",
		UsageLogIDs: []int{usageID},
	})
	if err != nil {
		t.Fatalf("charge usage with currency mismatch: %v", err)
	}
	if result.LedgerEntry.ID != 0 || len(result.ChargedUsageLogIDs) != 0 {
		t.Fatalf("expected currency mismatch to leave batch uncharged, got %+v", result)
	}

	log, err := client.UsageLog.Query().Where(entusagelog.IDEQ(usageID)).Only(ctx)
	if err != nil {
		t.Fatalf("load usage log: %v", err)
	}
	if log.ChargedAt != nil {
		t.Fatalf("expected rejected currency mismatch to leave log uncharged, got %v", log.ChargedAt)
	}
}

func TestStorePersistsPricingRules(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new billing store: %v", err)
	}

	ctx := context.Background()
	from := time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)
	threshold := 200000
	rule, err := store.CreatePricingRule(ctx, contract.PricingRule{
		ModelID:                           11,
		ProviderID:                        22,
		BillingMode:                       contract.BillingModeImage,
		InputPricePerMillionTokens:        "1.25000000",
		OutputPricePerMillionTokens:       "2.50000000",
		CacheReadPricePerMillionTokens:    "0.10000000",
		CacheWritePricePerMillionTokens:   "0.20000000",
		CacheWrite5mPricePerMillionTokens: "0.20000000",
		CacheWrite1hPricePerMillionTokens: "0.40000000",
		ImageOutputPricePerMillionTokens:  "5.00000000",
		PerRequestPrice:                   "0.03000000",
		ServiceTierMultipliers:            map[string]string{"priority": "2.00000000"},
		LongContextThresholdTokens:        &threshold,
		LongContextMultiplier:             "2.00000000",
		Intervals: []contract.PricingInterval{
			{ImageSize: "1024x1024", PerImagePrice: "0.04000000"},
		},
		Currency:      "USD",
		EffectiveFrom: &from,
		EffectiveTo:   &to,
	})
	if err != nil {
		t.Fatalf("create pricing rule: %v", err)
	}
	rules, err := store.ListPricingRules(ctx)
	if err != nil {
		t.Fatalf("list pricing rules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID || rules[0].EffectiveFrom == nil || rules[0].EffectiveTo == nil {
		t.Fatalf("expected persisted pricing rule with effectivity, got %+v", rules)
	}
	if rules[0].BillingMode != contract.BillingModeImage || rules[0].PerRequestPrice != "0.03000000" || len(rules[0].Intervals) != 1 || rules[0].Intervals[0].PerImagePrice != "0.04000000" {
		t.Fatalf("expected persisted billing mode and interval, got %+v", rules[0])
	}
	if rules[0].CacheWrite1hPricePerMillionTokens != "0.40000000" ||
		rules[0].ImageOutputPricePerMillionTokens != "5.00000000" ||
		rules[0].ServiceTierMultipliers["priority"] != "2.00000000" ||
		rules[0].LongContextThresholdTokens == nil || *rules[0].LongContextThresholdTokens != threshold ||
		rules[0].LongContextMultiplier != "2.00000000" {
		t.Fatalf("expected persisted batch13 pricing dimensions, got %+v", rules[0])
	}
}

func createUser(t *testing.T, client *ent.Client, email, balance string) int {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(email).
		SetName("Billing User").
		SetPasswordHash("hash").
		SetStatus("active").
		SetBalance(balance).
		SetCurrency("USD").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user.ID
}

func createUsageLog(t *testing.T, client *ent.Client, userID int, requestID, cost string, chargedAt *time.Time) int {
	t.Helper()
	return createUsageLogAt(t, client, userID, requestID, cost, chargedAt, time.Time{})
}

func createUsageLogAt(t *testing.T, client *ent.Client, userID int, requestID, cost string, chargedAt *time.Time, createdAt time.Time) int {
	t.Helper()
	create := client.UsageLog.Create().
		SetRequestID(requestID).
		SetUserID(userID).
		SetAPIKeyID(1).
		SetSourceEndpoint("/v1/chat/completions").
		SetModel("billing-model").
		SetInputTokens(5).
		SetOutputTokens(7).
		SetTotalTokens(12).
		SetSuccess(true).
		SetCost(cost).
		SetBillableCost(cost).
		SetCurrency("USD")
	if chargedAt != nil {
		create.SetChargedAt(*chargedAt)
	}
	if !createdAt.IsZero() {
		create.SetCreatedAt(createdAt).SetUpdatedAt(createdAt)
	}
	log, err := create.Save(context.Background())
	if err != nil {
		t.Fatalf("create usage log: %v", err)
	}
	return log.ID
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "billing.db") + "?_fk=1"
}

func TestChargeUsageRejectsPartiallyAlreadyChargedBatch(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new billing store: %v", err)
	}

	ctx := context.Background()
	userID := createUser(t, client, "partial@srapi.local", "1.00000000")
	pendingID := createUsageLog(t, client, userID, "req_pending_partial", "0.25000000", nil)
	chargedAt := time.Date(2026, 5, 24, 8, 0, 0, 0, time.UTC)
	chargedID := createUsageLog(t, client, userID, "req_done_partial", "0.25000000", &chargedAt)

	if _, err := store.ChargeUsage(ctx, contract.ChargeUsageRequest{
		UserID:      userID,
		UsageLogIDs: []int{pendingID, chargedID},
	}); err == nil {
		t.Fatal("expected partially charged batch to be rejected")
	}

	log, err := client.UsageLog.Query().Where(entusagelog.IDEQ(pendingID)).Only(ctx)
	if err != nil {
		t.Fatalf("load pending usage log: %v", err)
	}
	if log.ChargedAt != nil {
		t.Fatalf("expected rejected batch to leave pending log uncharged, got %v", log.ChargedAt)
	}
}
