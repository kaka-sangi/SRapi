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
	var daily struct {
		Data struct {
			AccountID string                              `json:"account_id"`
			Days      int                                 `json:"days"`
			Points    []apiopenapi.AccountUsageDailyPoint `json:"points"`
		} `json:"data"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(dailyRec.Body).Decode(&daily); err != nil {
		t.Fatalf("decode usage-daily: %v", err)
	}
	if daily.Data.Days != 30 {
		t.Fatalf("usage-daily: expected default 30 days, got %d", daily.Data.Days)
	}
	if len(daily.Data.Points) != 30 {
		t.Fatalf("usage-daily: expected 30 dense points, got %d", len(daily.Data.Points))
	}
	totalRequests := 0
	for _, point := range daily.Data.Points {
		totalRequests += point.Requests
		if point.Cost == "" {
			t.Fatalf("usage-daily: point %q missing cost", point.Date)
		}
	}
	if totalRequests != 1 {
		t.Fatalf("usage-daily: expected exactly 1 request across the series, got %d", totalRequests)
	}
	last := daily.Data.Points[len(daily.Data.Points)-1]
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
