package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	opserrorlogscontract "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/contract"
	opserrorlogsservice "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/service"
	opserrorlogsmemory "github.com/srapi/srapi/apps/api/internal/modules/ops_error_logs/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminOpsRealtimeTrafficSummarizesUsageAndErrorOnlyRows(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	currentAt := now.Add(-10 * time.Second)
	usageStore := usagememory.New()
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_current_success",
		AttemptNo:      1,
		UserID:         1,
		APIKeyID:       2,
		SourceEndpoint: "/v1/responses",
		Model:          "traffic-model",
		InputTokens:    10,
		OutputTokens:   5,
		TotalTokens:    15,
		Success:        true,
		CreatedAt:      currentAt,
	})
	errClass := "provider_5xx"
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_usage_error",
		AttemptNo:      1,
		UserID:         1,
		APIKeyID:       2,
		SourceEndpoint: "/v1/responses",
		Model:          "traffic-model",
		InputTokens:    4,
		OutputTokens:   6,
		TotalTokens:    10,
		Success:        false,
		ErrorClass:     &errClass,
		CreatedAt:      now.Add(-90 * time.Second),
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_old_success",
		AttemptNo:      1,
		UserID:         1,
		APIKeyID:       2,
		SourceEndpoint: "/v1/responses",
		Model:          "traffic-model",
		InputTokens:    20,
		OutputTokens:   10,
		TotalTokens:    30,
		Success:        true,
		CreatedAt:      now.Add(-4 * time.Minute),
	})

	opsStore := opserrorlogsmemory.New()
	opsSvc, err := opserrorlogsservice.New(opsStore, func() time.Time { return now })
	if err != nil {
		t.Fatalf("new ops error service: %v", err)
	}
	status := 503
	if err := opsSvc.RecordError(t.Context(), opserrorlogscontract.RecordRequest{
		OccurredAt:     currentAt,
		RequestID:      "req_usage_error",
		AttemptNo:      1,
		SourceEndpoint: "/v1/responses",
		Model:          "traffic-model",
		StatusCode:     &status,
		ErrorClass:     "provider_5xx",
		ErrorMessage:   "duplicate usage-backed failure",
	}); err != nil {
		t.Fatalf("record duplicate ops error: %v", err)
	}
	if err := opsSvc.RecordError(t.Context(), opserrorlogscontract.RecordRequest{
		OccurredAt:     currentAt,
		RequestID:      "req_error_only",
		AttemptNo:      1,
		SourceEndpoint: "/v1/responses",
		Model:          "traffic-model",
		StatusCode:     &status,
		InputTokens:    7,
		OutputTokens:   3,
		ErrorClass:     "timeout",
		ErrorMessage:   "no usage row was recorded",
	}); err != nil {
		t.Fatalf("record error-only ops error: %v", err)
	}

	handler := New(config.Load(), nil, WithUsageStore(usageStore), WithOpsErrorLogsStore(opsStore))
	_, sessionCookie := mustLoginAdmin(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/realtime-traffic?window=5m", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("realtime traffic: got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.OpsRealtimeTrafficResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode realtime traffic response: %v", err)
	}
	data := resp.Data
	if data.TotalRequests != 4 {
		t.Fatalf("total_requests should dedupe usage-backed ops error and include error-only row, got %+v", data)
	}
	if data.ErrorCount != 2 {
		t.Fatalf("error_count should include usage failure and error-only row, got %+v", data)
	}
	if data.RequestsPerMin.Current != 2 || data.RequestsPerMin.Peak != 2 || data.RequestsPerMin.Average != 0 {
		t.Fatalf("unexpected request rates: %+v", data.RequestsPerMin)
	}
	if data.TokensPerMin.Current != 25 || data.TokensPerMin.Peak != 30 || data.TokensPerMin.Average != 13 {
		t.Fatalf("unexpected token rates: %+v", data.TokensPerMin)
	}
	if data.UsageLogCount != 3 || data.OpsErrorLogCount != 2 {
		t.Fatalf("unexpected source counts: %+v", data)
	}
	if data.ErrorRate != 0.5 {
		t.Fatalf("unexpected error rate: %+v", data)
	}
}

func TestAdminOpsRealtimeTrafficRejectsInvalidWindow(t *testing.T) {
	handler := New(config.Load(), nil)
	_, sessionCookie := mustLoginAdmin(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/realtime-traffic?window=2h", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid window 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}
