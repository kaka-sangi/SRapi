package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestAdminErrorLogsDerivedFromFailedUsage proves the admin error-logs endpoints
// surface ONLY the failed usage logs (degraded mode: error logs are derived from
// usage_log rows where success == false, since there is no status_code column).
// A failing upstream produces a failed usage log; a healthy upstream produces a
// successful one. The list must contain just the failed row, the detail handler
// must return it by id, and an unknown id must 404.
func TestAdminErrorLogsDerivedFromFailedUsage(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer healthy.Close()
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer failing.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	// Healthy stack: a successful request -> a success usage log (NOT an error log).
	okProvider := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"ok-provider","display_name":"OK","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	okModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"ok-model","display_name":"OK Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(okModel.Data.Id), `{"provider_id":"`+string(okProvider.Data.Id)+`","upstream_model_name":"ok-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(okProvider.Data.Id)+`","name":"ok-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+healthy.URL+`/v1"},"status":"active"}`)

	// Failing stack: a request that exhausts failover -> a failed usage log (an error log).
	badProvider := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"bad-provider","display_name":"Bad","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	badModel := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"bad-model","display_name":"Bad Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(badModel.Data.Id), `{"provider_id":"`+string(badProvider.Data.Id)+`","upstream_model_name":"bad-up","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(badProvider.Data.Id)+`","name":"bad-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+failing.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"ok-model","messages":[{"role":"user","content":"hi"}]}`)
	// The failing request is expected to return a non-2xx gateway status; we drive
	// it directly (not via mustGatewayRequest, which would fail the test on non-200)
	// solely to record the failed usage log.
	failReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"bad-model","messages":[{"role":"user","content":"hi"}]}`))
	failReq.Header.Set("Authorization", "Bearer "+apiKey)
	failReq.Header.Set("Content-Type", "application/json")
	handler.ServeHTTP(httptest.NewRecorder(), failReq)

	// List: only the failed row should be present.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/error-logs?page=1&page_size=50", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("error-logs list: expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var list apiopenapi.ErrorLogListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode error-logs list: %v", err)
	}
	if len(list.Data) != 1 {
		t.Fatalf("expected exactly 1 derived error log, got %d (%+v)", len(list.Data), list.Data)
	}
	if list.Pagination.Total != 1 {
		t.Fatalf("expected pagination total 1, got %d", list.Pagination.Total)
	}
	errLog := list.Data[0]
	if errLog.Model != "bad-model" {
		t.Fatalf("expected error log for bad-model, got %q", errLog.Model)
	}
	if errLog.ErrorClass == nil || *errLog.ErrorClass == "" {
		t.Fatalf("expected a non-empty error_class on the derived error log, got %v", errLog.ErrorClass)
	}

	// Detail: fetch by id returns the same row in the inline {data, request_id} shape.
	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/error-logs/"+string(errLog.Id), nil)
	detailReq.AddCookie(sessionCookie)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("error-log detail: expected 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detail struct {
		Data      apiopenapi.ErrorLog `json:"data"`
		RequestID string              `json:"request_id"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode error-log detail: %v", err)
	}
	if detail.Data.Id != errLog.Id {
		t.Fatalf("detail id mismatch: got %q want %q", detail.Data.Id, errLog.Id)
	}
	if detail.RequestID == "" {
		t.Fatalf("expected a request_id on the detail response")
	}

	// Unknown id -> 404.
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/error-logs/99999999", nil)
	missingReq.AddCookie(sessionCookie)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("unknown error-log id: expected 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}
}
