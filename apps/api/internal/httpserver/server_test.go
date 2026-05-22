package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	auditcontract "github.com/srapi/srapi/apps/api/internal/modules/audit/contract"
	auditmemory "github.com/srapi/srapi/apps/api/internal/modules/audit/store/memory"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

type upstreamMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func TestHealthIncludesRequestIDAndDependencies(t *testing.T) {
	handler := New(config.Load(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	request.Header.Set("X-Request-ID", "req_test")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if got := response.Header().Get("X-Request-ID"); got != "req_test" {
		t.Fatalf("expected request id header to be preserved, got %q", got)
	}

	var body apiopenapi.HealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.RequestId != "req_test" {
		t.Fatalf("expected body request id req_test, got %q", body.RequestId)
	}
	if body.Data.Dependencies.Database == "" {
		t.Fatal("expected database dependency status")
	}
	if body.Data.Dependencies.Redis == "" {
		t.Fatal("expected redis dependency status")
	}
}

func TestLivezDoesNotRequireDependencies(t *testing.T) {
	handler := New(config.Load(), nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected generated request id")
	}
}

func TestGatewayMaxBodySizeRejectsOversizedJSON(t *testing.T) {
	cfg := config.Load()
	cfg.Gateway.MaxBodySize = int64(len(`{"email":"a","password":"b"}`) - 1)
	handler := New(cfg, nil)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a","password":"b"}`))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized request, got %d body=%s", response.Code, response.Body.String())
	}
}

func TestReadyzFailsWhenDependenciesAreUnavailable(t *testing.T) {
	cfg := config.Load()
	cfg.Database.Host = "127.0.0.1"
	cfg.Database.Port = 1
	cfg.Redis.Host = "127.0.0.1"
	cfg.Redis.Port = 1
	handler := New(cfg, nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}
}

func TestReadyzUsesInjectedDependencyProbes(t *testing.T) {
	handler := New(config.Load(), nil,
		WithDatabasePinger(stubDependencyPinger{err: nil}),
		WithRedisPinger(stubDependencyPinger{err: errors.New("redis unavailable")}),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", response.Code)
	}

	var body apiopenapi.HealthResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Data.Dependencies.Database != apiopenapi.HealthDependencyStatusOk {
		t.Fatalf("expected database dependency ok, got %s", body.Data.Dependencies.Database)
	}
	if body.Data.Dependencies.Redis != apiopenapi.HealthDependencyStatusUnavailable {
		t.Fatalf("expected redis dependency unavailable, got %s", body.Data.Dependencies.Redis)
	}
}

func TestMetricsExposeBaselineSRapiSignals(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"metrics smoke"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected gateway success 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d", metricsRec.Code)
	}
	body := metricsRec.Body.String()
	for _, metric := range []string{
		"srapi_gateway_requests_total",
		"srapi_gateway_request_duration_seconds_count",
		"srapi_gateway_request_duration_seconds_sum",
		"srapi_gateway_inflight_requests",
		"srapi_gateway_errors_total",
		"srapi_scheduler_decisions_total",
		"srapi_provider_errors_total",
		"srapi_usage_tokens_total",
		"srapi_reverse_proxy_ban_signals_total",
	} {
		if !strings.Contains(body, metric) {
			t.Fatalf("expected metrics body to contain %s, got:\n%s", metric, body)
		}
	}
	if !strings.Contains(body, `srapi_gateway_requests_total{endpoint_family="chat_completions",model="gpt-4o-mini",provider_protocol="openai-compatible",result="success"} 1`) {
		t.Fatalf("expected gateway request metric, got:\n%s", body)
	}
	if !strings.Contains(body, `srapi_usage_tokens_total{model="gpt-4o-mini",provider_protocol="openai-compatible",token_kind="input"}`) {
		t.Fatalf("expected usage token metric, got:\n%s", body)
	}
}

func TestAuthAndGatewayFlow(t *testing.T) {
	handler := New(config.Load(), nil)

	loginBody := `{"email":"admin@srapi.local","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected 200 login, got %d", loginRec.Code)
	}

	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if loginResp.Data.CsrfToken == "" {
		t.Fatal("expected csrf token in login response")
	}
	if len(loginRec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
	sessionCookie := loginRec.Result().Cookies()[0]
	if sessionCookie.Name != sessionCookieName || !sessionCookie.HttpOnly || sessionCookie.Path != "/" || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.Secure {
		t.Fatalf("unexpected local session cookie attributes: %+v", sessionCookie)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	meReq.AddCookie(sessionCookie)
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected 200 me, got %d", meRec.Code)
	}

	var meResp apiopenapi.UserResponse
	if err := json.NewDecoder(meRec.Body).Decode(&meResp); err != nil {
		t.Fatalf("decode me response: %v", err)
	}
	if meResp.Data.Email != "admin@srapi.local" {
		t.Fatalf("expected admin email, got %s", meResp.Data.Email)
	}

	createBody := `{"name":"default","scopes":["gateway:invoke"]}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(sessionCookie)
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 create api key, got %d", createRec.Code)
	}

	var createResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}
	if createResp.Data.PlaintextKey == "" {
		t.Fatal("expected plaintext key in create response")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 api keys list, got %d", listRec.Code)
	}
	var listResp apiopenapi.ApiKeyListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode api keys list: %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatal("expected at least one api key")
	}
	if body := listRec.Body.String(); strings.Contains(body, createResp.Data.PlaintextKey) || strings.Contains(body, "plaintext_key") || strings.Contains(body, "hmac-sha256") || strings.Contains(body, `"hash"`) {
		t.Fatalf("api key list leaked secret material: %s", body)
	}
	keyID := createResp.Data.ApiKey.Id
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+string(keyID), strings.NewReader(`{"name":"renamed","status":"disabled","allowed_models":["gpt-4o-mini"],"group_ids":["2","3"]}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.AddCookie(sessionCookie)
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200 update api key, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	if strings.Contains(updateRec.Body.String(), "plaintext_key") {
		t.Fatalf("update response must not include plaintext key: %s", updateRec.Body.String())
	}
	var updateResp apiopenapi.ApiKeyResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode api key update response: %v", err)
	}
	if updateResp.Data.Name != "renamed" || updateResp.Data.Status != apiopenapi.ApiKeyStatusDisabled {
		t.Fatalf("unexpected updated api key: %+v", updateResp.Data)
	}
	if len(updateResp.Data.GroupIds) != 2 || updateResp.Data.GroupIds[0] != "2" || updateResp.Data.GroupIds[1] != "3" {
		t.Fatalf("unexpected updated group ids: %+v", updateResp.Data.GroupIds)
	}

	modelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+createResp.Data.PlaintextKey)
	modelsRec := httptest.NewRecorder()
	handler.ServeHTTP(modelsRec, modelsReq)
	if modelsRec.Code != http.StatusForbidden {
		t.Fatalf("expected disabled api key to be rejected with 403, got %d", modelsRec.Code)
	}

	enableReq := httptest.NewRequest(http.MethodPatch, "/api/v1/api-keys/"+string(keyID), strings.NewReader(`{"status":"active"}`))
	enableReq.Header.Set("Content-Type", "application/json")
	enableReq.AddCookie(sessionCookie)
	enableReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	enableRec := httptest.NewRecorder()
	handler.ServeHTTP(enableRec, enableReq)
	if enableRec.Code != http.StatusOK {
		t.Fatalf("expected 200 enable api key, got %d body=%s", enableRec.Code, enableRec.Body.String())
	}

	modelsReq = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+createResp.Data.PlaintextKey)
	modelsRec = httptest.NewRecorder()
	handler.ServeHTTP(modelsRec, modelsReq)
	if modelsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 models, got %d", modelsRec.Code)
	}

	var modelsResp apiopenapi.OpenAIModelList
	if err := json.NewDecoder(modelsRec.Body).Decode(&modelsResp); err != nil {
		t.Fatalf("decode models response: %v", err)
	}
	if len(modelsResp.Data) == 0 {
		t.Fatal("expected seeded model list")
	}

	expiredReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"expired","scopes":["gateway:invoke"],"expires_at":"2000-01-01T00:00:00Z"}`))
	expiredReq.Header.Set("Content-Type", "application/json")
	expiredReq.AddCookie(sessionCookie)
	expiredReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	expiredRec := httptest.NewRecorder()
	handler.ServeHTTP(expiredRec, expiredReq)
	if expiredRec.Code != http.StatusCreated {
		t.Fatalf("expected expired api key create 201, got %d body=%s", expiredRec.Code, expiredRec.Body.String())
	}
	var expiredResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(expiredRec.Body).Decode(&expiredResp); err != nil {
		t.Fatalf("decode expired api key response: %v", err)
	}
	expiredModelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	expiredModelsReq.Header.Set("Authorization", "Bearer "+expiredResp.Data.PlaintextKey)
	expiredModelsRec := httptest.NewRecorder()
	handler.ServeHTTP(expiredModelsRec, expiredModelsReq)
	if expiredModelsRec.Code != http.StatusForbidden {
		t.Fatalf("expected expired api key to be rejected with 403, got %d body=%s", expiredModelsRec.Code, expiredModelsRec.Body.String())
	}
	if !strings.Contains(expiredModelsRec.Body.String(), "api_key_disabled") {
		t.Fatalf("expected stable disabled-or-expired error code, got %s", expiredModelsRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	createAudit := mustFindAuditLog(t, auditResp.Data, "api_key.create")
	if _, ok := createAudit.After["hash"]; ok {
		t.Fatalf("api key create audit must not expose hash: %+v", createAudit.After)
	}
	if _, ok := createAudit.After["plaintext_key"]; ok {
		t.Fatalf("api key create audit must not expose plaintext key: %+v", createAudit.After)
	}
	updateAudit := mustFindAuditLog(t, auditResp.Data, "api_key.update")
	if updateAudit.Before["name"] != "default" || updateAudit.After["name"] != "renamed" {
		t.Fatalf("expected api key audit before/after name change, got before=%+v after=%+v", updateAudit.Before, updateAudit.After)
	}
	if _, ok := updateAudit.After["hash"]; ok {
		t.Fatalf("api key audit must not expose hash: %+v", updateAudit.After)
	}
}

func TestConsoleWriteRoutesRequireCSRF(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"csrf-provider","display_name":"CSRF Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"csrf-model","display_name":"CSRF Model","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"csrf-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"csrf-group","description":"CSRF Group","status":"active"}`)
	apiKeyResp, _ := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	writeRoutes := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/auth/logout", `{}`},
		{http.MethodPost, "/api/v1/api-keys", `{"name":"blocked"}`},
		{http.MethodPatch, "/api/v1/api-keys/" + string(apiKeyResp.Data.ApiKey.Id), `{"name":"blocked"}`},
		{http.MethodPost, "/api/v1/admin/providers", `{"name":"blocked","display_name":"Blocked","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`},
		{http.MethodPatch, "/api/v1/admin/providers/" + string(providerResp.Data.Id), `{"display_name":"Blocked"}`},
		{http.MethodPost, "/api/v1/admin/providers/" + string(providerResp.Data.Id) + "/test", `{}`},
		{http.MethodPost, "/api/v1/admin/models", `{"canonical_name":"blocked-model","display_name":"Blocked Model","status":"active"}`},
		{http.MethodPatch, "/api/v1/admin/models/" + string(modelResp.Data.Id), `{"display_name":"Blocked Model"}`},
		{http.MethodPost, "/api/v1/admin/models/" + string(modelResp.Data.Id) + "/aliases", `{"alias":"blocked-alias","status":"active"}`},
		{http.MethodPost, "/api/v1/admin/models/" + string(modelResp.Data.Id) + "/mappings", `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"blocked","status":"active"}`},
		{http.MethodPost, "/api/v1/admin/accounts", `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"blocked-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`},
		{http.MethodPost, "/api/v1/admin/accounts/import", `{"accounts":[{"provider_id":"` + string(providerResp.Data.Id) + `","name":"blocked-import","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}]}`},
		{http.MethodPatch, "/api/v1/admin/accounts/" + string(accountResp.Data.Id), `{"name":"blocked-account"}`},
		{http.MethodPatch, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/proxy", `{"proxy_id":"proxy-blocked"}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/test", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/disable", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/enable", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/recover", `{}`},
		{http.MethodPost, "/api/v1/admin/account-groups", `{"name":"blocked-group","status":"active"}`},
		{http.MethodPatch, "/api/v1/admin/account-groups/" + string(groupResp.Data.Id), `{"description":"blocked"}`},
		{http.MethodPost, "/api/v1/admin/account-groups/" + string(groupResp.Data.Id) + "/accounts/" + string(accountResp.Data.Id), `{}`},
		{http.MethodDelete, "/api/v1/admin/account-groups/" + string(groupResp.Data.Id) + "/accounts/" + string(accountResp.Data.Id), `{}`},
		{http.MethodPost, "/api/v1/admin/subscription-plans", `{"name":"blocked-plan","price":"1.00","currency":"USD","validity_days":30}`},
		{http.MethodPost, "/api/v1/admin/user-subscriptions", `{"user_id":"` + string(loginResp.Data.User.Id) + `","plan_id":"1"}`},
		{http.MethodPost, "/api/v1/admin/pricing-rules", `{"model_id":"` + string(modelResp.Data.Id) + `","provider_id":"` + string(providerResp.Data.Id) + `","input_price_per_million_tokens":"1","output_price_per_million_tokens":"2","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"USD"}`},
		{http.MethodPost, "/api/v1/admin/ops/slo", `{"name":"blocked-slo","objective":99}`},
		{http.MethodPatch, "/api/v1/admin/ops/slo/1", `{"status":"disabled"}`},
		{http.MethodPost, "/api/v1/admin/ops/alerts/1/ack", `{}`},
	}

	for _, route := range writeRoutes {
		req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s without csrf: expected 403, got %d body=%s", route.method, route.path, rec.Code, rec.Body.String())
		}
	}
}

