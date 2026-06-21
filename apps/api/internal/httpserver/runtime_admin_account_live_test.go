package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func TestAdminAccountLiveTestUsesRegisteredMappedModel(t *testing.T) {
	var upstreamModel string
	var upstreamPrompt string
	var gotAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("expected chat completions path, got %s", r.URL.Path)
		}
		gotAuthorization = r.Header.Get("Authorization")
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode live probe payload: %v", err)
		}
		upstreamModel = payload.Model
		if len(payload.Messages) > 0 {
			upstreamPrompt = payload.Messages[0].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"OK"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-live-test-provider","display_name":"Admin Live Test Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"admin-live-test-model","display_name":"Admin Live Test Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"live-upstream-model","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-live-test-account","runtime_class":"api_key","credential":{"api_key":"live-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","supported_models":["live-upstream-model"]},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"live","prompt":"Return SRapi test pong."}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account live test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account test response: %v", err)
	}
	if !resp.Data.Ok || resp.Data.Checks == nil || (*resp.Data.Checks)["live_probe"] != "ok" {
		t.Fatalf("unexpected live test result: %+v", resp.Data)
	}
	if upstreamModel != "live-upstream-model" {
		t.Fatalf("expected mapped upstream model, got %q", upstreamModel)
	}
	if upstreamPrompt != "Return SRapi test pong." {
		t.Fatalf("expected custom live probe prompt, got %q", upstreamPrompt)
	}
	if gotAuthorization != "Bearer live-secret" {
		t.Fatalf("expected live test bearer token, got %q", gotAuthorization)
	}
}

func TestAdminAccountLiveTestRequiresRegisteredMapping(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-live-unmapped-provider","display_name":"Admin Live Unmapped Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-live-unmapped-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"https://upstream.invalid/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"live"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account live test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account test response: %v", err)
	}
	if resp.Data.Ok || resp.Data.Checks == nil || (*resp.Data.Checks)["live_probe"] != "skipped_no_model" {
		t.Fatalf("expected unmapped model failure, got %+v", resp.Data)
	}
}

func TestAdminAccountLiveTestHonorsProviderExcludedModels(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-live-provider-excluded","display_name":"Admin Live Provider Excluded","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"excluded_models":["live-blocked"]}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"live-blocked","display_name":"Live Blocked","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"live-blocked","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-live-provider-excluded-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"https://upstream.invalid/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"live"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account live test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account test response: %v", err)
	}
	if resp.Data.Ok || resp.Data.Checks == nil || (*resp.Data.Checks)["live_probe"] != "skipped_no_model" {
		t.Fatalf("expected provider-excluded model to be skipped, got %+v", resp.Data)
	}
}

func TestAdminAccountLiveTestHonorsProviderSupportedModels(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-live-provider-supported","display_name":"Admin Live Provider Supported","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"supported_models":["live-visible"]}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"live-blocked-by-provider","display_name":"Live Blocked By Provider","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"live-blocked-by-provider","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-live-provider-supported-account","runtime_class":"api_key","credential":{"api_key":"secret"},"metadata":{"base_url":"https://upstream.invalid/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"live"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account live test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account test response: %v", err)
	}
	if resp.Data.Ok || resp.Data.Checks == nil || (*resp.Data.Checks)["live_probe"] != "skipped_no_model" {
		t.Fatalf("expected provider allowlist miss to be skipped, got %+v", resp.Data)
	}
}

