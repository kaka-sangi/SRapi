package availabilityrollup

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	healthrollupsmemory "github.com/srapi/srapi/apps/api/internal/modules/health_rollups/store/memory"
)

const testMasterKey = "availability_rollup_master_key_32"

func TestRunOnceMaterializesRollupsForUnviewedAccounts(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	accounts := accountmemory.New()
	rollups := healthrollupsmemory.New()
	active := mustCreateAccount(t, accounts, "active")
	disabled := accountcontract.StatusDisabled
	inactive := mustCreateAccount(t, accounts, "disabled")
	inactive.Status = disabled
	if _, err := accounts.Update(ctx, inactive); err != nil {
		t.Fatalf("disable account: %v", err)
	}
	recordSnapshot(t, accounts, active.ID, active.ProviderID, "healthy", 0.90, now.Add(-time.Hour))
	recordSnapshot(t, accounts, active.ID, active.ProviderID, "dead", 0.10, now.Add(-2*time.Hour))
	recordSnapshot(t, accounts, active.ID, active.ProviderID, "healthy", 1, now.AddDate(0, 0, -8))
	recordSnapshot(t, accounts, inactive.ID, inactive.ProviderID, "healthy", 1, now.Add(-time.Hour))

	worker, err := New(accounts, rollups, discardLogger(), Config{
		MasterKey:  testMasterKey,
		Clock:      fixedClock{now: now},
		WindowDays: 7,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Selected != 2 || result.Skipped != 1 || result.Refreshed != 1 || result.Rollups != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	persisted, err := rollups.ListRollupsByAccount(ctx, active.ID, "2026-06-01")
	if err != nil {
		t.Fatalf("list active rollups: %v", err)
	}
	if len(persisted) != 1 {
		t.Fatalf("expected one active rollup, got %+v", persisted)
	}
	if persisted[0].Date != "2026-06-10" || persisted[0].TotalSamples != 2 || persisted[0].HealthySamples != 1 {
		t.Fatalf("unexpected active rollup: %+v", persisted[0])
	}
	if persisted[0].AvailabilityRatio != 0.5 || persisted[0].AvgSuccessRate != 0.5 {
		t.Fatalf("unexpected active ratios: %+v", persisted[0])
	}
	inactiveRollups, err := rollups.ListRollupsByAccount(ctx, inactive.ID, "2026-06-01")
	if err != nil {
		t.Fatalf("list inactive rollups: %v", err)
	}
	if len(inactiveRollups) != 0 {
		t.Fatalf("disabled accounts should not be materialized, got %+v", inactiveRollups)
	}
}

func TestRunOnceSkipsAccountsWithoutRecentSamples(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	accounts := accountmemory.New()
	rollups := healthrollupsmemory.New()
	account := mustCreateAccount(t, accounts, "stale")
	recordSnapshot(t, accounts, account.ID, account.ProviderID, "healthy", 1, now.AddDate(0, 0, -10))

	worker, err := New(accounts, rollups, discardLogger(), Config{
		MasterKey:  testMasterKey,
		Clock:      fixedClock{now: now},
		WindowDays: 7,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Selected != 1 || result.Skipped != 1 || result.Refreshed != 0 || result.Rollups != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func mustCreateAccount(t *testing.T, store *accountmemory.Store, name string) accountcontract.ProviderAccount {
	t.Helper()
	account, err := store.Create(context.Background(), accountcontract.CreateStoredAccount{
		ProviderID:           7,
		Name:                 name,
		RuntimeClass:         accountcontract.RuntimeClassAPIKey,
		CredentialCiphertext: "ciphertext",
		CredentialVersion:    "v1",
		Status:               accountcontract.StatusActive,
		Weight:               1,
		Metadata:             map[string]any{},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return account
}

func recordSnapshot(t *testing.T, store *accountmemory.Store, accountID int, providerID int, status string, successRate float32, at time.Time) {
	t.Helper()
	if _, err := store.RecordHealthSnapshot(context.Background(), accountcontract.AccountHealthSnapshot{
		AccountID:    accountID,
		ProviderID:   providerID,
		Status:       status,
		SuccessRate:  successRate,
		SnapshotAt:   at,
		CircuitState: "closed",
	}); err != nil {
		t.Fatalf("record health snapshot: %v", err)
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }
