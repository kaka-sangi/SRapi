package memory

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
)

func TestListByRequestIDFiltersExactAndOrdersAttempts(t *testing.T) {
	store := New()
	ctx := context.Background()
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "req_exact", AttemptNo: 2, UserID: 7, APIKeyID: 1, TotalTokens: 20, Success: false, CreatedAt: now.Add(time.Second)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "req_exact_neighbor", AttemptNo: 1, UserID: 7, APIKeyID: 1, TotalTokens: 999, Success: true, CreatedAt: now.Add(2 * time.Second)})
	createUsage(t, ctx, store, contract.UsageLog{RequestID: "req_exact", AttemptNo: 1, UserID: 7, APIKeyID: 1, TotalTokens: 10, Success: false, CreatedAt: now})

	logs, err := store.ListByRequestID(ctx, "req_exact")
	if err != nil {
		t.Fatalf("list by request id: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 exact request rows, got %+v", logs)
	}
	if logs[0].RequestID != "req_exact" || logs[0].AttemptNo != 1 || logs[1].RequestID != "req_exact" || logs[1].AttemptNo != 2 {
		t.Fatalf("expected exact rows ordered by attempt, got %+v", logs)
	}
}

func createUsage(t *testing.T, ctx context.Context, store *Store, log contract.UsageLog) {
	t.Helper()
	if _, err := store.Create(ctx, log); err != nil {
		t.Fatalf("create usage %s: %v", log.RequestID, err)
	}
}
