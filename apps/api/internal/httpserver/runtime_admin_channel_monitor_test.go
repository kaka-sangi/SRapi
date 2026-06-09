package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/config"
)

func channelMonitorDo(t *testing.T, handler http.Handler, method, path, csrf string, cookie *http.Cookie, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func mustCreateChannelMonitorAccount(t *testing.T, handler http.Handler, cookie *http.Cookie, csrf string) (int, int) {
	t.Helper()
	providerResp := mustCreateProvider(t, handler, cookie, csrf, `{"name":"channel-monitor-provider","display_name":"Channel Monitor Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, cookie, csrf, `{"canonical_name":"channel-monitor-model","display_name":"Channel Monitor Model","status":"active"}`)
	mustCreateMapping(t, handler, cookie, csrf, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"channel-monitor-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, cookie, csrf, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"channel-monitor-account","runtime_class":"api_key","credential":{"api_key":"channel-monitor-secret"},"metadata":{"base_url":"https://example.invalid/v1"},"status":"active"}`)
	accountID, err := strconv.Atoi(string(accountResp.Data.Id))
	if err != nil {
		t.Fatalf("parse account id %q: %v", accountResp.Data.Id, err)
	}
	providerID, err := strconv.Atoi(string(providerResp.Data.Id))
	if err != nil {
		t.Fatalf("parse provider id %q: %v", providerResp.Data.Id, err)
	}
	return accountID, providerID
}

func TestChannelMonitorCRUDAndRun(t *testing.T) {
	handler := New(config.Load(), nil)
	login, cookie := mustLoginAdmin(t, handler)
	csrf := login.Data.CsrfToken
	accountID, _ := mustCreateChannelMonitorAccount(t, handler, cookie, csrf)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini"}]}`))
	}))
	defer upstream.Close()

	// Create a monitor scoped to the seed account with a custom request.
	createBody := fmt.Sprintf(`{
		"name":"seed-monitor",
		"scope":"account",
		"scope_ref":"%d",
		"interval_seconds":120,
		"model":"channel-monitor-model",
		"request":{"method":"GET","url":"%s/models","expected_status_codes":[200,401],"response_contains":"data"}
	}`, accountID, upstream.URL)
	createRec := channelMonitorDo(t, handler, http.MethodPost, "/api/v1/admin/channel-monitors", csrf, cookie, createBody)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected monitor create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Data struct {
			ID              int    `json:"id"`
			Name            string `json:"name"`
			Scope           string `json:"scope"`
			IntervalSeconds int    `json:"interval_seconds"`
			Request         struct {
				Method              string `json:"method"`
				ExpectedStatusCodes []int  `json:"expected_status_codes"`
				ResponseContains    string `json:"response_contains"`
			} `json:"request"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Data.ID == 0 || created.Data.Scope != "account" {
		t.Fatalf("unexpected created monitor: %+v", created.Data)
	}
	if created.Data.Request.Method != "GET" || created.Data.Request.ResponseContains != "data" {
		t.Fatalf("custom request not persisted: %+v", created.Data.Request)
	}
	monitorID := created.Data.ID

	// Update the monitor (disable).
	updateRec := channelMonitorDo(t, handler, http.MethodPatch, fmt.Sprintf("/api/v1/admin/channel-monitors/%d", monitorID), csrf, cookie, `{"enabled":false,"interval_seconds":600}`)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected monitor update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	// Run-now: returns per-model CheckResult (probe will fail with no upstream but still records a check).
	runRec := channelMonitorDo(t, handler, http.MethodPost, fmt.Sprintf("/api/v1/admin/channel-monitors/%d/run", monitorID), csrf, cookie, "")
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected monitor run 200, got %d body=%s", runRec.Code, runRec.Body.String())
	}
	var runResp struct {
		Data struct {
			CheckedCount int `json:"checked_count"`
			Results      []struct {
				AccountID int    `json:"account_id"`
				Model     string `json:"model"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(runRec.Body).Decode(&runResp); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if runResp.Data.CheckedCount != 1 || len(runResp.Data.Results) != 1 {
		t.Fatalf("expected one per-model result, got checked=%d results=%d", runResp.Data.CheckedCount, len(runResp.Data.Results))
	}
	if runResp.Data.Results[0].AccountID != accountID || runResp.Data.Results[0].Model != "channel-monitor-model" {
		t.Fatalf("unexpected check result: %+v", runResp.Data.Results[0])
	}

	// Run-history contains the run.
	historyRec := channelMonitorDo(t, handler, http.MethodGet, fmt.Sprintf("/api/v1/admin/channel-monitors/%d/runs", monitorID), "", cookie, "")
	if historyRec.Code != http.StatusOK {
		t.Fatalf("expected runs list 200, got %d body=%s", historyRec.Code, historyRec.Body.String())
	}
	var historyResp struct {
		Data []struct {
			MonitorID int `json:"monitor_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(historyRec.Body).Decode(&historyResp); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(historyResp.Data) != 1 || historyResp.Data[0].MonitorID != monitorID {
		t.Fatalf("expected one run in history, got %+v", historyResp.Data)
	}

	// Delete the monitor.
	delRec := channelMonitorDo(t, handler, http.MethodDelete, fmt.Sprintf("/api/v1/admin/channel-monitors/%d", monitorID), csrf, cookie, "")
	if delRec.Code != http.StatusOK {
		t.Fatalf("expected monitor delete 200, got %d body=%s", delRec.Code, delRec.Body.String())
	}
}

func TestChannelMonitorTemplateApply(t *testing.T) {
	handler := New(config.Load(), nil)
	login, cookie := mustLoginAdmin(t, handler)
	csrf := login.Data.CsrfToken
	accountID, _ := mustCreateChannelMonitorAccount(t, handler, cookie, csrf)

	// Create two monitors.
	makeMonitor := func(name string) int {
		body := fmt.Sprintf(`{"name":"%s","scope":"account","scope_ref":"%d"}`, name, accountID)
		rec := channelMonitorDo(t, handler, http.MethodPost, "/api/v1/admin/channel-monitors", csrf, cookie, body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected monitor create 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				ID int `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode monitor: %v", err)
		}
		return resp.Data.ID
	}
	m1 := makeMonitor("mon-a")
	m2 := makeMonitor("mon-b")

	// Create a template with a distinctive request body.
	tplBody := `{"name":"probe-tpl","description":"reusable","request":{"method":"POST","body":"{\"model\":\"gpt-4o-mini\"}","response_json_path":"data.0.id"}}`
	tplRec := channelMonitorDo(t, handler, http.MethodPost, "/api/v1/admin/channel-monitor-templates", csrf, cookie, tplBody)
	if tplRec.Code != http.StatusCreated {
		t.Fatalf("expected template create 201, got %d body=%s", tplRec.Code, tplRec.Body.String())
	}
	var tpl struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(tplRec.Body).Decode(&tpl); err != nil {
		t.Fatalf("decode template: %v", err)
	}

	// Apply the template to both monitors.
	applyBody := fmt.Sprintf(`{"monitor_ids":[%d,%d]}`, m1, m2)
	applyRec := channelMonitorDo(t, handler, http.MethodPost, fmt.Sprintf("/api/v1/admin/channel-monitor-templates/%d/apply", tpl.Data.ID), csrf, cookie, applyBody)
	if applyRec.Code != http.StatusOK {
		t.Fatalf("expected template apply 200, got %d body=%s", applyRec.Code, applyRec.Body.String())
	}
	var applied struct {
		Data []struct {
			ID      int `json:"id"`
			Request struct {
				Method           string `json:"method"`
				Body             string `json:"body"`
				ResponseJSONPath string `json:"response_json_path"`
			} `json:"request"`
		} `json:"data"`
	}
	if err := json.NewDecoder(applyRec.Body).Decode(&applied); err != nil {
		t.Fatalf("decode apply: %v", err)
	}
	if len(applied.Data) != 2 {
		t.Fatalf("expected two monitors updated, got %d", len(applied.Data))
	}
	for _, def := range applied.Data {
		if def.Request.Method != "POST" || def.Request.ResponseJSONPath != "data.0.id" {
			t.Fatalf("template request not applied to monitor %d: %+v", def.ID, def.Request)
		}
	}
}

