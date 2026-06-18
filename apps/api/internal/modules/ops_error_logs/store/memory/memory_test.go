package memory

import (
	"context"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
)

func TestListFiltersExactRequestID(t *testing.T) {
	store := New()
	ctx := context.Background()
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	first, err := store.Insert(ctx, contract.Entry{
		OccurredAt:   now,
		RequestID:    "req_exact",
		ErrorClass:   "timeout",
		ErrorMessage: "timeout",
	})
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	if _, err := store.Insert(ctx, contract.Entry{
		OccurredAt:   now.Add(time.Second),
		RequestID:    "req_exact_neighbor",
		ErrorClass:   "server_bad",
		ErrorMessage: "server bad",
	}); err != nil {
		t.Fatalf("insert neighbor: %v", err)
	}

	list, err := store.List(ctx, contract.ListFilter{RequestID: "req_exact", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list by request id: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != first.ID {
		t.Fatalf("expected exact request id row, got %+v", list)
	}
}