func TestAdminAccountTestRejectsUnknownMode(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-test-mode-provider","display_name":"Admin Test Mode Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-test-mode-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"unknown"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid mode 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminAccountLiveTestPersistsCodexQuotaSignals(t *testing.T) {
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("expected codex responses path, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer codex-token" {
			t.Fatalf("expected codex bearer token, got %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamPayload); err != nil {
			t.Fatalf("decode codex live probe payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Codex-Primary-Used-Percent", "42")
		w.Header().Set("X-Codex-Primary-Reset-After-Seconds", "60")
		w.Header().Set("X-Codex-Primary-Window-Minutes", "300")
		w.Header().Set("X-Codex-Secondary-Used-Percent", "25")
		w.Header().Set("X-Codex-Secondary-Reset-After-Seconds", "120")
		w.Header().Set("X-Codex-Secondary-Window-Minutes", "10080")
		_, _ = w.Write([]byte(
			"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
				"data: [DONE]\n\n",
		))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-live-codex-provider","display_name":"Admin Live Codex Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-live-test-model","display_name":"Codex Live Test Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"codex-upstream-model","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-live-codex-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","supported_models":["codex-upstream-model"],"user_agent":"codex-cli/test"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"live","prompt":"Codex custom probe."}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account live test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account test response: %v", err)
	}
	if !resp.Data.Ok || resp.Data.Checks == nil || (*resp.Data.Checks)["quota_signals_persisted"] == nil {
		t.Fatalf("expected quota signals to be persisted, got %+v", resp.Data)
	}
	if upstreamPayload["messages"] != nil {
		t.Fatalf("codex live probe must use responses payload, got chat messages: %+v", upstreamPayload)
	}
	if upstreamPayload["input"] == nil || upstreamPayload["store"] != false || upstreamPayload["stream"] != true {
		t.Fatalf("unexpected codex live probe payload: %+v", upstreamPayload)
	}
	if !strings.Contains(string(mustMarshalJSON(t, upstreamPayload["input"])), "Codex custom probe.") {
		t.Fatalf("expected custom codex prompt in payload, got %+v", upstreamPayload["input"])
	}
	if upstreamPayload["model"] != "codex-upstream-model" {
		t.Fatalf("expected mapped codex upstream model, got %+v", upstreamPayload)
	}

	quotaReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/quota", nil)
	quotaReq.AddCookie(sessionCookie)
	quotaRec := httptest.NewRecorder()
	handler.ServeHTTP(quotaRec, quotaReq)
	if quotaRec.Code != http.StatusOK {
		t.Fatalf("expected account quota 200, got %d body=%s", quotaRec.Code, quotaRec.Body.String())
	}
	var quotaResp apiopenapi.AccountQuotaListResponse
	if err := json.NewDecoder(quotaRec.Body).Decode(&quotaResp); err != nil {
		t.Fatalf("decode account quota: %v", err)
	}
	assertAccountQuotaSnapshot(t, quotaResp.Data, "codex_5h_percent", "42", "58", 0.58)
	assertAccountQuotaSnapshot(t, quotaResp.Data, "codex_7d_percent", "25", "75", 0.75)

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/health-summary", nil)
	healthReq.AddCookie(sessionCookie)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected health summary 200, got %d body=%s", healthRec.Code, healthRec.Body.String())
	}
	var healthResp apiopenapi.AccountHealthSummaryResponse
	if err := json.NewDecoder(healthRec.Body).Decode(&healthResp); err != nil {
		t.Fatalf("decode health summary: %v", err)
	}
	var health *apiopenapi.AccountHealthSnapshot
	for i := range healthResp.Data {
		if healthResp.Data[i].AccountId == accountResp.Data.Id {
			health = &healthResp.Data[i]
			break
		}
	}
	if health == nil {
		t.Fatalf("expected account in health summary, got %+v", healthResp.Data)
	}
	if health.QuotaRemainingRatio != 0.58 {
		t.Fatalf("expected constrained health quota ratio 0.58, got %+v", health.QuotaRemainingRatio)
	}
	if health.QuotaWindows == nil || len(*health.QuotaWindows) != 2 {
		t.Fatalf("expected two quota windows in health summary, got %+v", health.QuotaWindows)
	}
	assertAccountQuotaSnapshot(t, *health.QuotaWindows, "codex_5h_percent", "42", "58", 0.58)
	assertAccountQuotaSnapshot(t, *health.QuotaWindows, "codex_7d_percent", "25", "75", 0.75)
}

func TestAdminAccountLiveTestCodexUsesRegisteredModelWhenBodyModelOmitted(t *testing.T) {
	var upstreamModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/codex/responses" {
			t.Fatalf("expected codex responses path, got %s", r.URL.Path)
		}
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode codex live probe payload: %v", err)
		}
		upstreamModel = payload.Model
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(
			"data: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"OK\"}]}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n" +
				"data: [DONE]\n\n",
		))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-live-codex-default-provider","display_name":"Admin Live Codex Default Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"codex-auto-test-model","display_name":"Codex Auto Test Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"openai/gpt5.4mini-openai-compact","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-live-codex-default-account","runtime_class":"cli_client_token","upstream_client":"codex_cli","credential":{"cli_client_token":"codex-token"},"metadata":{"base_url":"`+upstream.URL+`/backend-api/codex","supported_models":["gpt-5.4-mini"],"user_agent":"codex-cli/test"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", strings.NewReader(`{"mode":"live"}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected account live test 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account test response: %v", err)
	}
	if !resp.Data.Ok || upstreamModel != "gpt-5.4-mini" {
		t.Fatalf("expected normalized registered codex upstream model, got result=%+v model=%q", resp.Data, upstreamModel)
	}
}

func TestAdminAccountQuotaFetchRefreshesOAuthCredential(t *testing.T) {
	var refreshForm url.Values
	var quotaAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse refresh form: %v", err)
			}
			refreshForm = cloneURLValues(r.PostForm)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"fresh-quota-access","refresh_token":"fresh-quota-refresh","expires_in":3600}`))
		case "/quota":
			quotaAuthorization = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Codex-Primary-Used-Percent", "10")
			w.Header().Set("X-Codex-Primary-Reset-After-Seconds", "60")
			w.Header().Set("X-Codex-Primary-Window-Minutes", "300")
			_, _ = w.Write([]byte(`{"account_plan":{"account_plan_id":"plus","subscription_plan":{"allowance":"90","usage":"10","limit":"100","currency":"credits"}}}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-quota-refresh-provider","display_name":"Admin Quota Refresh Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","config_schema":{"quota_url":"`+upstream.URL+`/quota","quota_plan_path":"account_plan.account_plan_id","quota_credits_remaining_path":"account_plan.subscription_plan.allowance","quota_credits_used_path":"account_plan.subscription_plan.usage","quota_credits_limit_path":"account_plan.subscription_plan.limit","quota_currency_path":"account_plan.subscription_plan.currency","auth_mode":"bearer"}}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-quota-refresh-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"expired-quota-access","refresh_token":"old-quota-refresh","expires_at":"2000-01-01T00:00:00Z"},"metadata":{"oauth_token_url":"`+upstream.URL+`/oauth/token","user_agent":"codex-cli/test"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/quota-fetch", nil)
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected quota fetch 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if refreshForm.Get("grant_type") != "refresh_token" || refreshForm.Get("refresh_token") != "old-quota-refresh" {
		t.Fatalf("unexpected refresh form: %v", refreshForm)
	}
	if quotaAuthorization != "Bearer fresh-quota-access" {
		t.Fatalf("expected quota fetch to use refreshed access token, got %q", quotaAuthorization)
	}
	var resp apiopenapi.AccountQuotaReportResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode quota response: %v", err)
	}
	if !resp.Data.Supported || resp.Data.CreditsRemaining != "90" || resp.Data.CreditsLimit != "100" {
		t.Fatalf("unexpected quota report: %+v", resp.Data)
	}
	if resp.Data.StatusCode != http.StatusOK || len(resp.Data.QuotaSignals) != 1 {
		t.Fatalf("expected openapi quota status and signal, got %+v", resp.Data)
	}
	signal := resp.Data.QuotaSignals[0]
	if signal.QuotaType != "codex_5h_percent" || signal.Used != "10" || signal.Remaining != "90" || signal.QuotaLimit != "100" || signal.RemainingRatio != 0.9 || signal.ResetAt == nil {
		t.Fatalf("unexpected quota signal: %+v", signal)
	}
}

func TestAdminAccountAvailabilityUsesOpenAPIWireTypes(t *testing.T) {
	handler, srv := newWithServer(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"admin-availability-provider","display_name":"Admin Availability Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"admin-availability-account","runtime_class":"api_key","credential":{"api_key":"availability-secret"},"status":"active"}`)
	accountID := mustAtoi(t, string(accountResp.Data.Id))
	providerID := mustAtoi(t, string(providerResp.Data.Id))
	snapshotAt := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)

	for _, snapshot := range []accountcontract.AccountHealthSnapshot{
		{
			AccountID:    accountID,
			ProviderID:   providerID,
			Status:       "healthy",
			SuccessRate:  1,
			ErrorRate:    0,
			CircuitState: "closed",
			SnapshotAt:   snapshotAt,
		},
		{
			AccountID:    accountID,
			ProviderID:   providerID,
			Status:       "unhealthy",
			SuccessRate:  0,
			ErrorRate:    1,
			CircuitState: "open",
			SnapshotAt:   snapshotAt.Add(time.Hour),
		},
	} {
		if _, err := srv.runtime.accounts.RecordHealthSnapshot(t.Context(), snapshot); err != nil {
			t.Fatalf("seed health snapshot: %v", err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/availability?days=1", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected availability 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AccountAvailabilityResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode availability response: %v", err)
	}
	if resp.Data.AccountId != int64(accountID) || resp.Data.WindowDays != 1 || resp.Data.OverallUptime != 0.5 {
		t.Fatalf("unexpected availability summary: %+v", resp.Data)
	}
	if len(resp.Data.DailyAvailability) != 1 {
		t.Fatalf("expected one daily rollup, got %+v", resp.Data.DailyAvailability)
	}
	rollup := resp.Data.DailyAvailability[0]
	if rollup.ProviderId != int64(providerID) || rollup.TotalSamples != 2 || rollup.HealthySamples != 1 || rollup.AvailabilityRatio != 0.5 || rollup.AvgSuccessRate != 0.5 {
		t.Fatalf("unexpected availability rollup: %+v", rollup)
	}
}

func assertAccountQuotaSnapshot(t *testing.T, snapshots []apiopenapi.AccountQuotaSnapshot, quotaType string, used string, remaining string, remainingRatio float32) {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.QuotaType != quotaType {
			continue
		}
		if snapshot.Used != used || snapshot.Remaining != remaining || snapshot.QuotaLimit != "100" || snapshot.RemainingRatio != remainingRatio {
			t.Fatalf("unexpected %s quota snapshot: %+v", quotaType, snapshot)
		}
		return
	}
	t.Fatalf("missing %s quota snapshot in %+v", quotaType, snapshots)
}

func mustAtoi(t *testing.T, value string) int {
	t.Helper()
	parsed, err := strconv.Atoi(value)
	if err != nil {
		t.Fatalf("parse int %q: %v", value, err)
	}
	return parsed
}