func TestAdminOpsSLOAndAlertControlPlane(t *testing.T) {
	usageStore := usagememory.New()
	operationsStore := operationsmemory.NewWithUsageStore(usageStore)
	auditStore := auditmemory.New()
	handler := New(config.Load(), nil,
		WithUsageStore(usageStore),
		WithOperationsStore(operationsStore),
		WithAuditStore(auditStore),
	)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	ctx := t.Context()
	now := time.Now().UTC().Add(-time.Minute)
	clientError := "invalid_request"
	providerError := "rate_limit"
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_ops_good",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "ops-model",
		Success:        true,
		CreatedAt:      now,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_ops_bad",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "ops-model",
		Success:        false,
		ErrorClass:     &providerError,
		CreatedAt:      now,
	})
	seedUsageLog(t, usageStore, usagecontract.UsageLog{
		RequestID:      "req_ops_client_bad",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "ops-model",
		Success:        false,
		ErrorClass:     &clientError,
		CreatedAt:      now,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ops/slo", strings.NewReader(`{"name":"Gateway availability","sli_type":"availability","objective":99,"window_days":28,"filter":{"source_endpoint":"/v1/chat/completions","model":"ops-model","error_owner_exclude":["client"]},"alert_policy":{"name":"multi_window_burn_rate","thresholds":[{"severity":"critical","short_window_seconds":300,"long_window_seconds":3600,"burn_rate":14,"min_request_count":2}]}}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createReq.AddCookie(sessionCookie)
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected slo create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp apiopenapi.OpsSLODefinitionResponse
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode slo create: %v", err)
	}
	if createResp.Data.Objective < 0.989 || createResp.Data.Objective > 0.991 {
		t.Fatalf("expected percent objective normalized to ratio, got %+v", createResp.Data)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/slo", nil)
	listReq.AddCookie(sessionCookie)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected slo list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp apiopenapi.OpsSLOListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode slo list: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].Evaluation.TotalRequests != 2 || listResp.Data[0].Evaluation.BadRequests != 1 {
		t.Fatalf("expected evaluated slo to count provider-owned failure only, got %+v", listResp.Data)
	}

	updateReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/ops/slo/"+string(createResp.Data.Id), strings.NewReader(`{"status":"disabled","objective":0.995}`))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateReq.AddCookie(sessionCookie)
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected slo update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updateResp apiopenapi.OpsSLODefinitionResponse
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode slo update: %v", err)
	}
	if updateResp.Data.Status != apiopenapi.OpsSLOStatusDisabled || updateResp.Data.Objective < 0.994 || updateResp.Data.Objective > 0.996 {
		t.Fatalf("unexpected slo update response: %+v", updateResp.Data)
	}

	sloID, err := strconv.Atoi(string(createResp.Data.Id))
	if err != nil {
		t.Fatalf("parse slo id: %v", err)
	}
	alert, err := operationsStore.CreateAlert(ctx, operationscontract.AlertEvent{
		SLOID:       &sloID,
		RuleID:      "slo.gateway.availability",
		Severity:    operationscontract.AlertSeverityCritical,
		Status:      operationscontract.AlertStatusFiring,
		Fingerprint: "slo:gateway:availability",
		Summary:     "Gateway availability burn rate high",
		Details:     map[string]any{"burn_rate": 50, "sample": "not-copied"},
		StartedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("seed alert: %v", err)
	}

	alertsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/alerts?status=firing&severity=critical", nil)
	alertsReq.AddCookie(sessionCookie)
	alertsRec := httptest.NewRecorder()
	handler.ServeHTTP(alertsRec, alertsReq)
	if alertsRec.Code != http.StatusOK {
		t.Fatalf("expected alert list 200, got %d body=%s", alertsRec.Code, alertsRec.Body.String())
	}
	var alertsResp apiopenapi.OpsAlertListResponse
	if err := json.NewDecoder(alertsRec.Body).Decode(&alertsResp); err != nil {
		t.Fatalf("decode alert list: %v", err)
	}
	if len(alertsResp.Data) != 1 || alertsResp.Data[0].Status != apiopenapi.Firing {
		t.Fatalf("expected firing critical alert, got %+v", alertsResp.Data)
	}

	ackReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/ops/alerts/"+strconv.Itoa(alert.ID)+"/ack", nil)
	ackReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	ackReq.AddCookie(sessionCookie)
	ackRec := httptest.NewRecorder()
	handler.ServeHTTP(ackRec, ackReq)
	if ackRec.Code != http.StatusOK {
		t.Fatalf("expected alert ack 200, got %d body=%s", ackRec.Code, ackRec.Body.String())
	}
	var ackResp apiopenapi.OpsAlertResponse
	if err := json.NewDecoder(ackRec.Body).Decode(&ackResp); err != nil {
		t.Fatalf("decode alert ack: %v", err)
	}
	if ackResp.Data.Status != apiopenapi.Acknowledged || ackResp.Data.AcknowledgedBy == nil {
		t.Fatalf("expected acknowledged alert response, got %+v", ackResp.Data)
	}

	auditLogs, err := auditStore.List(ctx)
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if !auditContractLogHasAction(auditLogs, "ops_slo.create") || !auditContractLogHasAction(auditLogs, "ops_slo.update") || !auditContractLogHasAction(auditLogs, "ops_alert.ack") {
		t.Fatalf("expected ops audit actions, got %+v", auditLogs)
	}
	for _, item := range auditLogs {
		if item.Action == "ops_alert.ack" {
			if _, ok := item.After["details"]; ok {
				t.Fatalf("alert ack audit must not expose alert details: %+v", item.After)
			}
		}
	}
}

