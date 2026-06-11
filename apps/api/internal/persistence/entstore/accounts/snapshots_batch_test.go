package accounts

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	contract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
)

func TestBatchLatestSnapshotsAcrossAccounts(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	createAccount := func(name string) contract.ProviderAccount {
		t.Helper()
		account, err := store.Create(ctx, contract.CreateStoredAccount{
			ProviderID:           3,
			Name:                 name,
			RuntimeClass:         contract.RuntimeClassAPIKey,
			CredentialCiphertext: "ciphertext",
			CredentialVersion:    "v1",
			Status:               contract.StatusActive,
			Priority:             1,
			Weight:               1,
		})
		if err != nil {
			t.Fatalf("create account %s: %v", name, err)
		}
		return account
	}
	first := createAccount("batch-a")
	second := createAccount("batch-b")
	bare := createAccount("batch-no-snapshots")

	now := time.Date(2026, 6, 11, 9, 0, 0, 0, time.UTC)
	recordHealth := func(accountID int, successRate float32, at time.Time) {
		t.Helper()
		if _, err := store.RecordHealthSnapshot(ctx, contract.AccountHealthSnapshot{
			AccountID:    accountID,
			ProviderID:   3,
			Status:       "healthy",
			SuccessRate:  successRate,
			CircuitState: "closed",
			SnapshotAt:   at,
		}); err != nil {
			t.Fatalf("record health: %v", err)
		}
	}
	recordHealth(first.ID, 0.50, now.Add(-2*time.Minute))
	recordHealth(first.ID, 0.90, now.Add(-time.Minute))
	recordHealth(second.ID, 0.70, now.Add(-3*time.Minute))
	recordHealth(first.ID, 0.10, now.Add(-10*time.Minute))

	health, err := store.LatestHealthSnapshotsByAccounts(ctx, []int{first.ID, second.ID, bare.ID})
	if err != nil {
		t.Fatalf("batch health: %v", err)
	}
	if len(health) != 2 {
		t.Fatalf("expected snapshots for 2 accounts, got %+v", health)
	}
	if health[first.ID].SuccessRate != 0.90 {
		t.Fatalf("expected newest snapshot for first account, got %+v", health[first.ID])
	}
	if health[second.ID].SuccessRate != 0.70 {
		t.Fatalf("expected snapshot for second account, got %+v", health[second.ID])
	}
	if _, ok := health[bare.ID]; ok {
		t.Fatalf("expected no snapshot for bare account")
	}

	recordQuota := func(accountID int, quotaType string, ratio float32, at time.Time) {
		t.Helper()
		if _, err := store.RecordQuotaSnapshot(ctx, contract.AccountQuotaSnapshot{
			AccountID:      accountID,
			ProviderID:     3,
			QuotaType:      quotaType,
			Remaining:      "1",
			Used:           "1",
			QuotaLimit:     "100",
			RemainingRatio: ratio,
			SnapshotAt:     at,
		}); err != nil {
			t.Fatalf("record quota: %v", err)
		}
	}
	recordQuota(first.ID, "codex_5h_percent", 0.80, now.Add(-4*time.Minute))
	recordQuota(first.ID, "codex_5h_percent", 0.40, now.Add(-time.Minute))
	recordQuota(first.ID, "codex_7d_percent", 0.90, now.Add(-2*time.Minute))
	recordQuota(second.ID, "codex_5h_percent", 0.65, now.Add(-time.Minute))
	recordQuota(first.ID, "codex_5h_percent", 0.05, now.Add(-10*time.Minute))

	quotas, err := store.LatestQuotaSnapshotsByAccounts(ctx, []int{first.ID, second.ID, bare.ID})
	if err != nil {
		t.Fatalf("batch quotas: %v", err)
	}
	if len(quotas) != 2 {
		t.Fatalf("expected quota entries for 2 accounts, got %+v", quotas)
	}
	firstQuotas := quotas[first.ID]
	if len(firstQuotas) != 2 {
		t.Fatalf("expected one latest snapshot per type, got %+v", firstQuotas)
	}
	if firstQuotas[0].SnapshotAt.Before(firstQuotas[1].SnapshotAt) {
		t.Fatalf("expected newest-first ordering, got %+v", firstQuotas)
	}
	byType := map[string]float32{}
	for _, snapshot := range firstQuotas {
		byType[snapshot.QuotaType] = snapshot.RemainingRatio
	}
	if byType["codex_5h_percent"] != 0.40 || byType["codex_7d_percent"] != 0.90 {
		t.Fatalf("expected latest ratio per type, got %+v", firstQuotas)
	}
	if len(quotas[second.ID]) != 1 || quotas[second.ID][0].RemainingRatio != 0.65 {
		t.Fatalf("unexpected second account quotas: %+v", quotas[second.ID])
	}

	emptyHealth, err := store.LatestHealthSnapshotsByAccounts(ctx, nil)
	if err != nil || len(emptyHealth) != 0 {
		t.Fatalf("empty input should return empty map, got %v, %v", emptyHealth, err)
	}
}
