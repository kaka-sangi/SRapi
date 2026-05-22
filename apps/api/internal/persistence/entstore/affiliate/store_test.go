package affiliate

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entbillingledger "github.com/srapi/srapi/apps/api/ent/billingledger"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	"github.com/srapi/srapi/apps/api/internal/modules/affiliate/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStorePersistsAffiliateRelationshipsRulesAndLedgers(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	code, err := store.CreateInviteCode(ctx, contract.InviteCode{
		UserID: 1,
		Code:   "INVITE-1",
		Status: contract.InviteCodeStatusActive,
	})
	if err != nil {
		t.Fatalf("create invite code: %v", err)
	}
	if code.ID == 0 || code.Code != "INVITE-1" {
		t.Fatalf("unexpected invite code: %+v", code)
	}
	foundCode, err := store.FindInviteCodeByCode(ctx, "INVITE-1")
	if err != nil {
		t.Fatalf("find invite code: %v", err)
	}
	if foundCode.ID != code.ID {
		t.Fatalf("unexpected invite lookup: %+v", foundCode)
	}

	relationship, err := store.CreateRelationship(ctx, contract.InviteRelationship{
		InviterUserID: code.UserID,
		InviteeUserID: 2,
		InviteCodeID:  code.ID,
		Status:        contract.RelationshipStatusActive,
	})
	if err != nil {
		t.Fatalf("create relationship: %v", err)
	}
	firstPaidAt := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	updated, err := store.MarkRelationshipFirstPaid(ctx, relationship.ID, firstPaidAt)
	if err != nil {
		t.Fatalf("mark first paid: %v", err)
	}
	if updated.FirstPaidAt == nil || !updated.FirstPaidAt.Equal(firstPaidAt) {
		t.Fatalf("expected first paid timestamp, got %+v", updated)
	}

	validFrom := firstPaidAt.Add(-time.Hour)
	rule, err := store.CreateRule(ctx, contract.AffiliateRule{
		Name:            "ten-percent",
		Status:          contract.RuleStatusActive,
		TriggerType:     contract.TriggerTypePaymentPaid,
		Rate:            "0.10000000",
		FixedAmount:     "0.00000000",
		Currency:        "USD",
		MaxRebateAmount: "0.00000000",
		ValidFrom:       &validFrom,
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	effective, err := store.GetEffectiveRule(ctx, contract.TriggerTypePaymentPaid, "USD", firstPaidAt)
	if err != nil {
		t.Fatalf("get effective rule: %v", err)
	}
	if effective.ID != rule.ID {
		t.Fatalf("unexpected effective rule: %+v", effective)
	}

	orderID := 99
	ledger, created, err := store.AppendLedger(ctx, contract.AffiliateLedger{
		UserID:         1,
		RelatedUserID:  2,
		PaymentOrderID: &orderID,
		Type:           contract.LedgerTypeAccrue,
		Amount:         "10.00000000",
		Currency:       "USD",
		Status:         contract.LedgerStatusPending,
		ReferenceID:    "payment_paid:pay_99",
		Metadata:       map[string]any{"payment_amount": "100.00000000"},
	})
	if err != nil || !created {
		t.Fatalf("append ledger: ledger=%+v created=%v err=%v", ledger, created, err)
	}
	duplicate, created, err := store.AppendLedger(ctx, contract.AffiliateLedger{
		UserID:         1,
		RelatedUserID:  2,
		PaymentOrderID: &orderID,
		Type:           contract.LedgerTypeAccrue,
		Amount:         "10.00000000",
		Currency:       "USD",
		Status:         contract.LedgerStatusPending,
		ReferenceID:    "payment_paid:pay_99",
	})
	if err != nil || created || duplicate.ID != ledger.ID {
		t.Fatalf("expected idempotent ledger append: duplicate=%+v created=%v err=%v", duplicate, created, err)
	}
	ledgers, err := store.ListLedgersByPaymentOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("list ledgers by order: %v", err)
	}
	if len(ledgers) != 1 || ledgers[0].ReferenceID != ledger.ReferenceID {
		t.Fatalf("unexpected order ledgers: %+v", ledgers)
	}
}