func TestAdminSubscriptionPricingControlPlane(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"subscription-provider","display_name":"Subscription Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"subscription-model","display_name":"Subscription Model","status":"active"}`)

	planReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/subscription-plans", strings.NewReader(`{"name":"commercial-pro","description":"Commercial Pro","price":"19.99","currency":"usd","validity_days":30,"entitlements":{"allowed_models":["subscription-model"],"monthly_token_quota":1000,"scheduler_strategy":"cost_saver"},"for_sale":true,"sort_order":5,"status":"active"}`))
	planReq.Header.Set("Content-Type", "application/json")
	planReq.AddCookie(sessionCookie)
	planReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	planRec := httptest.NewRecorder()
	handler.ServeHTTP(planRec, planReq)
	if planRec.Code != http.StatusCreated {
		t.Fatalf("expected subscription plan create 201, got %d body=%s", planRec.Code, planRec.Body.String())
	}
	var planResp apiopenapi.SubscriptionPlanResponse
	if err := json.NewDecoder(planRec.Body).Decode(&planResp); err != nil {
		t.Fatalf("decode subscription plan: %v", err)
	}
	if planResp.Data.Price != "19.99000000" || planResp.Data.Currency != "USD" || planResp.Data.Status != apiopenapi.SubscriptionPlanStatusActive {
		t.Fatalf("unexpected subscription plan response: %+v", planResp.Data)
	}
	if planResp.Data.Entitlements["scheduler_strategy"] != "cost_saver" {
		t.Fatalf("expected entitlement snapshot in plan response, got %+v", planResp.Data.Entitlements)
	}

	userSubReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/user-subscriptions", strings.NewReader(`{"user_id":"`+string(loginResp.Data.User.Id)+`","plan_id":"`+string(planResp.Data.Id)+`","source_type":"manual","source_id":"admin-seed"}`))
	userSubReq.Header.Set("Content-Type", "application/json")
	userSubReq.AddCookie(sessionCookie)
	userSubReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	userSubRec := httptest.NewRecorder()
	handler.ServeHTTP(userSubRec, userSubReq)
	if userSubRec.Code != http.StatusCreated {
		t.Fatalf("expected user subscription create 201, got %d body=%s", userSubRec.Code, userSubRec.Body.String())
	}
	var userSubResp apiopenapi.UserSubscriptionResponse
	if err := json.NewDecoder(userSubRec.Body).Decode(&userSubResp); err != nil {
		t.Fatalf("decode user subscription: %v", err)
	}
	if userSubResp.Data.UserId != loginResp.Data.User.Id || userSubResp.Data.PlanId != planResp.Data.Id || userSubResp.Data.Status != apiopenapi.Active {
		t.Fatalf("unexpected user subscription response: %+v", userSubResp.Data)
	}
	if userSubResp.Data.EntitlementsSnapshot["scheduler_strategy"] != "cost_saver" {
		t.Fatalf("expected entitlement snapshot copied to subscription, got %+v", userSubResp.Data.EntitlementsSnapshot)
	}

	pricingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules", strings.NewReader(`{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"`+string(providerResp.Data.Id)+`","input_price_per_million_tokens":"1.25","output_price_per_million_tokens":"2.50","cache_read_price_per_million_tokens":"0.10","cache_write_price_per_million_tokens":"0.20","currency":"usd"}`))
	pricingReq.Header.Set("Content-Type", "application/json")
	pricingReq.AddCookie(sessionCookie)
	pricingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	pricingRec := httptest.NewRecorder()
	handler.ServeHTTP(pricingRec, pricingReq)
	if pricingRec.Code != http.StatusCreated {
		t.Fatalf("expected pricing rule create 201, got %d body=%s", pricingRec.Code, pricingRec.Body.String())
	}
	var pricingResp apiopenapi.PricingRuleResponse
	if err := json.NewDecoder(pricingRec.Body).Decode(&pricingResp); err != nil {
		t.Fatalf("decode pricing rule: %v", err)
	}
	if pricingResp.Data.InputPricePerMillionTokens != "1.25000000" || pricingResp.Data.OutputPricePerMillionTokens != "2.50000000" || pricingResp.Data.Currency != "USD" {
		t.Fatalf("expected normalized decimal pricing rule, got %+v", pricingResp.Data)
	}

	plansReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/subscription-plans", nil)
	plansReq.AddCookie(sessionCookie)
	plansRec := httptest.NewRecorder()
	handler.ServeHTTP(plansRec, plansReq)
	if plansRec.Code != http.StatusOK {
		t.Fatalf("expected subscription plan list 200, got %d body=%s", plansRec.Code, plansRec.Body.String())
	}
	var plansResp apiopenapi.SubscriptionPlanListResponse
	if err := json.NewDecoder(plansRec.Body).Decode(&plansResp); err != nil {
		t.Fatalf("decode subscription plan list: %v", err)
	}
	if len(plansResp.Data) != 1 || plansResp.Data[0].Id != planResp.Data.Id {
		t.Fatalf("unexpected subscription plan list: %+v", plansResp.Data)
	}

	adminSubsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/user-subscriptions?user_id="+string(loginResp.Data.User.Id), nil)
	adminSubsReq.AddCookie(sessionCookie)
	adminSubsRec := httptest.NewRecorder()
	handler.ServeHTTP(adminSubsRec, adminSubsReq)
	if adminSubsRec.Code != http.StatusOK {
		t.Fatalf("expected admin user subscription list 200, got %d body=%s", adminSubsRec.Code, adminSubsRec.Body.String())
	}
	var adminSubsResp apiopenapi.UserSubscriptionListResponse
	if err := json.NewDecoder(adminSubsRec.Body).Decode(&adminSubsResp); err != nil {
		t.Fatalf("decode admin user subscription list: %v", err)
	}
	if len(adminSubsResp.Data) != 1 || adminSubsResp.Data[0].Id != userSubResp.Data.Id {
		t.Fatalf("unexpected admin user subscription list: %+v", adminSubsResp.Data)
	}

	meSubsReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/subscriptions", nil)
	meSubsReq.AddCookie(sessionCookie)
	meSubsRec := httptest.NewRecorder()
	handler.ServeHTTP(meSubsRec, meSubsReq)
	if meSubsRec.Code != http.StatusOK {
		t.Fatalf("expected current user subscription list 200, got %d body=%s", meSubsRec.Code, meSubsRec.Body.String())
	}
	var meSubsResp apiopenapi.UserSubscriptionListResponse
	if err := json.NewDecoder(meSubsRec.Body).Decode(&meSubsResp); err != nil {
		t.Fatalf("decode current user subscription list: %v", err)
	}
	if len(meSubsResp.Data) != 1 || meSubsResp.Data[0].Id != userSubResp.Data.Id {
		t.Fatalf("unexpected current user subscription list: %+v", meSubsResp.Data)
	}

	rulesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing-rules", nil)
	rulesReq.AddCookie(sessionCookie)
	rulesRec := httptest.NewRecorder()
	handler.ServeHTTP(rulesRec, rulesReq)
	if rulesRec.Code != http.StatusOK {
		t.Fatalf("expected pricing rule list 200, got %d body=%s", rulesRec.Code, rulesRec.Body.String())
	}
	var rulesResp apiopenapi.PricingRuleListResponse
	if err := json.NewDecoder(rulesRec.Body).Decode(&rulesResp); err != nil {
		t.Fatalf("decode pricing rule list: %v", err)
	}
	if len(rulesResp.Data) != 1 || rulesResp.Data[0].Id != pricingResp.Data.Id {
		t.Fatalf("unexpected pricing rule list: %+v", rulesResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit log list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"subscription_plan.create", "user_subscription.create", "pricing_rule.create"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
}

func TestAdminAccountInspectAndExportStaySecretSafe(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-ops-provider","display_name":"Account Ops Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"account-ops-account","runtime_class":"api_key","credential":{"api_key":"account-ops-secret"},"metadata":{"base_url":"https://upstream.example/v1","access_token":"metadata-access-token","cookie":"metadata-cookie","nested":{"refresh_token":"nested-refresh-token","safe":"kept"},"labels":[{"api_key":"nested-api-key","region":"us"}]},"status":"active"}`)
	groupResp := mustCreateAccountGroup(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"account-ops-group","description":"ops","provider_scope":{"provider_id":"`+string(providerResp.Data.Id)+`"},"model_scope":{"family":"claude"},"strategy_hint":"balanced","status":"active"}`)

	addMemberReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-groups/"+string(groupResp.Data.Id)+"/accounts/"+string(accountResp.Data.Id), nil)
	addMemberReq.AddCookie(sessionCookie)
	addMemberReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	addMemberRec := httptest.NewRecorder()
	handler.ServeHTTP(addMemberRec, addMemberReq)
	if addMemberRec.Code != http.StatusOK {
		t.Fatalf("expected account group member add 200, got %d body=%s", addMemberRec.Code, addMemberRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account inspect: %v", err)
	}
	if getResp.Data.Id != accountResp.Data.Id || len(getResp.Data.GroupIds) != 1 || getResp.Data.GroupIds[0] != groupResp.Data.Id {
		t.Fatalf("expected inspected account with group id, got %+v", getResp.Data)
	}
	if strings.Contains(getRec.Body.String(), "account-ops-secret") {
		t.Fatalf("account inspect leaked credential material: %s", getRec.Body.String())
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/export", nil)
	exportReq.AddCookie(sessionCookie)
	exportRec := httptest.NewRecorder()
	handler.ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected account export 200, got %d body=%s", exportRec.Code, exportRec.Body.String())
	}
	if body := exportRec.Body.String(); strings.Contains(body, "account-ops-secret") ||
		strings.Contains(body, "metadata-access-token") ||
		strings.Contains(body, "metadata-cookie") ||
		strings.Contains(body, "nested-refresh-token") ||
		strings.Contains(body, "nested-api-key") {
		t.Fatalf("account export leaked sensitive material: %s", body)
	}
	var exportResp apiopenapi.ProviderAccountExportResponse
	if err := json.NewDecoder(exportRec.Body).Decode(&exportResp); err != nil {
		t.Fatalf("decode account export: %v", err)
	}
	item := findProviderAccountExportByName(exportResp.Data, "account-ops-account")
	if item == nil {
		t.Fatalf("exported account not found in %+v", exportResp.Data)
	}
	if item.CredentialExported || item.GroupIds == nil || len(*item.GroupIds) != 1 || (*item.GroupIds)[0] != groupResp.Data.Id {
		t.Fatalf("expected non-secret export with group id, got %+v", item)
	}
	if item.Metadata == nil || (*item.Metadata)["base_url"] != "https://upstream.example/v1" || (*item.Metadata)["nested"].(map[string]any)["safe"] != "kept" {
		t.Fatalf("expected safe metadata retained, got %+v", item.Metadata)
	}
}

func TestAdminAccountModelDiscoveryPreviewOpenAICompatible(t *testing.T) {
	var upstreamSeenAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		upstreamSeenAuth = r.Header.Get("Authorization")
		if r.Header.Get("X-Request-ID") != "" || r.Header.Get("Cookie") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi/client header leakage: %+v", r.Header)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"z-model"},{"id":"a-model"},{"id":"a-model"}]}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-openai-provider","display_name":"Discovery OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovery-openai-account","runtime_class":"api_key","credential":{"api_key":"discovery-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"limit":2}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamSeenAuth != "Bearer discovery-secret" {
		t.Fatalf("expected upstream bearer auth, got %q", upstreamSeenAuth)
	}
	if strings.Contains(rec.Body.String(), "discovery-secret") {
		t.Fatalf("discovery response leaked credential: %s", rec.Body.String())
	}
	var resp apiopenapi.AccountModelDiscoveryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if resp.Data.Source != apiopenapi.AccountModelDiscoverySourceOpenaiCompatible || resp.Data.Persisted || len(resp.Data.ModelIds) != 2 || resp.Data.ModelIds[0] != "a-model" || resp.Data.ModelIds[1] != "z-model" {
		t.Fatalf("unexpected discovery response: %+v", resp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata != nil && jsonObjectContainsString(*getResp.Data.Metadata, "a-model") {
		t.Fatalf("preview discovery should not persist supported models: %+v", getResp.Data.Metadata)
	}
}

func TestAdminAccountModelDiscoveryPersistsGeminiCompatible(t *testing.T) {
	var upstreamSeenKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models" {
			t.Fatalf("unexpected discovery path %s", r.URL.Path)
		}
		upstreamSeenKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-1.5-pro"},{"name":"models/gemini-1.5-flash"}]}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-gemini-provider","display_name":"Discovery Gemini","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovery-gemini-account","runtime_class":"api_key","credential":{"api_key":"gemini-discovery-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1beta"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamSeenKey != "gemini-discovery-secret" {
		t.Fatalf("expected Gemini API key query, got %q", upstreamSeenKey)
	}
	if strings.Contains(rec.Body.String(), "gemini-discovery-secret") {
		t.Fatalf("discovery response leaked credential: %s", rec.Body.String())
	}
	var resp apiopenapi.AccountModelDiscoveryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if resp.Data.Source != apiopenapi.AccountModelDiscoverySourceGeminiCompatible || !resp.Data.Persisted || !stringSliceContains(resp.Data.ModelIds, "gemini-1.5-pro") || !stringSliceContains(resp.Data.ModelIds, "gemini-1.5-flash") {
		t.Fatalf("unexpected Gemini discovery response: %+v", resp.Data)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), nil)
	getReq.AddCookie(sessionCookie)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected account inspect 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if getResp.Data.Metadata == nil || !jsonObjectContainsString(*getResp.Data.Metadata, "gemini-1.5-pro") || (*getResp.Data.Metadata)["model_discovery_source"] != "gemini-compatible" || (*getResp.Data.Metadata)["model_discovery_last_seen_at"] == "" {
		t.Fatalf("expected persisted discovery metadata, got %+v", getResp.Data.Metadata)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	if strings.Contains(auditRec.Body.String(), "gemini-discovery-secret") {
		t.Fatalf("audit log leaked discovery credential: %s", auditRec.Body.String())
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	discoveryAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.discover_models")
	if discoveryAudit.After["model_count"] != float64(2) && discoveryAudit.After["model_count"] != 2 {
		t.Fatalf("expected model_count audit evidence, got %+v", discoveryAudit.After)
	}
}

func TestAdminAccountModelDiscoveryRejectsUnsupportedRuntime(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-reverse-provider","display_name":"Discovery Reverse","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovery-reverse-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"oauth-access","refresh_token":"oauth-refresh"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected unsupported discovery 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if upstreamCalled {
		t.Fatal("unsupported runtime should not call upstream")
	}
}

func TestGatewayUsesDiscoveredSupportedModelsForCandidateFiltering(t *testing.T) {
	var (
		mu        sync.Mutex
		modelAuth string
		chatAuth  string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/v1/models":
			modelAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"discovered-upstream"}]}`))
		case "/v1/chat/completions":
			chatAuth = r.Header.Get("Authorization")
			if chatAuth != "Bearer discovered-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":{"message":"wrong account"}}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"routed through discovered account"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
		default:
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"discovery-routing-provider","display_name":"Discovery Routing Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"discovery-routing-model","display_name":"Discovery Routing Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"discovered-upstream","status":"active"}`)
	discoveredAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"discovered-account","runtime_class":"api_key","credential":{"api_key":"discovered-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.1},"status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"blocked-account","runtime_class":"api_key","credential":{"api_key":"blocked-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1","health_score":0.99,"supported_models":["other-upstream"]},"status":"active"}`)

	discoverReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(discoveredAccount.Data.Id)+"/discover-models", strings.NewReader(`{"persist":true}`))
	discoverReq.Header.Set("Content-Type", "application/json")
	discoverReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	discoverReq.AddCookie(sessionCookie)
	discoverRec := httptest.NewRecorder()
	handler.ServeHTTP(discoverRec, discoverReq)
	if discoverRec.Code != http.StatusOK {
		t.Fatalf("expected discovery 200, got %d body=%s", discoverRec.Code, discoverRec.Body.String())
	}
	if modelAuth != "Bearer discovered-secret" {
		t.Fatalf("expected discovery upstream auth, got %q", modelAuth)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"discovery-routing-model","messages":[{"role":"user","content":"route with discovered catalog"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	if chatAuth != "Bearer discovered-secret" {
		t.Fatalf("expected discovered account to be selected, got auth %q", chatAuth)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=discovery-routing-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SelectedAccountId == nil || *decisionsResp.Data[0].SelectedAccountId != string(discoveredAccount.Data.Id) {
		t.Fatalf("expected discovered account to win scheduling, got %+v", decisionsResp.Data)
	}
}

type stubDependencyPinger struct {
	err error
}

func (s stubDependencyPinger) Ping(_ context.Context) error {
	return s.err
}

func TestAdminCatalogFlow(t *testing.T) {
	handler := New(config.Load(), nil)

	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"anthropic","display_name":"Anthropic","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}

	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	if providerResp.Data.Name != "anthropic" {
		t.Fatalf("expected provider anthropic, got %s", providerResp.Data.Name)
	}

	createModelBody := `{"canonical_name":"claude-sonnet-4","display_name":"Claude Sonnet 4","family":"claude","context_window":200000,"max_output_tokens":4096,"quality_tier":"premium","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`
	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(createModelBody))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}

	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(createModelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}
	if modelResp.Data.CanonicalName != "claude-sonnet-4" {
		t.Fatalf("expected model claude-sonnet-4, got %s", modelResp.Data.CanonicalName)
	}

	createAliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/aliases", strings.NewReader(`{"alias":"claude-sonnet","fallback_models":["gpt-4o-mini"],"strategy_hint":"balanced","status":"active"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasReq.AddCookie(sessionCookie)
	createAliasReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAliasRec := httptest.NewRecorder()
	handler.ServeHTTP(createAliasRec, createAliasReq)
	if createAliasRec.Code != http.StatusCreated {
		t.Fatalf("expected model alias create 201, got %d", createAliasRec.Code)
	}
	var aliasResp apiopenapi.ModelAliasResponse
	if err := json.NewDecoder(createAliasRec.Body).Decode(&aliasResp); err != nil {
		t.Fatalf("decode model alias response: %v", err)
	}
	if aliasResp.Data.Alias != "claude-sonnet" || aliasResp.Data.ModelId != modelResp.Data.Id {
		t.Fatalf("unexpected alias response: %+v", aliasResp.Data)
	}

	createMappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"2","upstream_model_name":"claude-3-7-sonnet","status":"active","capability_override":[{"key":"streaming","level":"required","status":"stable","version":"v1"}],"pricing_override":{"currency":"USD"}}`))
	createMappingReq.Header.Set("Content-Type", "application/json")
	createMappingReq.AddCookie(sessionCookie)
	createMappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createMappingRec := httptest.NewRecorder()
	handler.ServeHTTP(createMappingRec, createMappingReq)
	if createMappingRec.Code != http.StatusCreated {
		t.Fatalf("expected model mapping create 201, got %d", createMappingRec.Code)
	}
	var mappingResp apiopenapi.ModelProviderMappingResponse
	if err := json.NewDecoder(createMappingRec.Body).Decode(&mappingResp); err != nil {
		t.Fatalf("decode model mapping response: %v", err)
	}
	if mappingResp.Data.ModelId != modelResp.Data.Id || mappingResp.Data.ProviderId != providerResp.Data.Id {
		t.Fatalf("unexpected mapping response: %+v", mappingResp.Data)
	}

	createAccountBody := `{"provider_id":"2","name":"anthropic-main","runtime_class":"api_key","credential":{"api_key":"secret-value"},"status":"active","priority":10,"weight":1.5}`
	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(createAccountBody))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.AddCookie(sessionCookie)
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", createAccountRec.Code)
	}

	var accountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(createAccountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	if accountResp.Data.Name != "anthropic-main" {
		t.Fatalf("expected account anthropic-main, got %s", accountResp.Data.Name)
	}

	assertAdminListContains(t, handler, sessionCookie, loginResp.Data.CsrfToken, "/api/v1/admin/providers", func(payload []byte) int {
		var resp apiopenapi.ProviderListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode providers list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertAdminListContains(t, handler, sessionCookie, loginResp.Data.CsrfToken, "/api/v1/admin/models", func(payload []byte) int {
		var resp apiopenapi.ModelListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode models list: %v", err)
		}
		return len(resp.Data)
	}, 2)
	assertAdminListContains(t, handler, sessionCookie, loginResp.Data.CsrfToken, "/api/v1/admin/accounts", func(payload []byte) int {
		var resp apiopenapi.ProviderAccountListResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode accounts list: %v", err)
		}
		return len(resp.Data)
	}, 2)

	capabilitiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/capabilities", nil)
	capabilitiesReq.AddCookie(sessionCookie)
	capabilitiesRec := httptest.NewRecorder()
	handler.ServeHTTP(capabilitiesRec, capabilitiesReq)
	if capabilitiesRec.Code != http.StatusOK {
		t.Fatalf("expected capabilities 200, got %d", capabilitiesRec.Code)
	}
	var capabilitiesResp apiopenapi.CapabilityListResponse
	if err := json.NewDecoder(capabilitiesRec.Body).Decode(&capabilitiesResp); err != nil {
		t.Fatalf("decode capabilities response: %v", err)
	}
	if len(capabilitiesResp.Data) < 4 {
		t.Fatalf("expected seeded capabilities, got %d", len(capabilitiesResp.Data))
	}

	strategiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/strategies", nil)
	strategiesReq.AddCookie(sessionCookie)
	strategiesRec := httptest.NewRecorder()
	handler.ServeHTTP(strategiesRec, strategiesReq)
	if strategiesRec.Code != http.StatusOK {
		t.Fatalf("expected strategies 200, got %d", strategiesRec.Code)
	}
	var strategiesResp apiopenapi.SchedulerStrategyListResponse
	if err := json.NewDecoder(strategiesRec.Body).Decode(&strategiesResp); err != nil {
		t.Fatalf("decode strategies response: %v", err)
	}
	if len(strategiesResp.Data) != 2 {
		t.Fatalf("expected 2 strategies, got %d", len(strategiesResp.Data))
	}
	for _, strategy := range strategiesResp.Data {
		if strategy.Version == "" || !strings.HasPrefix(strategy.ConfigHash, "sha256:") || len(strategy.Config) == 0 {
			t.Fatalf("expected seeded strategy registry descriptor, got %+v", strategy)
		}
	}

	apiKeyResp, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-sonnet","messages":[{"role":"user","content":"say hello"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d", chatRec.Code)
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat completion response: %v", err)
	}
	if len(chatResp.Choices) != 1 {
		t.Fatalf("expected 1 chat choice, got %d", len(chatResp.Choices))
	}
	if chatText := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); !strings.Contains(chatText, "say hello") {
		t.Fatalf("expected chat response to echo prompt, got %q", chatText)
	}

	chatStreamReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"claude-sonnet","messages":[{"role":"user","content":"stream hello"}],"stream":true}`))
	chatStreamReq.Header.Set("Content-Type", "application/json")
	chatStreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatStreamRec := httptest.NewRecorder()
	handler.ServeHTTP(chatStreamRec, chatStreamReq)
	if body := chatStreamRec.Body.String(); !strings.Contains(body, "data:") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("expected chat stream SSE payload, got %q", body)
	}

	responsesReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-sonnet","input":"say responses"}`))
	responsesReq.Header.Set("Content-Type", "application/json")
	responsesReq.Header.Set("Authorization", "Bearer "+apiKey)
	responsesRec := httptest.NewRecorder()
	handler.ServeHTTP(responsesRec, responsesReq)
	if responsesRec.Code != http.StatusOK {
		t.Fatalf("expected responses 200, got %d", responsesRec.Code)
	}
	var responsesResp apiopenapi.ResponsesResponse
	if err := json.NewDecoder(responsesRec.Body).Decode(&responsesResp); err != nil {
		t.Fatalf("decode responses response: %v", err)
	}
	if len(responsesResp.Output) != 1 || responsesResp.Output[0].Content == nil || len(*responsesResp.Output[0].Content) == 0 {
		t.Fatal("expected responses output content")
	}
	if text := (*responsesResp.Output[0].Content)[0].Text; text == nil || !strings.Contains(*text, "say responses") {
		t.Fatalf("expected responses prompt echo, got %v", text)
	}

	responsesStreamReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"claude-sonnet","input":"stream responses","stream":true}`))
	responsesStreamReq.Header.Set("Content-Type", "application/json")
	responsesStreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	responsesStreamRec := httptest.NewRecorder()
	handler.ServeHTTP(responsesStreamRec, responsesStreamReq)
	if body := responsesStreamRec.Body.String(); !strings.Contains(body, "event: response.output_text.delta") || !strings.Contains(body, "event: response.completed") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected responses stream SSE payload, got %q", body)
	}

	messageReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet","max_tokens":16,"messages":[{"role":"user","content":"say messages"}]}`))
	messageReq.Header.Set("Content-Type", "application/json")
	messageReq.Header.Set("Authorization", "Bearer "+apiKey)
	messageRec := httptest.NewRecorder()
	handler.ServeHTTP(messageRec, messageReq)
	if messageRec.Code != http.StatusOK {
		t.Fatalf("expected messages 200, got %d", messageRec.Code)
	}
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || !strings.Contains(*messageResp.Content[0].Text, "say messages") {
		t.Fatalf("expected messages prompt echo, got %v", messageResp.Content)
	}

	messageStreamReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet","max_tokens":16,"messages":[{"role":"user","content":"stream messages"}],"stream":true}`))
	messageStreamReq.Header.Set("Content-Type", "application/json")
	messageStreamReq.Header.Set("Authorization", "Bearer "+apiKey)
	messageStreamRec := httptest.NewRecorder()
	handler.ServeHTTP(messageStreamRec, messageStreamReq)
	if body := messageStreamRec.Body.String(); !strings.Contains(body, "event: message_start") || !strings.Contains(body, "event: content_block_delta") || !strings.Contains(body, "event: message_stop") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("expected messages stream SSE payload, got %q", body)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) < 6 {
		t.Fatalf("expected at least 6 scheduler decisions, got %d", len(decisionsResp.Data))
	}
	latestDecision := decisionsResp.Data[len(decisionsResp.Data)-1]
	if latestDecision.SelectedAccountId == nil || latestDecision.CandidateCount == 0 || len(latestDecision.Scores) == 0 {
		t.Fatalf("expected selected account and score payload, got %+v", latestDecision)
	}
	if latestDecision.StrategyVersion == "" || !strings.HasPrefix(latestDecision.StrategyConfigHash, "sha256:") || len(latestDecision.StrategyWeights) == 0 {
		t.Fatalf("expected strategy version/hash/weights snapshot, got %+v", latestDecision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) < 6 {
		t.Fatalf("expected at least 6 usage logs, got %d", len(usageResp.Data))
	}
	for _, item := range usageResp.Data {
		if !item.Success {
			t.Fatalf("expected successful usage log, got %+v", item)
		}
		if !item.UsageEstimated {
			t.Fatalf("expected estimated usage log, got %+v", item)
		}
		if item.TotalTokens == 0 {
			t.Fatalf("expected token usage, got %+v", item)
		}
	}

	meUsageReq := httptest.NewRequest(http.MethodGet, "/api/v1/me/usage", nil)
	meUsageReq.AddCookie(sessionCookie)
	meUsageRec := httptest.NewRecorder()
	handler.ServeHTTP(meUsageRec, meUsageReq)
	if meUsageRec.Code != http.StatusOK {
		t.Fatalf("expected current user usage 200, got %d", meUsageRec.Code)
	}
	var meUsageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(meUsageRec.Body).Decode(&meUsageResp); err != nil {
		t.Fatalf("decode current user usage: %v", err)
	}
	if len(meUsageResp.Data) != len(usageResp.Data) {
		t.Fatalf("expected current user usage count %d, got %d", len(usageResp.Data), len(meUsageResp.Data))
	}

	adminOverviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/overview", nil)
	adminOverviewReq.AddCookie(sessionCookie)
	adminOverviewRec := httptest.NewRecorder()
	handler.ServeHTTP(adminOverviewRec, adminOverviewReq)
	if adminOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected admin overview 200, got %d", adminOverviewRec.Code)
	}
	var adminOverviewResp apiopenapi.AdminOverviewResponse
	if err := json.NewDecoder(adminOverviewRec.Body).Decode(&adminOverviewResp); err != nil {
		t.Fatalf("decode admin overview: %v", err)
	}
	if adminOverviewResp.Data.ProviderCount != 2 || adminOverviewResp.Data.ModelCount != 2 || adminOverviewResp.Data.ActiveAccountCount != 2 {
		t.Fatalf("unexpected admin overview: %+v", adminOverviewResp.Data)
	}
	if adminOverviewResp.Data.UsageLogCount < 6 || adminOverviewResp.Data.SchedulerDecisionCount < 6 {
		t.Fatalf("expected usage and decision counts in overview, got %+v", adminOverviewResp.Data)
	}

	schedulerOverviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/overview", nil)
	schedulerOverviewReq.AddCookie(sessionCookie)
	schedulerOverviewRec := httptest.NewRecorder()
	handler.ServeHTTP(schedulerOverviewRec, schedulerOverviewReq)
	if schedulerOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler overview 200, got %d", schedulerOverviewRec.Code)
	}
	var schedulerOverviewResp apiopenapi.SchedulerOverviewResponse
	if err := json.NewDecoder(schedulerOverviewRec.Body).Decode(&schedulerOverviewResp); err != nil {
		t.Fatalf("decode scheduler overview: %v", err)
	}
	if schedulerOverviewResp.Data.TotalDecisions < 6 || schedulerOverviewResp.Data.SelectedDecisions < 6 || schedulerOverviewResp.Data.SuccessRate != 1 {
		t.Fatalf("unexpected scheduler overview: %+v", schedulerOverviewResp.Data)
	}

	accountHealthReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/health", nil)
	accountHealthReq.AddCookie(sessionCookie)
	accountHealthRec := httptest.NewRecorder()
	handler.ServeHTTP(accountHealthRec, accountHealthReq)
	if accountHealthRec.Code != http.StatusOK {
		t.Fatalf("expected account health 200, got %d", accountHealthRec.Code)
	}
	var accountHealthResp apiopenapi.AccountHealthResponse
	if err := json.NewDecoder(accountHealthRec.Body).Decode(&accountHealthResp); err != nil {
		t.Fatalf("decode account health: %v", err)
	}
	if accountHealthResp.Data.Status != "healthy" || accountHealthResp.Data.SuccessRate != 1 {
		t.Fatalf("unexpected account health: %+v", accountHealthResp.Data)
	}

	accountQuotaReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/quota", nil)
	accountQuotaReq.AddCookie(sessionCookie)
	accountQuotaRec := httptest.NewRecorder()
	handler.ServeHTTP(accountQuotaRec, accountQuotaReq)
	if accountQuotaRec.Code != http.StatusOK {
		t.Fatalf("expected account quota 200, got %d", accountQuotaRec.Code)
	}
	var accountQuotaResp apiopenapi.AccountQuotaListResponse
	if err := json.NewDecoder(accountQuotaRec.Body).Decode(&accountQuotaResp); err != nil {
		t.Fatalf("decode account quota: %v", err)
	}
	if len(accountQuotaResp.Data) == 0 || accountQuotaResp.Data[0].QuotaType != "monthly_tokens" || accountQuotaResp.Data[0].Used == "0" {
		t.Fatalf("unexpected account quota: %+v", accountQuotaResp.Data)
	}

	providerTestReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers/"+string(providerResp.Data.Id)+"/test", nil)
	providerTestReq.AddCookie(sessionCookie)
	providerTestReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerTestRec := httptest.NewRecorder()
	handler.ServeHTTP(providerTestRec, providerTestReq)
	if providerTestRec.Code != http.StatusOK {
		t.Fatalf("expected provider test 200, got %d body=%s", providerTestRec.Code, providerTestRec.Body.String())
	}
	var providerTestResp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(providerTestRec.Body).Decode(&providerTestResp); err != nil {
		t.Fatalf("decode provider test: %v", err)
	}
	if !providerTestResp.Data.Ok || providerTestResp.Data.ProviderId == nil || *providerTestResp.Data.ProviderId != providerResp.Data.Id {
		t.Fatalf("unexpected provider test result: %+v", providerTestResp.Data)
	}

	accountTestReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/test", nil)
	accountTestReq.AddCookie(sessionCookie)
	accountTestReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountTestRec := httptest.NewRecorder()
	handler.ServeHTTP(accountTestRec, accountTestReq)
	if accountTestRec.Code != http.StatusOK {
		t.Fatalf("expected account test 200, got %d body=%s", accountTestRec.Code, accountTestRec.Body.String())
	}
	var accountTestResp apiopenapi.AdminTestResultResponse
	if err := json.NewDecoder(accountTestRec.Body).Decode(&accountTestResp); err != nil {
		t.Fatalf("decode account test: %v", err)
	}
	if !accountTestResp.Data.Ok || accountTestResp.Data.AccountId == nil || *accountTestResp.Data.AccountId != accountResp.Data.Id {
		t.Fatalf("unexpected account test result: %+v", accountTestResp.Data)
	}

	updateProviderReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/providers/"+string(providerResp.Data.Id), strings.NewReader(`{"display_name":"Anthropic Updated","status":"disabled"}`))
	updateProviderReq.Header.Set("Content-Type", "application/json")
	updateProviderReq.AddCookie(sessionCookie)
	updateProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(updateProviderRec, updateProviderReq)
	if updateProviderRec.Code != http.StatusOK {
		t.Fatalf("expected provider update 200, got %d", updateProviderRec.Code)
	}
	var updateProviderResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(updateProviderRec.Body).Decode(&updateProviderResp); err != nil {
		t.Fatalf("decode provider update: %v", err)
	}
	if updateProviderResp.Data.DisplayName != "Anthropic Updated" || updateProviderResp.Data.Status != apiopenapi.ResourceStatusDisabled {
		t.Fatalf("unexpected provider update response: %+v", updateProviderResp.Data)
	}

	updateModelReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/models/"+string(modelResp.Data.Id), strings.NewReader(`{"display_name":"Claude Sonnet Updated","status":"disabled","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"json_mode","level":"optional","status":"stable","version":"v1"}]}`))
	updateModelReq.Header.Set("Content-Type", "application/json")
	updateModelReq.AddCookie(sessionCookie)
	updateModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateModelRec := httptest.NewRecorder()
	handler.ServeHTTP(updateModelRec, updateModelReq)
	if updateModelRec.Code != http.StatusOK {
		t.Fatalf("expected model update 200, got %d", updateModelRec.Code)
	}
	var updateModelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(updateModelRec.Body).Decode(&updateModelResp); err != nil {
		t.Fatalf("decode model update: %v", err)
	}
	if updateModelResp.Data.DisplayName != "Claude Sonnet Updated" || updateModelResp.Data.Status != apiopenapi.ResourceStatusDisabled || len(updateModelResp.Data.Capabilities) != 2 {
		t.Fatalf("unexpected model update response: %+v", updateModelResp.Data)
	}

	updateAccountReq := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/accounts/"+string(accountResp.Data.Id), strings.NewReader(`{"name":"anthropic-main-updated","priority":20,"weight":2,"metadata":{"base_url":"https://example.invalid/v1"}}`))
	updateAccountReq.Header.Set("Content-Type", "application/json")
	updateAccountReq.AddCookie(sessionCookie)
	updateAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	updateAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(updateAccountRec, updateAccountReq)
	if updateAccountRec.Code != http.StatusOK {
		t.Fatalf("expected account update 200, got %d", updateAccountRec.Code)
	}
	var updateAccountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(updateAccountRec.Body).Decode(&updateAccountResp); err != nil {
		t.Fatalf("decode account update: %v", err)
	}
	if updateAccountResp.Data.Name != "anthropic-main-updated" || updateAccountResp.Data.Priority != 20 {
		t.Fatalf("unexpected account update response: %+v", updateAccountResp.Data)
	}

	disableAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/disable", nil)
	disableAccountReq.AddCookie(sessionCookie)
	disableAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	disableAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(disableAccountRec, disableAccountReq)
	if disableAccountRec.Code != http.StatusOK {
		t.Fatalf("expected account disable 200, got %d", disableAccountRec.Code)
	}
	var disableAccountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(disableAccountRec.Body).Decode(&disableAccountResp); err != nil {
		t.Fatalf("decode account disable: %v", err)
	}
	if disableAccountResp.Data.Status != apiopenapi.ProviderAccountStatusDisabled {
		t.Fatalf("expected disabled account, got %+v", disableAccountResp.Data)
	}

	enableAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/enable", nil)
	enableAccountReq.AddCookie(sessionCookie)
	enableAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	enableAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(enableAccountRec, enableAccountReq)
	if enableAccountRec.Code != http.StatusOK {
		t.Fatalf("expected account enable 200, got %d", enableAccountRec.Code)
	}
	var enableAccountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(enableAccountRec.Body).Decode(&enableAccountResp); err != nil {
		t.Fatalf("decode account enable: %v", err)
	}
	if enableAccountResp.Data.Status != apiopenapi.ProviderAccountStatusActive {
		t.Fatalf("expected active account, got %+v", enableAccountResp.Data)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	for _, action := range []string{"api_key.create", "provider.create", "model.create", "model_alias.create", "model_provider_mapping.create", "provider_account.create", "provider.test", "provider_account.test", "provider.update", "model.update", "provider_account.update", "provider_account.disable", "provider_account.enable"} {
		if !auditLogHasAction(auditResp.Data, action) {
			t.Fatalf("expected audit action %s in %+v", action, auditResp.Data)
		}
	}
	if auditResp.Data[0].TraceId == "" || auditResp.Data[0].ActorUserId == nil {
		t.Fatalf("expected audit trace and actor, got %+v", auditResp.Data[0])
	}
	providerUpdateAudit := mustFindAuditLog(t, auditResp.Data, "provider.update")
	if providerUpdateAudit.Before["status"] == providerUpdateAudit.After["status"] || providerUpdateAudit.After["status"] != "disabled" {
		t.Fatalf("expected provider audit before/after status change, got before=%+v after=%+v", providerUpdateAudit.Before, providerUpdateAudit.After)
	}
	accountUpdateAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.update")
	if _, ok := accountUpdateAudit.After["credential_ciphertext"]; ok {
		t.Fatalf("account audit must not expose credential ciphertext: %+v", accountUpdateAudit.After)
	}

	billingReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/billing-ledger?reference_type=usage_log", nil)
	billingReq.AddCookie(sessionCookie)
	billingRec := httptest.NewRecorder()
	handler.ServeHTTP(billingRec, billingReq)
	if billingRec.Code != http.StatusOK {
		t.Fatalf("expected billing ledger 200, got %d", billingRec.Code)
	}
	var billingResp apiopenapi.BillingLedgerListResponse
	if err := json.NewDecoder(billingRec.Body).Decode(&billingResp); err != nil {
		t.Fatalf("decode billing ledger: %v", err)
	}
	if len(billingResp.Data) < len(usageResp.Data) {
		t.Fatalf("expected billing ledger for usage logs, got %d ledger entries for %d usage logs", len(billingResp.Data), len(usageResp.Data))
	}
	for _, entry := range billingResp.Data {
		if entry.Type != apiopenapi.UsageCharge || entry.ReferenceType != "usage_log" || entry.UserId == "" {
			t.Fatalf("unexpected billing ledger entry: %+v", entry)
		}
		if entry.Metadata["request_id"] == nil || entry.Metadata["total_tokens"] == nil {
			t.Fatalf("expected billing metadata to reference usage, got %+v", entry.Metadata)
		}
	}

	outboxReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/events/outbox?event_type=GatewayRequestCompleted", nil)
	outboxReq.AddCookie(sessionCookie)
	outboxRec := httptest.NewRecorder()
	handler.ServeHTTP(outboxRec, outboxReq)
	if outboxRec.Code != http.StatusOK {
		t.Fatalf("expected outbox 200, got %d", outboxRec.Code)
	}
	var outboxResp apiopenapi.DomainEventOutboxListResponse
	if err := json.NewDecoder(outboxRec.Body).Decode(&outboxResp); err != nil {
		t.Fatalf("decode outbox events: %v", err)
	}
	if len(outboxResp.Data) < len(usageResp.Data) {
		t.Fatalf("expected outbox events for usage logs, got %d events for %d usage logs", len(outboxResp.Data), len(usageResp.Data))
	}
	for _, event := range outboxResp.Data {
		if event.EventType != "GatewayRequestCompleted" || event.ProducerModule != "gateway" || event.Status != apiopenapi.DomainEventOutboxStatusPending {
			t.Fatalf("unexpected outbox event: %+v", event)
		}
		payload, err := json.Marshal(event.Payload)
		if err != nil {
			t.Fatalf("marshal outbox payload: %v", err)
		}
		if strings.Contains(string(payload), "say hello") || strings.Contains(string(payload), "secret-value") {
			t.Fatalf("outbox payload leaked request or credential material: %s", string(payload))
		}
	}

	_ = apiKeyResp
}

func TestGatewayRejectsModelWithoutProviderMapping(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"unmapped-provider","display_name":"Unmapped Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}

	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"unmapped-model","display_name":"Unmapped Model","status":"active"}`))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}

	apiKeyResp, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"unmapped-model","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unmapped model 503, got %d", rec.Code)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=unmapped-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected scheduler decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode scheduler decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected 1 unmapped decision, got %d", len(decisionsResp.Data))
	}
	if decisionsResp.Data[0].SelectedAccountId != nil || decisionsResp.Data[0].CandidateCount != 0 {
		t.Fatalf("expected no selected account and no candidates, got %+v", decisionsResp.Data[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=unmapped-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].Success || usageResp.Data[0].ErrorClass == nil || *usageResp.Data[0].ErrorClass != "no_available_account" {
		t.Fatalf("expected failed no_available_account usage log, got %+v", usageResp.Data)
	}

	outboxReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/ops/events/outbox?event_type=GatewayRequestCompleted", nil)
	outboxReq.AddCookie(sessionCookie)
	outboxRec := httptest.NewRecorder()
	handler.ServeHTTP(outboxRec, outboxReq)
	if outboxRec.Code != http.StatusOK {
		t.Fatalf("expected outbox 200, got %d", outboxRec.Code)
	}
	var outboxResp apiopenapi.DomainEventOutboxListResponse
	if err := json.NewDecoder(outboxRec.Body).Decode(&outboxResp); err != nil {
		t.Fatalf("decode outbox events: %v", err)
	}
	if len(outboxResp.Data) != 1 {
		t.Fatalf("expected one failed gateway outbox event, got %d", len(outboxResp.Data))
	}
	event := outboxResp.Data[0]
	if event.Payload["success"] != false || event.Payload["error_class"] != "no_available_account" || event.AggregateType != "usage_log" {
		t.Fatalf("unexpected failed gateway outbox event: %+v", event)
	}
	_ = apiKeyResp
}

func TestGatewaySubscriptionEntitlementRejectsBeforeSchedulerConsumesAccount(t *testing.T) {
	schedulerStore := schedulermemory.New()
	subscriptionStore := subscriptionmemory.New()
	subscriptionSvc, err := subscriptionservice.New(subscriptionStore, nil)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	handler := New(config.Load(), nil,
		WithSchedulerStore(schedulerStore),
		WithSubscriptionStore(subscriptionStore),
	)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"entitlement-provider","display_name":"Entitlement Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"blocked-model","display_name":"Blocked Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"blocked-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"entitlement-account","runtime_class":"api_key","credential":{"api_key":"entitlement-secret"},"status":"active"}`)

	userID, err := strconv.Atoi(string(loginResp.Data.User.Id))
	if err != nil {
		t.Fatalf("parse login user id: %v", err)
	}
	plan, err := subscriptionSvc.CreatePlan(t.Context(), subscriptioncontract.CreatePlanRequest{
		Name:         "locked-plan",
		Price:        "0",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"allowed_models": []any{"allowed-model"},
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := subscriptionSvc.CreateUserSubscription(t.Context(), subscriptioncontract.CreateSubscriptionRequest{
		UserID: userID,
		PlanID: plan.ID,
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"blocked-model","messages":[{"role":"user","content":"should be rejected"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected entitlement rejection 403, got %d body=%s", rec.Code, rec.Body.String())
	}
	var gatewayErr apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&gatewayErr); err != nil {
		t.Fatalf("decode gateway error: %v", err)
	}
	if gatewayErr.Error.Code == nil || *gatewayErr.Error.Code != "entitlement_model_not_allowed" {
		t.Fatalf("expected entitlement error code, got %+v", gatewayErr.Error)
	}

	decisions, err := schedulerStore.ListDecisions(t.Context())
	if err != nil {
		t.Fatalf("list scheduler decisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("entitlement rejection must happen before scheduler decision, got %+v", decisions)
	}
	leases, err := schedulerStore.ListLeases(t.Context())
	if err != nil {
		t.Fatalf("list scheduler leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("entitlement rejection must not acquire leases, got %+v", leases)
	}
	feedbacks, err := schedulerStore.ListFeedbacks(t.Context())
	if err != nil {
		t.Fatalf("list scheduler feedbacks: %v", err)
	}
	if len(feedbacks) != 0 {
		t.Fatalf("entitlement rejection must not record scheduler feedback, got %+v", feedbacks)
	}
}

func TestGatewayInvokesOpenAICompatibleProviderAdapter(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer upstream-secret" {
			t.Fatalf("unexpected upstream authorization %q", got)
		}
		if r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		if payload.Model != "upstream-model" || len(payload.Messages) != 1 || !strings.Contains(payload.Messages[0].Content, "call upstream") {
			t.Fatalf("unexpected upstream payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"upstream adapter response"}}],"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"upstream-openai","display_name":"Upstream OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.AddCookie(sessionCookie)
	providerReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", providerRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	modelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"adapter-model","display_name":"Adapter Model","status":"active"}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.AddCookie(sessionCookie)
	modelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", modelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	mappingBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"upstream-model","status":"active"}`
	mappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(mappingBody))
	mappingReq.Header.Set("Content-Type", "application/json")
	mappingReq.AddCookie(sessionCookie)
	mappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	mappingRec := httptest.NewRecorder()
	handler.ServeHTTP(mappingRec, mappingReq)
	if mappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", mappingRec.Code)
	}

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"upstream-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1"},"status":"active"}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")
	accountReq.AddCookie(sessionCookie)
	accountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", accountRec.Code)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"adapter-model","messages":[{"role":"user","content":"call upstream"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected chat completion 200, got %d", chatRec.Code)
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream call, got %d", upstreamCalls)
	}
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if text := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); text != "upstream adapter response" {
		t.Fatalf("expected upstream response text, got %q", text)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=adapter-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].UsageEstimated || usageResp.Data[0].TotalTokens != 12 {
		t.Fatalf("expected parsed upstream usage, got %+v", usageResp.Data)
	}
}

func TestGatewayGeminiGenerateContentUsesCanonicalPipeline(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []upstreamGeminiCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model     string            `json:"model"`
			Messages  []upstreamMessage `json:"messages"`
			MaxTokens int               `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamGeminiCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Messages:      payload.Messages,
			MaxTokens:     payload.MaxTokens,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"gemini upstream response"}}],"usage":{"prompt_tokens":6,"completion_tokens":8,"total_tokens":14}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp, modelResp, accountResp := mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"systemInstruction":{"parts":[{"text":"be brief"}]},"contents":[{"role":"user","parts":[{"text":"gemini route prompt"}]}],"generationConfig":{"maxOutputTokens":32,"temperature":0.2,"topP":0.8}}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/gemini-route-model:generateContent", body)
	var resp apiopenapi.GeminiGenerateContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gemini response: %v", err)
	}
	if len(resp.Candidates) != 1 || len(resp.Candidates[0].Content.Parts) != 1 || resp.Candidates[0].Content.Parts[0].Text == nil || *resp.Candidates[0].Content.Parts[0].Text != "gemini upstream response" {
		t.Fatalf("unexpected gemini response: %+v", resp)
	}
	if resp.UsageMetadata == nil || resp.UsageMetadata.TotalTokenCount == nil || *resp.UsageMetadata.TotalTokenCount != 14 {
		t.Fatalf("expected parsed gemini usage metadata, got %+v", resp.UsageMetadata)
	}

	mu.Lock()
	gotCalls := append([]upstreamGeminiCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/chat/completions" || call.Authorization != "Bearer gemini-route-upstream-secret" || call.Model != "gemini-route-upstream" || call.MaxTokens != 32 {
		t.Fatalf("unexpected upstream call: %+v", call)
	}
	if len(call.Messages) != 2 || call.Messages[0].Role != "system" || call.Messages[0].Content != "be brief" || call.Messages[1].Role != "user" || !strings.Contains(call.Messages[1].Content, "gemini route prompt") {
		t.Fatalf("expected canonical messages forwarded upstream, got %+v", call.Messages)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=gemini-route-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceProtocol != "gemini-compatible" || usage.SourceEndpoint != "/v1beta/models/gemini-route-model:generateContent" || usage.TargetProtocol == nil || *usage.TargetProtocol != "openai-compatible" || usage.TotalTokens != 14 || usage.UsageEstimated {
		t.Fatalf("unexpected gemini usage record: %+v", usage)
	}
	if usage.ProviderId == nil || *usage.ProviderId != providerResp.Data.Id || usage.AccountId == nil || *usage.AccountId != accountResp.Data.Id || usage.Model != modelResp.Data.CanonicalName {
		t.Fatalf("expected provider/account/model evidence, got %+v", usage)
	}
}

func TestGatewayGeminiGenerateContentSchedulesGeminiCompatibleUpstream(t *testing.T) {
	var (
		mu    sync.Mutex
		calls []upstreamNativeGeminiCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Contents []struct {
				Role  string `json:"role"`
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
			SystemInstruction *struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"systemInstruction"`
			GenerationConfig *struct {
				MaxOutputTokens int `json:"maxOutputTokens"`
			} `json:"generationConfig"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode gemini upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamNativeGeminiCall{
			Path:       r.URL.Path,
			APIKey:     r.URL.Query().Get("key"),
			Contents:   payload.Contents,
			SystemText: geminiSystemInstructionText(payload.SystemInstruction),
			MaxTokens:  payload.GenerationConfig.MaxOutputTokens,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"native gemini upstream response"}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp, modelResp, accountResp := mustCreateNativeGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	body := `{"systemInstruction":{"parts":[{"text":"answer tersely"}]},"contents":[{"role":"user","parts":[{"text":"native gemini prompt"}]}],"generationConfig":{"maxOutputTokens":48}}`
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/native-gemini-route-model:generateContent", body)
	var resp apiopenapi.GeminiGenerateContentResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode gemini response: %v", err)
	}
	if len(resp.Candidates) != 1 || len(resp.Candidates[0].Content.Parts) != 1 || resp.Candidates[0].Content.Parts[0].Text == nil || *resp.Candidates[0].Content.Parts[0].Text != "native gemini upstream response" {
		t.Fatalf("unexpected native gemini response: %+v", resp)
	}

	mu.Lock()
	gotCalls := append([]upstreamNativeGeminiCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one native gemini upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1beta/models/gemini-native-upstream:generateContent" || call.APIKey != "native-gemini-secret" || call.MaxTokens != 48 {
		t.Fatalf("unexpected native gemini call: %+v", call)
	}
	if call.SystemText != "answer tersely" || len(call.Contents) != 1 || call.Contents[0].Role != "user" || call.Contents[0].Parts[0].Text != "native gemini prompt" {
		t.Fatalf("expected canonical Gemini payload, got %+v", call)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=native-gemini-route-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.TargetProtocol == nil || *usage.TargetProtocol != "gemini-compatible" || usage.TotalTokens != 15 || usage.UsageEstimated {
		t.Fatalf("unexpected native gemini usage record: %+v", usage)
	}
	if usage.ProviderId == nil || *usage.ProviderId != providerResp.Data.Id || usage.AccountId == nil || *usage.AccountId != accountResp.Data.Id || usage.Model != modelResp.Data.CanonicalName {
		t.Fatalf("expected provider/account/model evidence, got %+v", usage)
	}
}

func TestGatewayGeminiStreamGenerateContentEmitsGeminiSSE(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"gemini stream response\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1beta/models/gemini-route-model:streamGenerateContent", `{"contents":[{"role":"user","parts":[{"text":"stream gemini"}]}]}`)
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected event stream content type, got %q", got)
	}
	body := rec.Body.String()
	for _, expected := range []string{"data:", "gemini stream response", "usageMetadata", "data: [DONE]"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected SSE body to contain %q, got %s", expected, body)
		}
	}
}

func TestGatewayGeminiProviderErrorsUseGoogleStyleEnvelope(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit","message":"slow down"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateGeminiGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, upstream.URL, false)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-route-model:generateContent", strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"rate limit me"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected Gemini rate limit 429, got %d body=%s", rec.Code, rec.Body.String())
	}
	var errResp apiopenapi.GeminiErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode gemini error: %v", err)
	}
	if errResp.Error.Code != http.StatusTooManyRequests || errResp.Error.Status != "RESOURCE_EXHAUSTED" || !strings.Contains(errResp.Error.Message, "rate limit") {
		t.Fatalf("unexpected gemini error envelope: %+v", errResp)
	}
}

func TestGatewayRequestRecordsSchedulerFeedback(t *testing.T) {
	schedulerStore := schedulermemory.New()
	handler := New(config.Load(), nil, WithSchedulerStore(schedulerStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"feedback-provider","display_name":"Feedback Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"feedback-model","display_name":"Feedback Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"feedback-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"feedback-account","runtime_class":"api_key","credential":{"api_key":"feedback-secret"},"status":"active"}`)

	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"feedback-model","messages":[{"role":"user","content":"record feedback"}]}`)

	feedbacks, err := schedulerStore.ListFeedbacks(t.Context())
	if err != nil {
		t.Fatalf("list scheduler feedbacks: %v", err)
	}
	if len(feedbacks) != 1 {
		t.Fatalf("expected one scheduler feedback, got %+v", feedbacks)
	}
	accountID, err := strconv.Atoi(string(accountResp.Data.Id))
	if err != nil {
		t.Fatalf("parse account id: %v", err)
	}
	providerID, err := strconv.Atoi(string(providerResp.Data.Id))
	if err != nil {
		t.Fatalf("parse provider id: %v", err)
	}
	feedback := feedbacks[0]
	if !feedback.Success || feedback.DecisionID <= 0 || feedback.AccountID != accountID || feedback.ProviderID != providerID {
		t.Fatalf("unexpected scheduler feedback identity: %+v", feedback)
	}
	if feedback.Model != "feedback-model" || feedback.InputTokens == 0 || feedback.OutputTokens == 0 || feedback.ActualCost != "0.00000000" || feedback.Currency != "USD" {
		t.Fatalf("unexpected scheduler feedback evidence: %+v", feedback)
	}
}

func TestGatewayCompatibilityEndpointsTargetSameOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		Model         string
		Messages      []upstreamMessage
	}

	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model    string            `json:"model"`
			Messages []upstreamMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Messages:      payload.Messages,
		})
		mu.Unlock()

		content := "upstream compatibility response"
		if len(payload.Messages) > 0 {
			content = "upstream echo: " + payload.Messages[len(payload.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":` + strconv.Quote(content) + `}}],"usage":{"prompt_tokens":4,"completion_tokens":5,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp080-openai","display_name":"WP080 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp080-model","display_name":"WP080 Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"},{"key":"tool_calling","level":"optional","status":"stable","version":"v1"},{"key":"structured_output","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"wp080-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp080-upstream-account","runtime_class":"api_key","credential":{"api_key":"wp080-upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"wp080-model","messages":[{"role":"user","content":"chat same upstream"}]}`)
	var chatResp apiopenapi.ChatCompletionResponse
	if err := json.NewDecoder(chatRec.Body).Decode(&chatResp); err != nil {
		t.Fatalf("decode chat response: %v", err)
	}
	if text := decodeChatMessageText(t, chatResp.Choices[0].Message.Content); !strings.Contains(text, "chat same upstream") {
		t.Fatalf("expected chat response from upstream, got %q", text)
	}

	responsesRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/responses", `{"model":"wp080-model","instructions":"respond briefly","input":"responses same upstream","max_output_tokens":32}`)
	var responsesResp apiopenapi.ResponsesResponse
	if err := json.NewDecoder(responsesRec.Body).Decode(&responsesResp); err != nil {
		t.Fatalf("decode responses response: %v", err)
	}
	if len(responsesResp.Output) != 1 || responsesResp.Output[0].Content == nil || len(*responsesResp.Output[0].Content) == 0 {
		t.Fatal("expected responses output content")
	}
	if text := (*responsesResp.Output[0].Content)[0].Text; text == nil || !strings.Contains(*text, "responses same upstream") {
		t.Fatalf("expected responses response from upstream, got %v", text)
	}

	messageRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/messages", `{"model":"wp080-model","system":"respond briefly","max_tokens":32,"messages":[{"role":"user","content":"messages same upstream"}]}`)
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || !strings.Contains(*messageResp.Content[0].Text, "messages same upstream") {
		t.Fatalf("expected messages response from upstream, got %v", messageResp.Content)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 3 {
		t.Fatalf("expected three upstream calls, got %d: %+v", len(gotCalls), gotCalls)
	}
	expectedPrompts := []string{"chat same upstream", "responses same upstream", "messages same upstream"}
	for idx, call := range gotCalls {
		if call.Path != "/v1/chat/completions" {
			t.Fatalf("expected upstream chat completions path for call %d, got %s", idx, call.Path)
		}
		if call.Authorization != "Bearer wp080-upstream-secret" {
			t.Fatalf("expected upstream credential for call %d, got %q", idx, call.Authorization)
		}
		if call.Model != "wp080-upstream" {
			t.Fatalf("expected upstream model for call %d, got %q", idx, call.Model)
		}
		if len(call.Messages) == 0 || !strings.Contains(call.Messages[len(call.Messages)-1].Content, expectedPrompts[idx]) {
			t.Fatalf("expected upstream prompt %q for call %d, got %+v", expectedPrompts[idx], idx, call.Messages)
		}
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp080-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 3 {
		t.Fatalf("expected three usage records, got %+v", usageResp.Data)
	}
	for _, item := range usageResp.Data {
		if !item.Success || item.AccountId == nil || *item.AccountId != accountResp.Data.Id || item.ProviderId == nil || *item.ProviderId != providerResp.Data.Id {
			t.Fatalf("expected successful usage on same provider account, got %+v", item)
		}
	}
}

func TestGatewayProviderAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")

	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"fallback-provider","display_name":"Fallback Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"alias-model","display_name":"Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-upstream","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	req := httptest.NewRequest(http.MethodPost, "/api/provider/openai-compatible/v1/chat/completions", strings.NewReader(`{"model":"alias-model","messages":[{"role":"user","content":"alias route"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider alias chat 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias decision, got %d", len(decisionsResp.Data))
	}
	if decisionsResp.Data[0].SelectedProviderId == nil || *decisionsResp.Data[0].SelectedProviderId != string(openaiProvider.Id) {
		t.Fatalf("expected alias to force openai-compatible provider, got %+v", decisionsResp.Data[0])
	}
	if decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("expected alias to filter candidates to one provider, got %+v", decisionsResp.Data[0])
	}
	if decisionsResp.Data[0].SourceEndpoint != "/api/provider/openai-compatible/v1/chat/completions" {
		t.Fatalf("expected alias source endpoint in decision, got %q", decisionsResp.Data[0].SourceEndpoint)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=alias-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].SourceEndpoint != "/api/provider/openai-compatible/v1/chat/completions" {
		t.Fatalf("expected alias source endpoint in usage log, got %+v", usageResp.Data)
	}
}

func TestGatewayProviderAliasUsesPresetProviderKey(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	deepseekProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"deepseek","display_name":"DeepSeek","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"deepseek-fallback-provider","display_name":"DeepSeek Fallback Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"deepseek-alias-model","display_name":"DeepSeek Alias Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"deepseek-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(deepseekProvider.Data.Id)+`","upstream_model_name":"deepseek-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(deepseekProvider.Data.Id)+`","name":"deepseek-account","runtime_class":"api_key","credential":{"api_key":"deepseek-secret"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/deepseek/v1/chat/completions", `{"model":"deepseek-alias-model","messages":[{"role":"user","content":"alias route"}]}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=deepseek-alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one deepseek alias decision, got %d", len(decisionsResp.Data))
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(deepseekProvider.Data.Id) {
		t.Fatalf("expected deepseek alias to force deepseek provider, got %+v", decision)
	}
	if decision.CandidateCount != 1 {
		t.Fatalf("expected deepseek alias to filter candidates to one provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/deepseek/v1/chat/completions" {
		t.Fatalf("expected deepseek alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayAnthropicProviderAliasTargetsMessagesUpstream(t *testing.T) {
	type upstreamCall struct {
		Path      string
		APIKey    string
		Version   string
		Model     string
		System    string
		MaxTokens int
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model     string `json:"model"`
			System    string `json:"system"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:      r.URL.Path,
			APIKey:    r.Header.Get("x-api-key"),
			Version:   r.Header.Get("anthropic-version"),
			Model:     payload.Model,
			System:    payload.System,
			MaxTokens: payload.MaxTokens,
			Messages:  payload.Messages,
		})
		mu.Unlock()
		content := "anthropic alias response"
		if len(payload.Messages) > 0 {
			content = "anthropic echo: " + payload.Messages[len(payload.Messages)-1].Content
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":` + strconv.Quote(content) + `}],"usage":{"input_tokens":4,"output_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	anthropicProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"anthropic-compatible","display_name":"Anthropic Compatible","adapter_type":"anthropic-compatible","protocol":"anthropic-compatible","status":"active"}`)
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"anthropic-fallback-provider","display_name":"Anthropic Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"anthropic-alias-model","display_name":"Anthropic Alias Model","status":"active","capabilities":[{"key":"streaming","level":"optional","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"anthropic-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(anthropicProvider.Data.Id)+`","upstream_model_name":"claude-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(anthropicProvider.Data.Id)+`","name":"anthropic-alias-account","runtime_class":"api_key","credential":{"api_key":"anthropic-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active","priority":10}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	messageRec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/anthropic-compatible/v1/messages", `{"model":"anthropic-alias-model","system":"respond briefly","max_tokens":32,"messages":[{"role":"user","content":"alias messages"}]}`)
	var messageResp apiopenapi.AnthropicMessagesResponse
	if err := json.NewDecoder(messageRec.Body).Decode(&messageResp); err != nil {
		t.Fatalf("decode messages response: %v", err)
	}
	if len(messageResp.Content) == 0 || messageResp.Content[0].Text == nil || !strings.Contains(*messageResp.Content[0].Text, "alias messages") {
		t.Fatalf("expected messages response from Anthropic upstream, got %v", messageResp.Content)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	call := gotCalls[0]
	if call.Path != "/v1/messages" || call.APIKey != "anthropic-secret" || call.Version == "" || call.Model != "claude-upstream" || call.System != "respond briefly" || call.MaxTokens != 32 {
		t.Fatalf("unexpected Anthropic upstream call: %+v", call)
	}
	if len(call.Messages) != 1 || call.Messages[0].Role != "user" || call.Messages[0].Content != "alias messages" {
		t.Fatalf("unexpected Anthropic upstream messages: %+v", call.Messages)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=anthropic-alias-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one Anthropic alias decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(anthropicProvider.Data.Id) || decision.TargetProtocol != "anthropic-compatible" || decision.SourceEndpoint != "/api/provider/anthropic-compatible/v1/messages" {
		t.Fatalf("expected Anthropic alias scheduler evidence, got %+v", decision)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=anthropic-alias-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || !usageResp.Data[0].Success || usageResp.Data[0].TargetProtocol == nil || *usageResp.Data[0].TargetProtocol != "anthropic-compatible" || usageResp.Data[0].TotalTokens != 9 {
		t.Fatalf("expected Anthropic usage evidence, got %+v", usageResp.Data)
	}
}

func TestGatewayEmbeddingsRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path          string
		Authorization string
		Model         string
		Input         []string
		Dimensions    *int
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model      string   `json:"model"`
			Input      []string `json:"input"`
			Dimensions *int     `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			Model:         payload.Model,
			Input:         append([]string(nil), payload.Input...),
			Dimensions:    payload.Dimensions,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.11,0.22,0.33],"index":0},{"object":"embedding","embedding":[0.44,0.55,0.66],"index":1}],"model":"embedding-upstream","usage":{"prompt_tokens":9,"total_tokens":9}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp270-openai","display_name":"WP270 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"embeddings":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp270-embedding-model","display_name":"WP270 Embedding Model","status":"active","capabilities":[{"key":"embeddings","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"embedding-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp270-embedding-account","runtime_class":"api_key","credential":{"api_key":"embedding-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/embeddings", `{"model":"wp270-embedding-model","input":["first embedding","second embedding"],"dimensions":3}`)
	var embeddingResp apiopenapi.EmbeddingResponse
	if err := json.NewDecoder(rec.Body).Decode(&embeddingResp); err != nil {
		t.Fatalf("decode embeddings response: %v", err)
	}
	if embeddingResp.Object != apiopenapi.EmbeddingResponseObjectList || embeddingResp.Model != "wp270-embedding-model" || len(embeddingResp.Data) != 2 {
		t.Fatalf("unexpected embeddings response: %+v", embeddingResp)
	}
	vector, err := embeddingResp.Data[0].Embedding.AsEmbeddingVector0()
	if err != nil || len(vector) != 3 {
		t.Fatalf("expected float embedding vector, got vector=%v err=%v", vector, err)
	}
	if embeddingResp.Usage.PromptTokens == nil || *embeddingResp.Usage.PromptTokens != 9 || embeddingResp.Usage.CompletionTokens == nil || *embeddingResp.Usage.CompletionTokens != 0 {
		t.Fatalf("expected upstream embedding usage, got %+v", embeddingResp.Usage)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/embeddings" || gotCalls[0].Authorization != "Bearer embedding-secret" || gotCalls[0].Model != "embedding-upstream" {
		t.Fatalf("unexpected upstream call: %+v", gotCalls[0])
	}
	if gotCalls[0].Dimensions == nil || *gotCalls[0].Dimensions != 3 || len(gotCalls[0].Input) != 2 {
		t.Fatalf("unexpected upstream input details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp270-embedding-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/embeddings" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) {
		t.Fatalf("unexpected embedding usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp270-embedding-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/embeddings" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected embedding decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayEmbeddingAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"embedding-fallback-provider","display_name":"Embedding Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"embeddings":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp270-alias-embedding-model","display_name":"WP270 Alias Embedding Model","status":"active","capabilities":[{"key":"embeddings","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-embedding","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"embedding-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-embedding","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/openai-compatible/v1/embeddings", `{"model":"wp270-alias-embedding-model","input":"alias embedding"}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp270-alias-embedding-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias embedding decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected embedding alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/embeddings" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayImageGenerationRouteTargetsOpenAICompatibleUpstream(t *testing.T) {
	type upstreamCall struct {
		Path           string
		Authorization  string
		Model          string
		Prompt         string
		Count          int
		Size           string
		Quality        string
		Style          string
		ResponseFormat string
	}
	var (
		mu    sync.Mutex
		calls []upstreamCall
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model          string `json:"model"`
			Prompt         string `json:"prompt"`
			N              int    `json:"n"`
			Size           string `json:"size"`
			Quality        string `json:"quality"`
			Style          string `json:"style"`
			ResponseFormat string `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		mu.Lock()
		calls = append(calls, upstreamCall{
			Path:           r.URL.Path,
			Authorization:  r.Header.Get("Authorization"),
			Model:          payload.Model,
			Prompt:         payload.Prompt,
			Count:          payload.N,
			Size:           payload.Size,
			Quality:        payload.Quality,
			Style:          payload.Style,
			ResponseFormat: payload.ResponseFormat,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":1710000001,"data":[{"url":"https://example.test/wp290-image.png","revised_prompt":"polished image prompt"}],"model":"image-upstream","usage":{"prompt_tokens":12,"completion_tokens":1,"total_tokens":13}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp290-openai","display_name":"WP290 OpenAI","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp290-image-model","display_name":"WP290 Image Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"image-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp290-image-account","runtime_class":"api_key","credential":{"api_key":"image-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/images/generations", `{"model":"wp290-image-model","prompt":"make a clean test image","n":1,"size":"1024x1024","quality":"high","style":"vivid","response_format":"url"}`)
	var imageResp apiopenapi.ImageGenerationResponse
	if err := json.NewDecoder(rec.Body).Decode(&imageResp); err != nil {
		t.Fatalf("decode image response: %v", err)
	}
	if imageResp.Created != 1710000001 || len(imageResp.Data) != 1 || imageResp.Data[0].Url == nil || *imageResp.Data[0].Url != "https://example.test/wp290-image.png" {
		t.Fatalf("unexpected image response: %+v", imageResp)
	}

	mu.Lock()
	gotCalls := append([]upstreamCall(nil), calls...)
	mu.Unlock()
	if len(gotCalls) != 1 {
		t.Fatalf("expected one upstream call, got %+v", gotCalls)
	}
	if gotCalls[0].Path != "/v1/images/generations" || gotCalls[0].Authorization != "Bearer image-secret" || gotCalls[0].Model != "image-upstream" {
		t.Fatalf("unexpected upstream image call: %+v", gotCalls[0])
	}
	if gotCalls[0].Prompt != "make a clean test image" || gotCalls[0].Count != 1 || gotCalls[0].Size != "1024x1024" || gotCalls[0].Quality != "high" || gotCalls[0].Style != "vivid" || gotCalls[0].ResponseFormat != "url" {
		t.Fatalf("unexpected upstream image details: %+v", gotCalls[0])
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=wp290-image-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 {
		t.Fatalf("expected one image usage record, got %+v", usageResp.Data)
	}
	usage := usageResp.Data[0]
	if !usage.Success || usage.SourceEndpoint != "/v1/images/generations" || usage.ProviderId == nil || *usage.ProviderId != string(providerResp.Data.Id) || usage.AccountId == nil || *usage.AccountId != string(accountResp.Data.Id) || usage.TotalTokens != 13 {
		t.Fatalf("unexpected image usage evidence: %+v", usage)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp290-image-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 || decisionsResp.Data[0].SourceEndpoint != "/v1/images/generations" || decisionsResp.Data[0].CandidateCount != 1 {
		t.Fatalf("unexpected image decision evidence: %+v", decisionsResp.Data)
	}
}

func TestGatewayImageGenerationAliasForcesProviderContext(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	openaiProvider := mustFindProviderByName(t, handler, sessionCookie, "openai-compatible")
	fallbackProvider := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"image-fallback-provider","display_name":"Image Fallback","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active","capabilities":{"images":true}}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp290-alias-image-model","display_name":"WP290 Alias Image Model","status":"active","capabilities":[{"key":"images","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","upstream_model_name":"fallback-image","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(fallbackProvider.Data.Id)+`","name":"image-fallback-account","runtime_class":"api_key","credential":{"api_key":"fallback-secret"},"status":"active","priority":100}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(openaiProvider.Id)+`","upstream_model_name":"alias-image","status":"active"}`)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/api/provider/openai-compatible/v1/images/generations", `{"model":"wp290-alias-image-model","prompt":"alias image prompt"}`)

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp290-alias-image-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one alias image decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.SelectedProviderId == nil || *decision.SelectedProviderId != string(openaiProvider.Id) || decision.CandidateCount != 1 {
		t.Fatalf("expected image alias to force openai-compatible provider, got %+v", decision)
	}
	if decision.SourceEndpoint != "/api/provider/openai-compatible/v1/images/generations" {
		t.Fatalf("expected alias source endpoint, got %q", decision.SourceEndpoint)
	}
}

func TestGatewayModelAliasAndSessionAffinityFeedScheduler(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"wp140-provider","display_name":"WP140 Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"wp140-canonical","display_name":"WP140 Canonical","status":"active"}`)
	createAliasReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/aliases", strings.NewReader(`{"alias":"wp140-cli-alias","fallback_models":["wp140-fallback"],"strategy_hint":"cost_saver","status":"active"}`))
	createAliasReq.Header.Set("Content-Type", "application/json")
	createAliasReq.AddCookie(sessionCookie)
	createAliasReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAliasRec := httptest.NewRecorder()
	handler.ServeHTTP(createAliasRec, createAliasReq)
	if createAliasRec.Code != http.StatusCreated {
		t.Fatalf("expected model alias create 201, got %d body=%s", createAliasRec.Code, createAliasRec.Body.String())
	}

	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"wp140-upstream","status":"active"}`)
	firstAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp140-first-account","runtime_class":"api_key","credential":{"api_key":"first-secret"},"metadata":{"health_score":0.9},"status":"active"}`)
	stickyAccount := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"wp140-sticky-account","runtime_class":"api_key","credential":{"api_key":"sticky-secret"},"metadata":{"health_score":0.9,"session_affinity_key":"thread-wp140"},"status":"active"}`)

	keyReq := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"gateway-alias","scopes":["gateway:invoke"],"allowed_models":["wp140-cli-alias"]}`))
	keyReq.Header.Set("Content-Type", "application/json")
	keyReq.AddCookie(sessionCookie)
	keyReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	keyRec := httptest.NewRecorder()
	handler.ServeHTTP(keyRec, keyReq)
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("expected api key create 201, got %d body=%s", keyRec.Code, keyRec.Body.String())
	}
	var keyResp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(keyRec.Body).Decode(&keyResp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"wp140-cli-alias","messages":[{"role":"user","content":"route with affinity"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+keyResp.Data.PlaintextKey)
	chatReq.Header.Set("X-SRapi-Session-Affinity-Key", "thread-wp140")
	chatReq.Header.Set("X-SRapi-Sticky-Strength", "hard")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected alias affinity chat 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=wp140-canonical", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d body=%s", decisionsRec.Code, decisionsRec.Body.String())
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one wp140 decision, got %+v", decisionsResp.Data)
	}
	decision := decisionsResp.Data[0]
	if decision.Strategy != apiopenapi.SchedulerDecisionStrategyCostSaver {
		t.Fatalf("expected alias strategy cost_saver, got %+v", decision)
	}
	if decision.SelectedAccountId == nil || *decision.SelectedAccountId != string(stickyAccount.Data.Id) {
		t.Fatalf("expected sticky account %s selected, got %+v", stickyAccount.Data.Id, decision.SelectedAccountId)
	}
	if !decision.StickyHit || !jsonObjectContainsString(decision.RejectReasons, "hard_sticky_mismatch") {
		t.Fatalf("expected hard sticky evidence, got decision=%+v first=%+v", decision, firstAccount.Data.Id)
	}
	hints, ok := decision.Scores["routing_hints"].(map[string]any)
	if !ok {
		t.Fatalf("expected routing hints, got %+v", decision.Scores)
	}
	if hints["model_alias"] != "wp140-cli-alias" || hints["sticky_strength"] != "hard" || hints["session_affinity_source"] != "header:x-srapi-session-affinity-key" {
		t.Fatalf("unexpected routing hints: %+v", hints)
	}
	if keyHash, ok := hints["session_affinity_key_hash"].(string); !ok || !strings.HasPrefix(keyHash, "sha256:") || strings.Contains(keyHash, "thread-wp140") {
		t.Fatalf("expected hashed affinity key, got %+v", hints)
	}
}

func TestGatewayRateLimitFeedbackAppliesAccountCooldown(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit","message":"slow down"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"rate-limit-provider","display_name":"Rate Limit Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.AddCookie(sessionCookie)
	providerReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", providerRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	modelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"rate-limit-model","display_name":"Rate Limit Model","status":"active"}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.AddCookie(sessionCookie)
	modelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", modelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	mappingBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","upstream_model_name":"rate-limit-upstream","status":"active"}`
	mappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(mappingBody))
	mappingReq.Header.Set("Content-Type", "application/json")
	mappingReq.AddCookie(sessionCookie)
	mappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	mappingRec := httptest.NewRecorder()
	handler.ServeHTTP(mappingRec, mappingReq)
	if mappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", mappingRec.Code)
	}

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"rate-limit-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"` + upstream.URL + `/v1"},"status":"active"}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")
	accountReq.AddCookie(sessionCookie)
	accountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", accountRec.Code)
	}
	var accountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(accountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"rate-limit-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected first rate limit 429, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts: %v", err)
	}
	cooldownAccount := findProviderAccountByID(accountsResp.Data, accountResp.Data.Id)
	if cooldownAccount == nil || cooldownAccount.Metadata == nil || (*cooldownAccount.Metadata)["cooldown_active"] != true || (*cooldownAccount.Metadata)["cooldown_reason"] != "rate_limit" {
		t.Fatalf("expected account cooldown metadata, got %+v", cooldownAccount)
	}

	healthReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts/"+string(accountResp.Data.Id)+"/health", nil)
	healthReq.AddCookie(sessionCookie)
	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("expected account health 200, got %d body=%s", healthRec.Code, healthRec.Body.String())
	}
	var healthResp apiopenapi.AccountHealthResponse
	if err := json.NewDecoder(healthRec.Body).Decode(&healthResp); err != nil {
		t.Fatalf("decode account health: %v", err)
	}
	if healthResp.Data.RuntimeClass != apiopenapi.RuntimeClassApiKey ||
		healthResp.Data.ErrorClass == nil ||
		*healthResp.Data.ErrorClass != "rate_limit" ||
		healthResp.Data.CooldownReason == nil ||
		*healthResp.Data.CooldownReason != "rate_limit" ||
		healthResp.Data.CooldownUntil == nil ||
		healthResp.Data.QuotaExhausted ||
		healthResp.Data.QuotaRemainingRatio != 1 {
		t.Fatalf("unexpected account health diagnostics: %+v", healthResp.Data)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"rate-limit-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second request blocked by cooldown 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected cooldown to prevent second upstream call, got %d upstream calls", upstreamCalls)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=rate-limit-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 2 || !jsonObjectContainsString(decisionsResp.Data[1].RejectReasons, "cooldown_active") {
		t.Fatalf("expected second decision cooldown_active, got %+v", decisionsResp.Data)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=rate-limit-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 2 || usageResp.Data[0].ErrorClass == nil || *usageResp.Data[0].ErrorClass != "rate_limit" {
		t.Fatalf("expected rate_limit usage followed by cooldown failure, got %+v", usageResp.Data)
	}
}

func TestGatewaySchedulerRejectsSaturatedAccount(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"limited-provider","display_name":"Limited Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"limited-model","display_name":"Limited Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(createModelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	createMappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"limited-upstream","status":"active"}`))
	createMappingReq.Header.Set("Content-Type", "application/json")
	createMappingReq.AddCookie(sessionCookie)
	createMappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createMappingRec := httptest.NewRecorder()
	handler.ServeHTTP(createMappingRec, createMappingReq)
	if createMappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", createMappingRec.Code)
	}

	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","name":"saturated-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active","metadata":{"max_concurrency":1,"current_concurrency":1}}`))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.AddCookie(sessionCookie)
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", createAccountRec.Code)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"limited-model","messages":[{"role":"user","content":"hello"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected saturated account 503, got %d", chatRec.Code)
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=limited-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one limited-model decision, got %d", len(decisionsResp.Data))
	}
	reasons := decisionsResp.Data[0].RejectReasons
	found := false
	for _, reason := range reasons {
		if reason == "concurrency_full" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected concurrency_full reject reason, got %+v", reasons)
	}
}

