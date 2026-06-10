package usage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestSummarizeUserWindowFiltersSuccessTimeAndProvider(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	providerA := 11
	providerB := 12

	createUsage(t, ctx, store, contract.UsageLog{RequestID: "before", UserID: 7, APIKeyID: 1, ProviderID: &providerA, TotalTokens: 100, Success: true, BillableCost: "1.00000000", CreatedAt: start.Add(-time.Nanosecond)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "inside-a", UserID: 7, APIKeyID: 1, ProviderID: &providerA, TotalTokens: 200, Success: true, BillableCost: "2.12500000", CreatedAt: start})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "inside-failed", UserID: 7, APIKeyID: 1, ProviderID: &providerA, TotalTokens: 400, Success: false, BillableCost: "4.00000000", CreatedAt: start.Add(time.Hour)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "inside-b", UserID: 7, APIKeyID: 1, ProviderID: &providerB, TotalTokens: 800, Success: true, BillableCost: "8.50000000", CreatedAt: start.Add(2 * time.Hour)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "other-user", UserID: 8, APIKeyID: 1, ProviderID: &providerA, TotalTokens: 1600, Success: true, BillableCost: "16.00000000", CreatedAt: start.Add(3 * time.Hour)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "end-exclusive", UserID: 7, APIKeyID: 1, ProviderID: &providerA, TotalTokens: 3200, Success: true, BillableCost: "32.00000000", CreatedAt: end})

	summary, err := store.SummarizeUserWindow(ctx, contract.UserWindowFilter{
		UserID:      7,
		ProviderID:  &providerA,
		Start:       start,
		End:         end,
		SuccessOnly: true,
	})
	if err != nil {
		t.Fatalf("summarize provider a success: %v", err)
	}
	if summary.TotalTokens != 200 || summary.BillableCost != "2.12500000" {
		t.Fatalf("provider a summary = %+v", summary)
	}

	summary, err = store.SummarizeUserWindow(ctx, contract.UserWindowFilter{
		UserID:      7,
		Start:       start,
		End:         end,
		SuccessOnly: true,
	})
	if err != nil {
		t.Fatalf("summarize all providers success: %v", err)
	}
	if summary.TotalTokens != 1000 || summary.BillableCost != "10.62500000" {
		t.Fatalf("all-provider success summary = %+v", summary)
	}

	summary, err = store.SummarizeUserWindow(ctx, contract.UserWindowFilter{
		UserID: 7,
		Start:  start,
		End:    end,
	})
	if err != nil {
		t.Fatalf("summarize all outcomes: %v", err)
	}
	if summary.TotalTokens != 1400 || summary.BillableCost != "14.62500000" {
		t.Fatalf("all-outcome summary = %+v", summary)
	}
}

func TestListByAccountWindowFiltersTimeAndLimits(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	start := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	accountA := 101
	accountB := 102
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "before", UserID: 7, APIKeyID: 1, AccountID: &accountA, TotalTokens: 10, Success: true, BillableCost: "1.00000000", CreatedAt: start.Add(-time.Nanosecond)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "inside-1", UserID: 7, APIKeyID: 1, AccountID: &accountA, TotalTokens: 20, Success: true, BillableCost: "2.00000000", CreatedAt: start})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "inside-2", UserID: 7, APIKeyID: 1, AccountID: &accountA, TotalTokens: 30, Success: true, BillableCost: "3.00000000", CreatedAt: start.Add(time.Minute)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "inside-3", UserID: 7, APIKeyID: 1, AccountID: &accountA, TotalTokens: 40, Success: true, BillableCost: "4.00000000", CreatedAt: start.Add(2 * time.Minute)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "other-account", UserID: 7, APIKeyID: 1, AccountID: &accountB, TotalTokens: 500, Success: true, BillableCost: "5.00000000", CreatedAt: start.Add(time.Minute)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "end-exclusive", UserID: 7, APIKeyID: 1, AccountID: &accountA, TotalTokens: 60, Success: true, BillableCost: "6.00000000", CreatedAt: end})

	logs, err := store.ListByAccountWindow(ctx, contract.AccountWindowFilter{
		AccountID: accountA,
		Start:     start,
		End:       end,
		Limit:     2,
	})
	if err != nil {
		t.Fatalf("list account window: %v", err)
	}
	if len(logs) != 2 || logs[0].RequestID != "inside-2" || logs[1].RequestID != "inside-3" {
		t.Fatalf("expected latest two account-window rows in ascending ID order, got %+v", logs)
	}
}

func createUsage(t *testing.T, ctx context.Context, store *Store, log contract.UsageLog) {
	t.Helper()
	if log.AttemptNo == 0 {
		log.AttemptNo = 1
	}
	if log.SourceProtocol == "" {
		log.SourceProtocol = "openai"
	}
	if log.SourceEndpoint == "" {
		log.SourceEndpoint = "/v1/chat/completions"
	}
	if log.TargetProtocol == "" {
		log.TargetProtocol = "openai"
	}
	if log.Model == "" {
		log.Model = "gpt-test"
	}
	if log.Currency == "" {
		log.Currency = "USD"
	}
	if _, err := store.Create(ctx, log); err != nil {
		t.Fatalf("create usage %s: %v", log.RequestID, err)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "usage.db") + "?_fk=1"
}
