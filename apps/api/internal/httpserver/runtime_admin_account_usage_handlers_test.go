package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

// TestAdminAccountUsageStatsEndpoints proves the per-account usage-stats
// endpoints aggregate the account's usage logs. A single successful gateway
// request records exactly one usage log for the routed account; the
// usage-windows, usage-daily and usage-today handlers must each surface that one
// request (with one success, zero errors) scoped to the account, and reject
// requests that are not authenticated as an admin.
func TestAdminAccountUsageStatsEndpoints(t *testing.T) {
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
	}))
	defer healthy.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	csrf := loginResp.Data.CsrfToken

	provider := mustCreateProvider(t, handler, sessionCookie, csrf, `{"name":"usage-provider","display_name":"Usage","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	model := mustCreateModel(t, handler, sessionCookie, csrf, `{"canonical_name":"usage-model","display_name":"Usage Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrf, string(model.Data.Id), `{"provider_id":"`+string(provider.Data.Id)+`","upstream_model_name":"usage-up","status":"active"}`)
	account := mustCreateAccount(t, handler, sessionCookie, csrf, `{"provider_id":"`+string(provider.Data.Id)+`","name":"usage-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"`+healthy.URL+`/v1"},"status":"active"}`)
	accountID := string(account.Data.Id)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, csrf)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"usage-model","messages":[{"role":"user","content":"hi"}]}`)

	// usage-windows: the 5h and 7d windows must each include the one request as a
	// success, scoped to this account.
	windowsRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/"+accountID+"/usage-windows")
	if windowsRec.Code != http.StatusOK {
		t.Fatalf("usage-windows: expected 200, got %d body=%s", windowsRec.Code, windowsRec.Body.String())
	}
	var windows struct {
		Data      apiopenapi.AccountUsageWindowsResult `json:"data"`
		RequestID string                               `json:"request_id"`
	}
	if err := json.NewDecoder(windowsRec.Body).Decode(&windows); err != nil {
		t.Fatalf("decode usage-windows: %v", err)
	}
	if windows.RequestID == "" {
		t.Fatalf("expected a request_id on usage-windows response")
	}
	if string(windows.Data.AccountId) != accountID {
		t.Fatalf("usage-windows account_id mismatch: got %q want %q", windows.Data.AccountId, accountID)
	}
	if len(windows.Data.Windows) != 2 {
		t.Fatalf("expected 2 windows, got %d (%+v)", len(windows.Data.Windows), windows.Data.Windows)
	}
	for _, win := range windows.Data.Windows {
		if win.Requests != 1 {
			t.Fatalf("window %q: expected 1 request, got %d", win.Window, win.Requests)
		}
		if win.SuccessCount != 1 || win.ErrorCount != 0 {
			t.Fatalf("window %q: expected 1 success/0 error, got %d/%d", win.Window, win.SuccessCount, win.ErrorCount)
		}
		if win.InputTokens != 3 || win.OutputTokens != 5 || win.TotalTokens != 8 {
			t.Fatalf("window %q: token mismatch in=%d out=%d total=%d", win.Window, win.InputTokens, win.OutputTokens, win.TotalTokens)
		}
		if win.Currency == "" {
			t.Fatalf("window %q: expected a non-empty currency", win.Window)
		}
	}

	// usage-today: same single success, with success_rate == 1.
	todayRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/"+accountID+"/usage-today")
	if todayRec.Code != http.StatusOK {
		t.Fatalf("usage-today: expected 200, got %d body=%s", todayRec.Code, todayRec.Body.String())
	}
	var today struct {
		Data      apiopenapi.AccountUsageToday `json:"data"`
		RequestID string                       `json:"request_id"`
	}
	if err := json.NewDecoder(todayRec.Body).Decode(&today); err != nil {
		t.Fatalf("decode usage-today: %v", err)
	}
	if today.Data.Requests != 1 || today.Data.SuccessCount != 1 || today.Data.ErrorCount != 0 {
		t.Fatalf("usage-today: expected 1 request/1 success/0 error, got %d/%d/%d", today.Data.Requests, today.Data.SuccessCount, today.Data.ErrorCount)
	}
	if today.Data.SuccessRate != 1 {
		t.Fatalf("usage-today: expected success_rate 1, got %v", today.Data.SuccessRate)
	}
	if today.Data.TotalTokens != 8 {
		t.Fatalf("usage-today: expected 8 total tokens, got %d", today.Data.TotalTokens)
	}

	// usage-daily: default 30-day dense series; today's bucket must hold the one
	// request, and every bucket must carry a 2dp cost string.
	dailyRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/"+accountID+"/usage-daily")
	if dailyRec.Code != http.StatusOK {
		t.Fatalf("usage-daily: expected 200, got %d body=%s", dailyRec.Code, dailyRec.Body.String())
	}
	// data is a bare AccountUsageDailyPoint array per the OpenAPI contract
	// (asserting the array shape here is the guard that the handler can't drift
	// back to an object wrapper, which the frontend cannot iterate).
	var daily struct {
		Data      []apiopenapi.AccountUsageDailyPoint `json:"data"`
		RequestID string                              `json:"request_id"`
	}
	if err := json.NewDecoder(dailyRec.Body).Decode(&daily); err != nil {
		t.Fatalf("decode usage-daily: %v", err)
	}
	if len(daily.Data) != 30 {
		t.Fatalf("usage-daily: expected 30 dense points (default window), got %d", len(daily.Data))
	}
	totalRequests := 0
	for _, point := range daily.Data {
		totalRequests += point.Requests
		if point.Cost == "" {
			t.Fatalf("usage-daily: point %q missing cost", point.Date)
		}
	}
	if totalRequests != 1 {
		t.Fatalf("usage-daily: expected exactly 1 request across the series, got %d", totalRequests)
	}
	last := daily.Data[len(daily.Data)-1]
	if last.Requests != 1 {
		t.Fatalf("usage-daily: expected today's (last) bucket to hold the 1 request, got %d", last.Requests)
	}

	// ?days bound rejection.
	badDaysRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/"+accountID+"/usage-daily?days=0")
	if badDaysRec.Code != http.StatusBadRequest {
		t.Fatalf("usage-daily days=0: expected 400, got %d body=%s", badDaysRec.Code, badDaysRec.Body.String())
	}

	// Unauthenticated (no session cookie) must be forbidden.
	anonReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+accountID+"/usage-today", nil)
	anonRec := httptest.NewRecorder()
	handler.ServeHTTP(anonRec, anonReq)
	if anonRec.Code != http.StatusForbidden {
		t.Fatalf("usage-today without admin session: expected 403, got %d", anonRec.Code)
	}

	// usage-today/batch must match the single-account number for the same id,
	// echo unknown ids as zeroed rows (not omit them — the list view wants a
	// consistent column for every selected row), and surface the requested
	// account_id back on each row so the frontend can join by id.
	batchRec := doAdminGet(t, handler, sessionCookie,
		"/api/v1/admin/accounts/usage-today/batch?account_ids="+accountID+",99999")
	if batchRec.Code != http.StatusOK {
		t.Fatalf("batch usage-today: expected 200, got %d body=%s", batchRec.Code, batchRec.Body.String())
	}
	var batch apiopenapi.BatchAccountUsageTodayResponse
	if err := json.NewDecoder(batchRec.Body).Decode(&batch); err != nil {
		t.Fatalf("decode batch usage-today: %v", err)
	}
	if len(batch.Data) != 2 {
		t.Fatalf("batch usage-today: expected 2 rows (real + unknown zero-row), got %d", len(batch.Data))
	}
	var matched *apiopenapi.AccountUsageTodayWithID
	for i := range batch.Data {
		if string(batch.Data[i].AccountId) == accountID {
			matched = &batch.Data[i]
		}
	}
	if matched == nil {
		t.Fatalf("batch usage-today: expected a row for account %q", accountID)
	}
	if matched.Requests != 1 || matched.SuccessCount != 1 || matched.SuccessRate != 1 {
		t.Fatalf("batch usage-today: row for account %q mismatch: %+v", accountID, matched)
	}
	if matched.TotalTokens != 8 {
		t.Fatalf("batch usage-today: expected 8 total tokens, got %d", matched.TotalTokens)
	}

	emptyRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/usage-today/batch?account_ids=")
	if emptyRec.Code != http.StatusOK {
		t.Fatalf("batch usage-today empty: expected 200, got %d", emptyRec.Code)
	}
	badRec := doAdminGet(t, handler, sessionCookie, "/api/v1/admin/accounts/usage-today/batch?account_ids=foo")
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("batch usage-today bad: expected 400, got %d", badRec.Code)
	}
}

// doAdminGet issues an admin-authenticated GET and returns the recorder.
func doAdminGet(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