func TestGatewayVisionRequestUsesCapabilityTaxonomy(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"text-only-provider","display_name":"Text Only Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`))
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderReq.AddCookie(sessionCookie)
	createProviderReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", createProviderRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	createModelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"text-only-model","display_name":"Text Only Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`))
	createModelReq.Header.Set("Content-Type", "application/json")
	createModelReq.AddCookie(sessionCookie)
	createModelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createModelRec, createModelReq)
	if createModelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", createModelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(createModelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	createMappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"text-only-upstream","status":"active"}`))
	createMappingReq.Header.Set("Content-Type", "application/json")
	createMappingReq.AddCookie(sessionCookie)
	createMappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createMappingRec := httptest.NewRecorder()
	handler.ServeHTTP(createMappingRec, createMappingReq)
	if createMappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", createMappingRec.Code)
	}

	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","name":"text-only-account","runtime_class":"api_key","credential":{"api_key":"secret"},"status":"active"}`))
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountReq.AddCookie(sessionCookie)
	createAccountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", createAccountRec.Code)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"text-only-model","messages":[{"role":"user","content":[{"type":"text","text":"describe this"},{"type":"image_url","image_url":{"url":"https://example.invalid/image.png"}}]}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected vision capability mismatch 503, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	decisionsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/decisions?model=text-only-model", nil)
	decisionsReq.AddCookie(sessionCookie)
	decisionsRec := httptest.NewRecorder()
	handler.ServeHTTP(decisionsRec, decisionsReq)
	if decisionsRec.Code != http.StatusOK {
		t.Fatalf("expected decisions 200, got %d", decisionsRec.Code)
	}
	var decisionsResp apiopenapi.SchedulerDecisionListResponse
	if err := json.NewDecoder(decisionsRec.Body).Decode(&decisionsResp); err != nil {
		t.Fatalf("decode decisions: %v", err)
	}
	if len(decisionsResp.Data) != 1 {
		t.Fatalf("expected one text-only-model decision, got %d", len(decisionsResp.Data))
	}
	if !jsonObjectContainsString(decisionsResp.Data[0].RejectReasons, "capability_mismatch") {
		t.Fatalf("expected capability_mismatch reject reason, got %+v", decisionsResp.Data[0].RejectReasons)
	}
	if !stringSliceContains(decisionsResp.Data[0].CompatibilityWarnings, "vision_ignored") {
		t.Fatalf("expected vision compatibility warning, got %+v", decisionsResp.Data[0].CompatibilityWarnings)
	}

	usageReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage-logs?model=text-only-model", nil)
	usageReq.AddCookie(sessionCookie)
	usageRec := httptest.NewRecorder()
	handler.ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusOK {
		t.Fatalf("expected usage logs 200, got %d", usageRec.Code)
	}
	var usageResp apiopenapi.UsageLogListResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("decode usage logs: %v", err)
	}
	if len(usageResp.Data) != 1 || usageResp.Data[0].Success || usageResp.Data[0].ErrorClass == nil || *usageResp.Data[0].ErrorClass != "no_available_account" {
		t.Fatalf("expected failed no_available_account usage log, got %+v", usageResp.Data)
	}
}

