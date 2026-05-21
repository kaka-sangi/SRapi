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

	"github.com/srapi/srapi/apps/api/internal/config"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

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
		{http.MethodPatch, "/api/v1/admin/accounts/" + string(accountResp.Data.Id), `{"name":"blocked-account"}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/test", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/disable", `{}`},
		{http.MethodPost, "/api/v1/admin/accounts/" + string(accountResp.Data.Id) + "/enable", `{}`},
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
	if len(accountQuotaResp.Data) != 1 || accountQuotaResp.Data[0].QuotaType != "monthly_tokens" || accountQuotaResp.Data[0].Used == "0" {
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
	type upstreamMessage struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
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
