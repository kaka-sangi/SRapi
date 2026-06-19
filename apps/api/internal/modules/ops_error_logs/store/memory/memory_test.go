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

func TestListFiltersErrorPhaseAndOwner(t *testing.T) {
	store := New()
	ctx := context.Background()
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	upstream, err := store.Insert(ctx, contract.Entry{
		OccurredAt:   now,
		RequestID:    "req_upstream",
		ErrorClass:   "server_bad",
		ErrorPhase:   "upstream",
		ErrorOwner:   "provider",
		ErrorMessage: "provider failed",
	})
	if err != nil {
		t.Fatalf("insert upstream: %v", err)
	}
	if _, err := store.Insert(ctx, contract.Entry{
		OccurredAt:   now.Add(time.Second),
		RequestID:    "req_routing",
		ErrorClass:   "no_available_account",
		ErrorPhase:   "routing",
		ErrorOwner:   "scheduler",
		ErrorMessage: "no available account",
	}); err != nil {
		t.Fatalf("insert routing: %v", err)
	}

	list, err := store.List(ctx, contract.ListFilter{ErrorPhase: "upstream", ErrorOwner: "provider", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list by phase and owner: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != upstream.ID {
		t.Fatalf("expected upstream provider row, got %+v", list)
	}
}

func TestListFiltersSourceEndpoint(t *testing.T) {
	store := New()
	ctx := context.Background()
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	responses, err := store.Insert(ctx, contract.Entry{
		OccurredAt:     now,
		RequestID:      "req_responses",
		SourceEndpoint: "/v1/responses",
		ErrorClass:     "server_bad",
		ErrorMessage:   "server bad",
	})
	if err != nil {
		t.Fatalf("insert responses: %v", err)
	}
	if _, err := store.Insert(ctx, contract.Entry{
		OccurredAt:     now.Add(time.Second),
		RequestID:      "req_chat",
		SourceEndpoint: "/v1/chat/completions",
		ErrorClass:     "server_bad",
		ErrorMessage:   "server bad",
	}); err != nil {
		t.Fatalf("insert chat: %v", err)
	}

	list, err := store.List(ctx, contract.ListFilter{SourceEndpoint: "/v1/responses", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list by source endpoint: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != responses.ID {
		t.Fatalf("expected responses row, got %+v", list)
	}
}

func TestListFiltersEquivalentErrorClasses(t *testing.T) {
	store := New()
	ctx := context.Background()
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	streamTimeout, err := store.Insert(ctx, contract.Entry{
		OccurredAt:   now,
		RequestID:    "req_stream_timeout",
		ErrorClass:   "stream_idle_timeout",
		ErrorMessage: "stream timed out",
	})
	if err != nil {
		t.Fatalf("insert stream timeout: %v", err)
	}
	rateLimit, err := store.Insert(ctx, contract.Entry{
		OccurredAt:   now.Add(time.Second),
		RequestID:    "req_rate_limit",
		ErrorClass:   "rate_limit",
		ErrorMessage: "slow down",
	})
	if err != nil {
		t.Fatalf("insert rate limit: %v", err)
	}

	list, err := store.List(ctx, contract.ListFilter{ErrorClass: "timeout", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list timeout aliases: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != streamTimeout.ID {
		t.Fatalf("expected stream timeout row for timeout filter, got %+v", list)
	}

	list, err = store.List(ctx, contract.ListFilter{ErrorClass: "rate_limit_error", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list rate limit aliases: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != rateLimit.ID {
		t.Fatalf("expected rate limit row for alias filter, got %+v", list)
	}
}