func TestGatewayReverseProxyAccountAutoProtectsOnSessionInvalid(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer oauth-access" {
			t.Fatalf("expected reverse proxy bearer token, got %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Request-ID") != "" || strings.Contains(r.Header.Get("User-Agent"), "SRapi") {
			t.Fatalf("unexpected SRapi header leakage: %+v", r.Header)
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"session_invalid","message":"session expired"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(`{"name":"reverse-provider","display_name":"Reverse Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`))
	providerReq.Header.Set("Content-Type", "application/json")
	providerReq.AddCookie(sessionCookie)
	providerReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d", providerRec.Code)
	}
	var providerResp apiopenapi.ProviderResponse
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	modelReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(`{"canonical_name":"reverse-model","display_name":"Reverse Model","status":"active"}`))
	modelReq.Header.Set("Content-Type", "application/json")
	modelReq.AddCookie(sessionCookie)
	modelReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d", modelRec.Code)
	}
	var modelResp apiopenapi.ModelResponse
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}

	mappingReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+string(modelResp.Data.Id)+"/mappings", strings.NewReader(`{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"reverse-upstream","status":"active"}`))
	mappingReq.Header.Set("Content-Type", "application/json")
	mappingReq.AddCookie(sessionCookie)
	mappingReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	mappingRec := httptest.NewRecorder()
	handler.ServeHTTP(mappingRec, mappingReq)
	if mappingRec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d", mappingRec.Code)
	}

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"reverse-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"oauth-access","refresh_token":"refresh-token"},"metadata":{"base_url":"` + upstream.URL + `/v1","user_agent":"Codex/1.0"},"status":"active"}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(accountBody))
	accountReq.Header.Set("Content-Type", "application/json")
	accountReq.AddCookie(sessionCookie)
	accountReq.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d", accountRec.Code)
	}
	var accountResp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(accountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"reverse-model","messages":[{"role":"user","content":"call reverse"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatReq.Header.Set("X-Request-ID", "req_reverse_gateway")
	chatReq.Header.Set("X-SRapi-Test", "must-not-forward")
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusBadGateway {
		t.Fatalf("expected reverse proxy gateway failure 502, got %d", chatRec.Code)
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?status=needs_reauth", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d", accountsRec.Code)
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}
	if len(accountsResp.Data) != 1 || accountsResp.Data[0].Id != accountResp.Data.Id {
		t.Fatalf("expected reverse account needs_reauth, got %+v", accountsResp.Data)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected metrics 200, got %d", metricsRec.Code)
	}
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_request_error_total{error_class="session_invalid"} 1`) {
		t.Fatalf("expected reverse proxy session_invalid metric, got:\n%s", metricsRec.Body.String())
	}
}

func TestGatewayReverseProxyBanSignalDisablesAccountAndStopsScheduling(t *testing.T) {
	upstreamCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"account_banned","message":"account banned"}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"banned-provider","display_name":"Banned Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"banned-model","display_name":"Banned Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"banned-upstream","status":"active"}`)
	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"banned-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"oauth-access","refresh_token":"refresh-token"},"metadata":{"base_url":"` + upstream.URL + `/v1","user_agent":"Codex/1.0"},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	firstReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"banned-model","messages":[{"role":"user","content":"first"}]}`))
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway {
		t.Fatalf("expected first banned request 502, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/accounts?status=disabled", nil)
	accountsReq.AddCookie(sessionCookie)
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("expected accounts list 200, got %d body=%s", accountsRec.Code, accountsRec.Body.String())
	}
	var accountsResp apiopenapi.ProviderAccountListResponse
	if err := json.NewDecoder(accountsRec.Body).Decode(&accountsResp); err != nil {
		t.Fatalf("decode accounts response: %v", err)
	}
	if len(accountsResp.Data) != 1 || accountsResp.Data[0].Id != accountResp.Data.Id {
		t.Fatalf("expected banned account disabled, got %+v", accountsResp.Data)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"banned-model","messages":[{"role":"user","content":"second"}]}`))
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected second banned request 503, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected disabled account to stop further upstream calls, got %d", upstreamCalls)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_account_banned_total 1`) {
		t.Fatalf("expected reverse proxy banned metric, got:\n%s", metricsRec.Body.String())
	}
}