func TestChannelMonitorRequiresAdmin(t *testing.T) {
	handler := New(config.Load(), nil)

	// Unauthenticated list is forbidden.
	rec := channelMonitorDo(t, handler, http.MethodGet, "/api/v1/admin/channel-monitors", "", nil, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected unauthenticated list 403, got %d body=%s", rec.Code, rec.Body.String())
	}

	// Create a non-admin user and confirm they are forbidden.
	login, adminCookie := mustLoginAdmin(t, handler)
	userBody := `{"email":"monitor-user@srapi.local","name":"User","password":"password123","roles":["user"]}`
	userRec := channelMonitorDo(t, handler, http.MethodPost, "/api/v1/admin/users", login.Data.CsrfToken, adminCookie, userBody)
	if userRec.Code != http.StatusCreated {
		t.Fatalf("expected user create 201, got %d body=%s", userRec.Code, userRec.Body.String())
	}
	userLogin := channelMonitorDo(t, handler, http.MethodPost, "/api/v1/auth/login", "", nil, `{"email":"monitor-user@srapi.local","password":"password123"}`)
	if userLogin.Code != http.StatusOK {
		t.Fatalf("expected user login 200, got %d", userLogin.Code)
	}
	cookies := userLogin.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected user session cookie")
	}
	forbiddenRec := channelMonitorDo(t, handler, http.MethodGet, "/api/v1/admin/channel-monitors", "", cookies[0], "")
	if forbiddenRec.Code != http.StatusForbidden {
		t.Fatalf("expected non-admin list 403, got %d body=%s", forbiddenRec.Code, forbiddenRec.Body.String())
	}
}