func TestTransferToBalanceWritesAffiliateLedgerBillingLedgerAndUserBalance(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	user, err := client.User.Create().
		SetEmail("affiliate@srapi.local").
		SetName("Affiliate").
		SetPasswordHash("hash").
		SetStatus("active").
		SetBalance("1.00000000").
		SetCurrency("USD").
		Save(ctx)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	orderID := 88
	if _, created, err := store.AppendLedger(ctx, contract.AffiliateLedger{
		UserID:         user.ID,
		RelatedUserID:  2,
		PaymentOrderID: &orderID,
		Type:           contract.LedgerTypeAccrue,
		Amount:         "10.00000000",
		Currency:       "USD",
		Status:         contract.LedgerStatusPending,
		ReferenceID:    "payment_paid:pay_88",
	}); err != nil || !created {
		t.Fatalf("seed accrual: created=%v err=%v", created, err)
	}

	now := time.Date(2026, 5, 22, 13, 0, 0, 0, time.UTC)
	result, created, err := store.TransferToBalance(ctx, contract.TransferToBalanceInput{
		UserID:      user.ID,
		Amount:      "6.00000000",
		Currency:    "USD",
		ReferenceID: "transfer_to_balance:idem_88",
		Metadata:    map[string]any{"idempotency_key": "idem_88"},
		CreatedAt:   now,
	})
	if err != nil || !created {
		t.Fatalf("transfer to balance: result=%+v created=%v err=%v", result, created, err)
	}
	if result.AffiliateLedger.Amount != "-6.00000000" || result.BillingLedgerID == 0 || result.BalanceBefore != "1.00000000" || result.BalanceAfter != "7.00000000" {
		t.Fatalf("unexpected transfer result: %+v", result)
	}
	loadedUser, err := client.User.Query().Where(entuser.IDEQ(user.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if loadedUser.Balance != "7.00000000" || loadedUser.Currency != "USD" {
		t.Fatalf("expected updated user balance, got %+v", loadedUser)
	}
	billingRows, err := client.BillingLedger.Query().Where(entbillingledger.UserIDEQ(user.ID)).All(ctx)
	if err != nil {
		t.Fatalf("query billing ledger: %v", err)
	}
	if len(billingRows) != 1 || billingRows[0].Type != string(billingcontract.LedgerTypeAffiliateTransfer) || billingRows[0].ReferenceID != "2" {
		t.Fatalf("expected affiliate transfer billing ledger, got %+v", billingRows)
	}

	duplicate, created, err := store.TransferToBalance(ctx, contract.TransferToBalanceInput{
		UserID:      user.ID,
		Amount:      "6.00000000",
		Currency:    "USD",
		ReferenceID: "transfer_to_balance:idem_88",
		CreatedAt:   now.Add(time.Hour),
	})
	if err != nil || created || duplicate.AffiliateLedger.ID != result.AffiliateLedger.ID {
		t.Fatalf("expected idempotent transfer duplicate, duplicate=%+v created=%v err=%v", duplicate, created, err)
	}
	billingRows, err = client.BillingLedger.Query().Where(entbillingledger.UserIDEQ(user.ID)).All(ctx)
	if err != nil {
		t.Fatalf("query billing ledger after duplicate: %v", err)
	}
	if len(billingRows) != 1 {
		t.Fatalf("duplicate transfer should not create billing rows, got %+v", billingRows)
	}
	if _, _, err := store.TransferToBalance(ctx, contract.TransferToBalanceInput{
		UserID:      user.ID,
		Amount:      "5.00000000",
		Currency:    "USD",
		ReferenceID: "transfer_to_balance:overdraft",
		CreatedAt:   now.Add(2 * time.Hour),
	}); !errors.Is(err, contract.ErrInsufficientBalance) {
		t.Fatalf("expected insufficient affiliate balance, got %v", err)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "affiliate.db") + "?_fk=1"
}