func TestGatewayReverseProxyOAuthRefreshPersistsCredentialAndAudits(t *testing.T) {
	var gotAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"refreshed ok"}}],"usage":{"input_tokens":2,"output_tokens":3}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"refresh-provider","display_name":"Refresh Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"refresh-model","display_name":"Refresh Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"refresh-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"refresh-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"expired-token","refresh_token":"fresh-token","expires_at":"2000-01-01T00:00:00Z"},"metadata":{"base_url":"` + upstream.URL + `/v1","user_agent":"Codex/1.0"},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"refresh-model","messages":[{"role":"user","content":"call refresh"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusOK {
		t.Fatalf("expected refreshed gateway success 200, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}
	if gotAuthorization != "Bearer fresh-token" {
		t.Fatalf("expected refreshed bearer token, got %q", gotAuthorization)
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	refreshAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.oauth_refresh")
	if refreshAudit.ResourceId != string(accountResp.Data.Id) || refreshAudit.After["refresh_status"] != "success" {
		t.Fatalf("unexpected refresh audit: %+v", refreshAudit)
	}
	if _, ok := refreshAudit.After["access_token"]; ok {
		t.Fatalf("refresh audit leaked access token: %+v", refreshAudit.After)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_oauth_refresh_total{status="success"} 1`) {
		t.Fatalf("expected reverse proxy refresh success metric, got:\n%s", metricsRec.Body.String())
	}
}

func TestGatewayReverseProxyOAuthRefreshFailureDoesNotPersistCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream should not be called when refresh credential is missing")
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"refresh-fail-provider","display_name":"Refresh Fail Provider","adapter_type":"reverse-proxy-codex-cli","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"refresh-fail-model","display_name":"Refresh Fail Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"refresh-fail-upstream","status":"active"}`)

	accountBody := `{"provider_id":"` + string(providerResp.Data.Id) + `","name":"refresh-fail-account","runtime_class":"oauth_refresh","upstream_client":"codex_cli","credential":{"access_token":"expired-token","expires_at":"2000-01-01T00:00:00Z"},"metadata":{"base_url":"` + upstream.URL + `/v1","user_agent":"Codex/1.0"},"status":"active"}`
	accountResp := mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, accountBody)

	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"refresh-fail-model","messages":[{"role":"user","content":"call refresh"}]}`))
	chatReq.Header.Set("Content-Type", "application/json")
	chatReq.Header.Set("Authorization", "Bearer "+apiKey)
	chatRec := httptest.NewRecorder()
	handler.ServeHTTP(chatRec, chatReq)
	if chatRec.Code != http.StatusBadGateway {
		t.Fatalf("expected refresh failure 502, got %d body=%s", chatRec.Code, chatRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/audit-logs", nil)
	auditReq.AddCookie(sessionCookie)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit logs 200, got %d", auditRec.Code)
	}
	var auditResp apiopenapi.AuditLogListResponse
	if err := json.NewDecoder(auditRec.Body).Decode(&auditResp); err != nil {
		t.Fatalf("decode audit logs: %v", err)
	}
	failedAudit := mustFindAuditLog(t, auditResp.Data, "provider_account.oauth_refresh_failed")
	if failedAudit.ResourceId != string(accountResp.Data.Id) || failedAudit.After["refresh_status"] != "failed" {
		t.Fatalf("unexpected refresh failure audit: %+v", failedAudit)
	}
	if auditLogHasAction(auditResp.Data, "provider_account.oauth_refresh") {
		t.Fatalf("did not expect successful refresh audit in %+v", auditResp.Data)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), `reverse_proxy_oauth_refresh_total{status="credential_missing"} 1`) {
		t.Fatalf("expected reverse proxy refresh missing credential metric, got:\n%s", metricsRec.Body.String())
	}
}

func mustLoginAdmin(t *testing.T, handler http.Handler) (apiopenapi.LoginResponse, *http.Cookie) {
	t.Helper()
	loginBody := `{"email":"admin@srapi.local","password":"password123"}`
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login 200, got %d", loginRec.Code)
	}
	var loginResp apiopenapi.LoginResponse
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	return loginResp, cookies[0]
}

func mustCreateGatewayAPIKey(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string) (apiopenapi.CreateApiKeyResponse, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/api-keys", strings.NewReader(`{"name":"gateway","scopes":["gateway:invoke"]}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected api key create 201, got %d", rec.Code)
	}
	var resp apiopenapi.CreateApiKeyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode api key response: %v", err)
	}
	return resp, resp.Data.PlaintextKey
}

