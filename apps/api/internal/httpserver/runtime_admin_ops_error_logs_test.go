package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminOpsErrorLogsListGetAndResolve(t *testing.T) {
	store := opserrorlogsmemory.New()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	userID := 7
	apiKeyID := 8
	accountID := 9
	providerID := 10
	statusCode := 502
	latencyMS := 456
	inserted, err := store.Insert(t.Context(), opserrorlogscontract.Entry{
		OccurredAt:        now,
		RequestID:         "req_ops_error",
		TraceID:           "trace_ops_error",
		UserID:            &userID,
		APIKeyID:          &apiKeyID,
		APIKeyPrefix:      "sk_ops",
		AccountID:         &accountID,
		ProviderID:        &providerID,
		Platform:          "openai-compatible",
		SourceEndpoint:    "/v1/responses",
		TargetProtocol:    "openai-compatible",
		Model:             "ops-model",
		StatusCode:        &statusCode,
		UpstreamRequestID: "upstream_req_ops",
		AttemptNo:         2,
		LatencyMS:         latencyMS,
		InputTokens:       11,
		OutputTokens:      5,
		UsageEstimated:    true,
		ErrorClass:        "server_bad",
		ErrorPhase:        "upstream",
		ErrorOwner:        "provider",
		ErrorSource:       "upstream_http",
		ErrorMessage:      "provider returned 502",
		ErrorBodyExcerpt:  `{"error":"bad gateway"}`,
		UpstreamErrors: []opserrorlogscontract.UpstreamErrorEvent{{
			AtUnixMs:           now.UnixMilli(),
			AttemptNo:          1,
			AccountID:          &accountID,
			AccountName:        "primary-account",
			UpstreamStatusCode: 502,
			UpstreamRequestID:  "upstream_req_ops",
			UpstreamURL:        "ops-model",
			Kind:               "http_error",
			Message:            "provider returned 502",
			BodyExcerpt:        `{"error":"bad gateway"}`,
		}},
		Resolution: opserrorlogscontract.ResolutionOpen,
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	if err != nil {
		t.Fatalf("insert ops error log: %v", err)
	}
	if _, err := store.Insert(t.Context(), opserrorlogscontract.Entry{
		OccurredAt:   now.Add(-time.Hour),
		RequestID:    "req_other",
		Model:        "other-model",
		ErrorClass:   "rate_limit",
		ErrorMessage: "quota",
		Resolution:   opserrorlogscontract.ResolutionOpen,
	}); err != nil {
		t.Fatalf("insert other ops error log: %v", err)
	}

	handler := New(config.Load(), nil, WithOpsErrorLogsStore(store))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/error-logs?page=1&page_size=20&model=ops-model&q=provider&status_min=500&error_phase=upstream&error_owner=provider&source_endpoint=/v1/responses", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list ops error logs: expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var list apiopenapi.OpsErrorLogListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.Pagination.Total != 1 || len(list.Data) != 1 {
		t.Fatalf("expected one filtered ops error log, got %+v", list)
	}
	row := list.Data[0]
	if row.Id == nil || *row.Id != "1" || row.Model == nil || *row.Model != "ops-model" || row.StatusCode == nil || *row.StatusCode != 502 || row.ApiKeyPrefix == nil || *row.ApiKeyPrefix != "sk_ops" {
		t.Fatalf("unexpected list row: %+v", row)
	}

	invalidFilters := []string{
		"/api/v1/admin/ops/error-logs?account_id=not-an-int",
		"/api/v1/admin/ops/error-logs?status_min=600",
		"/api/v1/admin/ops/error-logs?status_min=500&status_max=400",
		"/api/v1/admin/ops/error-logs?resolution=ignored",
		"/api/v1/admin/ops/error-logs?start=not-a-time",
		"/api/v1/admin/ops/error-logs?start=2026-06-18T11:00:00Z&end=2026-06-18T10:00:00Z",
		"/api/v1/admin/ops/error-logs?page=0",
		"/api/v1/admin/ops/error-logs?page_size=not-an-int",
	}
	for _, path := range invalidFilters {
		t.Run("invalid filter "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.AddCookie(sessionCookie)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/error-logs/1", nil)
	detailReq.AddCookie(sessionCookie)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("get ops error log: expected 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail apiopenapi.OpsErrorLogResponse
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.Data.RequestId == nil || *detail.Data.RequestId != inserted.RequestID {
		t.Fatalf("unexpected detail response: %+v", detail.Data)
	}
	if detail.Data.ApiKeyPrefix == nil || *detail.Data.ApiKeyPrefix != "sk_ops" {
		t.Fatalf("missing api key prefix evidence in detail response: %+v", detail.Data)
	}
	if detail.Data.AttemptNo == nil || *detail.Data.AttemptNo != 2 || detail.Data.LatencyMs == nil || *detail.Data.LatencyMs != latencyMS {
		t.Fatalf("missing attempt evidence in detail response: %+v", detail.Data)
	}
	if detail.Data.TargetProtocol == nil || *detail.Data.TargetProtocol != "openai-compatible" || detail.Data.UpstreamRequestId == nil || *detail.Data.UpstreamRequestId != "upstream_req_ops" {
		t.Fatalf("missing protocol/upstream evidence in detail response: %+v", detail.Data)
	}
	if detail.Data.UpstreamErrors == nil || len(*detail.Data.UpstreamErrors) != 1 {
		t.Fatalf("missing upstream history in detail response: %+v", detail.Data)
	}
	if detail.RequestId == "" {
		t.Fatalf("expected response request_id")
	}

	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/ops/error-logs/1", strings.NewReader(`{"resolution":"resolved","note":"credential rotated"}`))
	patchReq.AddCookie(sessionCookie)
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Header.Set(csrfHeaderName, csrf)
	patchRec := httptest.NewRecorder()
	handler.ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("patch ops error log: expected 200, got %d body=%s", patchRec.Code, patchRec.Body.String())
	}
	var patch apiopenapi.OpsErrorLogResponse
	if err := json.NewDecoder(patchRec.Body).Decode(&patch); err != nil {
		t.Fatalf("decode patch: %v", err)
	}
	if patch.Data.Resolution == nil || *patch.Data.Resolution != apiopenapi.OpsErrorLogResolutionResolved {
		t.Fatalf("expected resolved response, got %+v", patch.Data)
	}
	if patch.Data.ResolvedAt == nil || patch.Data.ResolvedByUserId == nil {
		t.Fatalf("expected resolved metadata, got %+v", patch.Data)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/error-logs/999", nil)
	missingReq.AddCookie(sessionCookie)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing ops error log: expected 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}

func TestAdminOpsErrorLogFingerprints(t *testing.T) {
	store := opserrorlogsmemory.New()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	statusBadGateway := 502
	statusRateLimit := 429
	first, err := store.Insert(t.Context(), opserrorlogscontract.Entry{
		OccurredAt:     now.Add(-time.Minute),
		RequestID:      "req_first",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "ops-model",
		StatusCode:     &statusBadGateway,
		ErrorClass:     "server_bad",
		ErrorPhase:     "upstream",
		ErrorOwner:     "provider",
		ErrorSource:    "upstream_http",
		ErrorMessage:   "provider returned 502 for request req_first after 123ms",
		Resolution:     opserrorlogscontract.ResolutionInvestigating,
		CreatedAt:      now.Add(-time.Minute),
		UpdatedAt:      now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("insert first: %v", err)
	}
	second, err := store.Insert(t.Context(), opserrorlogscontract.Entry{
		OccurredAt:     now,
		RequestID:      "req_second",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "ops-model",
		StatusCode:     &statusBadGateway,
		ErrorClass:     "server_bad",
		ErrorPhase:     "upstream",
		ErrorOwner:     "provider",
		ErrorSource:    "upstream_http",
		ErrorMessage:   "provider returned 503 for request req_second after 456ms",
		Resolution:     opserrorlogscontract.ResolutionOpen,
		CreatedAt:      now,
		UpdatedAt:      now,
	})
	if err != nil {
		t.Fatalf("insert second: %v", err)
	}
	if _, err := store.Insert(t.Context(), opserrorlogscontract.Entry{
		OccurredAt:     now,
		RequestID:      "req_rate_limit",
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "ops-model",
		StatusCode:     &statusRateLimit,
		ErrorClass:     "rate_limit",
		ErrorPhase:     "upstream",
		ErrorOwner:     "provider",
		ErrorSource:    "upstream_http",
		ErrorMessage:   "rate limited",
		Resolution:     opserrorlogscontract.ResolutionOpen,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("insert rate limit: %v", err)
	}

	handler := New(config.Load(), nil, WithOpsErrorLogsStore(store))
	_, sessionCookie := mustLoginAdmin(t, handler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/error-logs/fingerprints?model=ops-model&status_min=500&limit=5&start=2026-06-18T09:00:00Z&end=2026-06-18T11:00:00Z", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("fingerprints: expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			Fingerprint         string `json:"fingerprint"`
			Count               int    `json:"count"`
			OpenCount           int    `json:"open_count"`
			InvestigatingCount  int    `json:"investigating_count"`
			ExampleErrorLogID   string `json:"example_error_log_id"`
			ExampleRequestID    string `json:"example_request_id"`
			StatusClass         string `json:"status_class"`
			StatusCode          int    `json:"status_code"`
			MessagePattern      string `json:"message_pattern"`
			ExampleErrorMessage string `json:"example_error_message"`
		} `json:"data"`
		Meta struct {
			Total       int    `json:"total"`
			Scanned     int    `json:"scanned"`
			Truncated   bool   `json:"truncated"`
			WindowStart string `json:"window_start"`
			WindowEnd   string `json:"window_end"`
		} `json:"meta"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode fingerprints: %v", err)
	}
	if len(resp.Data) != 1 || resp.Meta.Total != 1 || resp.Meta.Scanned != 2 || resp.Meta.Truncated {
		t.Fatalf("unexpected fingerprint response: %+v", resp)
	}
	item := resp.Data[0]
	if item.Fingerprint == "" || item.Count != 2 || item.OpenCount != 1 || item.InvestigatingCount != 1 {
		t.Fatalf("unexpected fingerprint item: %+v", item)
	}
	if item.ExampleErrorLogID != "2" || item.ExampleRequestID != second.RequestID || item.StatusClass != "5xx" || item.StatusCode != statusBadGateway {
		t.Fatalf("unexpected fingerprint example/status: %+v first=%+v", item, first)
	}
	if strings.Contains(item.MessagePattern, "req_first") || strings.Contains(item.MessagePattern, "req_second") || strings.Contains(item.MessagePattern, "123") || strings.Contains(item.MessagePattern, "456") {
		t.Fatalf("message pattern leaked variable identifiers: %+v", item)
	}
	if resp.Meta.WindowStart == "" || resp.Meta.WindowEnd == "" || resp.RequestID == "" {
		t.Fatalf("missing response metadata: %+v", resp)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/error-logs/fingerprints?limit=bad", nil)
	invalidReq.AddCookie(sessionCookie)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid limit: expected 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}
