package admincontrol

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entbillingledger "github.com/srapi/srapi/apps/api/ent/billingledger"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	entuser "github.com/srapi/srapi/apps/api/ent/user"
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestRedeemCodeCreditsBalanceOncePersistently(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	user, err := client.User.Create().
		SetEmail("redeem@srapi.local").
		SetName("Redeem").
		SetPasswordHash("hash").
		SetStatus("active").
		SetBalance("1.00000000").
		SetCurrency("USD").
		Save(ctx)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	createdAt := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	code := admincontrolcontract.RedeemCode{
		Code:           "WELCOME10",
		Type:           admincontrolcontract.RedeemCodeTypeBalance,
		Status:         admincontrolcontract.RedeemCodeStatusActive,
		Value:          "10",
		Currency:       "USD",
		MaxRedemptions: 1,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if err := seedRedeemCodes(ctx, store, []admincontrolcontract.RedeemCode{code}); err != nil {
		t.Fatalf("seed redeem codes: %v", err)
	}

	result, err := store.RedeemCode(ctx, admincontrolcontract.RedeemCodeRedemptionInput{
		UserID:     user.ID,
		Code:       " welcome10 ",
		RedeemedAt: createdAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("redeem code: %v", err)
	}
	if result.AlreadyRedeemed || result.Redemption.Amount != "10.00000000" || result.Redemption.BalanceBefore != "1.00000000" || result.Redemption.BalanceAfter != "11.00000000" {
		t.Fatalf("unexpected redemption result: %+v", result)
	}
	updated, err := client.User.Query().Where(entuser.IDEQ(user.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("load updated user: %v", err)
	}
	if updated.Balance != "11.00000000" {
		t.Fatalf("balance = %s, want 11.00000000", updated.Balance)
	}
	ledgers, err := client.BillingLedger.Query().Where(entbillingledger.UserIDEQ(user.ID)).All(ctx)
	if err != nil {
		t.Fatalf("list billing ledgers: %v", err)
	}
	if len(ledgers) != 1 || ledgers[0].Type != string(billingcontract.LedgerTypeRedeemCodeCredit) || ledgers[0].Amount != "10.00000000" {
		t.Fatalf("unexpected ledger rows: %+v", ledgers)
	}

	repeated, err := store.RedeemCode(ctx, admincontrolcontract.RedeemCodeRedemptionInput{
		UserID:     user.ID,
		Code:       "WELCOME10",
		RedeemedAt: createdAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("repeat redeem code: %v", err)
	}
	if !repeated.AlreadyRedeemed || repeated.Redemption.ID != result.Redemption.ID {
		t.Fatalf("expected idempotent repeat result, got %+v", repeated)
	}
	updated, _ = client.User.Query().Where(entuser.IDEQ(user.ID)).Only(ctx)
	ledgers, _ = client.BillingLedger.Query().Where(entbillingledger.UserIDEQ(user.ID)).All(ctx)
	if updated.Balance != "11.00000000" || len(ledgers) != 1 {
		t.Fatalf("repeat redemption changed side effects: user=%+v ledgers=%+v", updated, ledgers)
	}
}

func seedRedeemCodes(ctx context.Context, store *Store, codes []admincontrolcontract.RedeemCode) error {
	for _, code := range codes {
		if _, err := store.CreateRedeemCode(ctx, code); err != nil {
			return err
		}
	}
	return nil
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "admincontrol.db") + "?_fk=1"
}