func mustGatewayRequest(t *testing.T, handler http.Handler, apiKey, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s %s 200, got %d body=%s", method, path, rec.Code, rec.Body.String())
	}
	return rec
}

func seedUsageLog(t *testing.T, store usagecontract.Store, input usagecontract.UsageLog) {
	t.Helper()
	if input.UserID == 0 {
		input.UserID = 1
	}
	if input.APIKeyID == 0 {
		input.APIKeyID = 1
	}
	if input.SourceProtocol == "" {
		input.SourceProtocol = "openai-compatible"
	}
	if input.TargetProtocol == "" {
		input.TargetProtocol = "openai-compatible"
	}
	if input.Cost == "" {
		input.Cost = "0.00000000"
	}
	if input.Currency == "" {
		input.Currency = "USD"
	}
	if _, err := store.Create(t.Context(), input); err != nil {
		t.Fatalf("seed usage log: %v", err)
	}
}

func mustCreateProvider(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ProviderResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/providers", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	return resp
}

func mustFindProviderByName(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, name string) apiopenapi.Provider {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/providers", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provider list 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider list: %v", err)
	}
	for _, provider := range resp.Data {
		if provider.Name == name {
			return provider
		}
	}
	t.Fatalf("provider %s not found in %+v", name, resp.Data)
	return apiopenapi.Provider{}
}

