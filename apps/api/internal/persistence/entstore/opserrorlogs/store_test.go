package opserrorlogs

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreListsFiltersAndUpdatesResolution(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	userID := 7
	accountID := 11
	providerID := 13
	apiKeyID := 17
	statusBadGateway := 502
	statusRateLimit := 429
	latencyMS := 321

	first, err := store.Insert(ctx, contract.Entry{
		OccurredAt:        now.Add(-time.Minute),
		RequestID:         "req_first",
		TraceID:           "trace_first",
		UserID:            &userID,
		APIKeyID:          &apiKeyID,
		AccountID:         &accountID,
		ProviderID:        &providerID,
		Platform:          "openai-compatible",
		SourceEndpoint:    "/v1/responses",
		TargetProtocol:    "openai-compatible",
		Model:             "codex-mini",
		StatusCode:        &statusBadGateway,
		UpstreamRequestID: "upstream_req_first",
		AttemptNo:         2,
		LatencyMS:         latencyMS,
		InputTokens:       10,
		OutputTokens:      2,
		UsageEstimated:    true,
		ErrorClass:        "server_bad",
		ErrorPhase:        "upstream",
		ErrorOwner:        "provider",
		ErrorSource:       "upstream_http",
		ErrorMessage:      "provider returned 502",
		ErrorBodyExcerpt:  `{"error":"bad gateway"}`,
		UpstreamErrors: []contract.UpstreamErrorEvent{{
			AtUnixMs:           now.UnixMilli(),
			AttemptNo:          1,
			AccountID:          &accountID,
			AccountName:        "primary",
			UpstreamStatusCode: 502,
			UpstreamRequestID:  "upstream_req_first",
			UpstreamURL:        "codex-mini",
			Kind:               "http_error",
			Message:            "provider returned 502",
			BodyExcerpt:        `{"error":"bad gateway"}`,
		}},
		Resolution: contract.ResolutionOpen,
		CreatedAt:  now.Add(-time.Minute),
		UpdatedAt:  now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	second, err := store.Insert(ctx, contract.Entry{
		OccurredAt:     now,
		RequestID:      "req_second",
		TraceID:        "trace_second",
		AccountID:      &accountID,
		ProviderID:     &providerID,
		Platform:       "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "gpt-4o-mini",
		StatusCode:     &statusRateLimit,
		ErrorClass:     "rate_limit",
		ErrorPhase:     "upstream",
		ErrorMessage:   "provider quota exceeded",
		Resolution:     contract.ResolutionOpen,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("insert second: %v", err)
	}

	list, err := store.List(ctx, contract.ListFilter{AccountID: &accountID, Query: "quota", Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list filtered: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != second.ID {
		t.Fatalf("expected only quota row, got %+v", list)
	}

	list, err = store.List(ctx, contract.ListFilter{StatusCodeMin: &statusBadGateway, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list status filter: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].ID != first.ID {
		t.Fatalf("expected only 5xx row, got %+v", list)
	}

	list, err = store.List(ctx, contract.ListFilter{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(list.Items) != 2 || list.Items[0].ID != second.ID || list.Items[1].ID != first.ID {
		t.Fatalf("expected newest-first ordering, got %+v", list.Items)
	}

	resolverID := 23
	resolvedAt := now.Add(time.Minute)
	updated, err := store.UpdateResolution(ctx, contract.UpdateResolutionRequest{
		ID:           first.ID,
		Resolution:   contract.ResolutionResolved,
		Note:         "rotated account credential",
		ResolvedByID: &resolverID,
		At:           resolvedAt,
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if updated.Resolution != contract.ResolutionResolved || updated.ResolvedAt == nil || updated.ResolvedByID == nil || *updated.ResolvedByID != resolverID {
		t.Fatalf("unexpected resolved row: %+v", updated)
	}

	reopened, err := store.UpdateResolution(ctx, contract.UpdateResolutionRequest{
		ID:         first.ID,
		Resolution: contract.ResolutionInvestigating,
		Note:       "still happening",
		At:         resolvedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.ResolvedAt != nil || reopened.ResolvedByID != nil {
		t.Fatalf("expected reopen to clear resolved metadata, got %+v", reopened)
	}

	found, err := store.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("get first: %v", err)
	}
	if found.Resolution != contract.ResolutionInvestigating || found.ResolutionNote != "still happening" {
		t.Fatalf("unexpected found row: %+v", found)
	}
	if found.TargetProtocol != "openai-compatible" || found.UpstreamRequestID != "upstream_req_first" || found.AttemptNo != 2 || found.LatencyMS != latencyMS || !found.UsageEstimated {
		t.Fatalf("missing structured evidence on found row: %+v", found)
	}
	if len(found.UpstreamErrors) != 1 || found.UpstreamErrors[0].AccountID == nil || *found.UpstreamErrors[0].AccountID != accountID {
		t.Fatalf("missing upstream history: %+v", found.UpstreamErrors)
	}
}

func TestStoreDeleteOlderThanAndNotFound(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	ctx := context.Background()
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.Insert(ctx, contract.Entry{OccurredAt: old, RequestID: "req_old", ErrorMessage: "old"}); err != nil {
		t.Fatalf("insert old: %v", err)
	}
	recentRow, err := store.Insert(ctx, contract.Entry{OccurredAt: recent, RequestID: "req_recent", ErrorMessage: "recent"})
	if err != nil {
		t.Fatalf("insert recent: %v", err)
	}

	removed, err := store.DeleteOlderThan(ctx, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("delete older than: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed: got %d want 1", removed)
	}
	if _, err := store.Get(ctx, recentRow.ID); err != nil {
		t.Fatalf("expected recent row to remain: %v", err)
	}
	if _, err := store.Get(ctx, 999); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "ops-error-logs.db") + "?_fk=1"
}