func mustCreateModel(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ModelResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}
	return resp
}

func mustCreateMapping(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, modelID, body string) apiopenapi.ModelProviderMappingResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/models/"+modelID+"/mappings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected mapping create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ModelProviderMappingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode mapping response: %v", err)
	}
	return resp
}

func mustCreateAccount(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.ProviderAccountResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.ProviderAccountResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account response: %v", err)
	}
	return resp
}

type upstreamGeminiCall struct {
	Path          string
	Authorization string
	Model         string
	Messages      []upstreamMessage
	MaxTokens     int
}

type upstreamNativeGeminiCall struct {
	Path     string
	APIKey   string
	Contents []struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	SystemText string
	MaxTokens  int
}

func geminiSystemInstructionText(value *struct {
	Parts []struct {
		Text string `json:"text"`
	} `json:"parts"`
}) string {
	if value == nil || len(value.Parts) == 0 {
		return ""
	}
	return value.Parts[0].Text
}

func mustCreateGeminiGatewayTarget(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, upstreamURL string, unique bool) (apiopenapi.ProviderResponse, apiopenapi.ModelResponse, apiopenapi.ProviderAccountResponse) {
	t.Helper()
	suffix := ""
	if unique {
		suffix = "-error"
	}
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"gemini-route-provider`+suffix+`","display_name":"Gemini Route Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"gemini-route-model`+suffix+`","display_name":"Gemini Route Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-route-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"gemini-route-account`+suffix+`","runtime_class":"api_key","credential":{"api_key":"gemini-route-upstream-secret"},"metadata":{"base_url":"`+upstreamURL+`/v1"},"status":"active"}`)
	return providerResp, modelResp, accountResp
}

func mustCreateNativeGeminiGatewayTarget(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, upstreamURL string) (apiopenapi.ProviderResponse, apiopenapi.ModelResponse, apiopenapi.ProviderAccountResponse) {
	t.Helper()
	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"native-gemini-route-provider","display_name":"Native Gemini Route Provider","adapter_type":"gemini-compatible","protocol":"gemini-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"native-gemini-route-model","display_name":"Native Gemini Route Model","status":"active","capabilities":[{"key":"streaming","level":"required","status":"stable","version":"v1"}]}`)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"gemini-native-upstream","status":"active"}`)
	accountResp := mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"native-gemini-route-account","runtime_class":"api_key","credential":{"api_key":"native-gemini-secret"},"metadata":{"base_url":"`+upstreamURL+`/v1beta"},"status":"active"}`)
	return providerResp, modelResp, accountResp
}

func mustCreateAccountGroup(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, body string) apiopenapi.AccountGroupResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/account-groups", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(sessionCookie)
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected account group create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.AccountGroupResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode account group response: %v", err)
	}
	return resp
}

func decodeChatMessageText(t *testing.T, content apiopenapi.ChatMessage_Content) string {
	t.Helper()
	text, err := content.AsChatMessageContent0()
	if err == nil {
		return text
	}
	blocks, err := content.AsChatMessageContent1()
	if err != nil || len(blocks) == 0 || blocks[0].Text == nil {
		t.Fatalf("unexpected chat message content: %v %v", blocks, err)
	}
	return *blocks[0].Text
}

func assertAdminListContains(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, path string, countFn func([]byte) int, expected int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected %s 200, got %d", path, rec.Code)
	}
	if got := countFn(rec.Body.Bytes()); got != expected {
		t.Fatalf("expected %s count %d, got %d", path, expected, got)
	}
	_ = csrfToken
}

func auditLogHasAction(items []apiopenapi.AuditLog, action string) bool {
	for _, item := range items {
		if item.Action == action {
			return true
		}
	}
	return false
}

func auditContractLogHasAction(items []auditcontract.Log, action string) bool {
	for _, item := range items {
		if item.Action == action {
			return true
		}
	}
	return false
}

func mustFindAuditLog(t *testing.T, items []apiopenapi.AuditLog, action string) apiopenapi.AuditLog {
	t.Helper()
	for _, item := range items {
		if item.Action == action {
			return item
		}
	}
	t.Fatalf("audit action %s not found in %+v", action, items)
	return apiopenapi.AuditLog{}
}

func findProviderAccountByID(items []apiopenapi.ProviderAccount, id apiopenapi.Id) *apiopenapi.ProviderAccount {
	for i := range items {
		if items[i].Id == id {
			return &items[i]
		}
	}
	return nil
}

func findProviderAccountExportByName(items []apiopenapi.ProviderAccountExportItem, name string) *apiopenapi.ProviderAccountExportItem {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func jsonObjectContainsString(value apiopenapi.JsonObject, target string) bool {
	for _, item := range value {
		switch item := item.(type) {
		case string:
			if item == target {
				return true
			}
		case []any:
			for _, nested := range item {
				if nested == target {
					return true
				}
			}
		case []string:
			for _, nested := range item {
				if nested == target {
					return true
				}
			}
		}
	}
	return false
}
