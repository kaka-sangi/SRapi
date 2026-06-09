package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/srapi/srapi/apps/api/internal/config"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeymemory "github.com/srapi/srapi/apps/api/internal/modules/api_keys/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

func mustCreateOpenAIChatGatewayTarget(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, providerName, modelName string) apiopenapi.ProviderResponse {
	t.Helper()
	providerResp, _ := mustCreateOpenAIChatGatewayTargetWithModel(t, handler, sessionCookie, csrfToken, providerName, modelName)
	return providerResp
}

func mustCreateOpenAIChatGatewayTargetWithModel(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken, providerName, modelName string) (apiopenapi.ProviderResponse, apiopenapi.ModelResponse) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","index":0,"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(upstream.Close)

	providerResp := mustCreateProvider(t, handler, sessionCookie, csrfToken, `{"name":"`+providerName+`","display_name":"`+providerName+`","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, csrfToken, `{"canonical_name":"`+modelName+`","display_name":"`+modelName+`","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, csrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"`+modelName+`-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, csrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"`+providerName+`-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	return providerResp, modelResp
}

func mustCreatePricingRule(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, body string) apiopenapi.PricingRuleResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing-rules", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeaderName, csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create pricing rule failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp apiopenapi.PricingRuleResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode pricing rule response: %v", err)
	}
	return resp
}

func TestGatewayKeyIPAllowed(t *testing.T) {
	cases := []struct {
		name    string
		key     apikeycontract.APIKey
		ip      string
		allowed bool
	}{
		{"no lists allows all", apikeycontract.APIKey{}, "1.2.3.4", true},
		{"allowlist exact match", apikeycontract.APIKey{AllowedIPs: []string{"1.2.3.4"}}, "1.2.3.4", true},
		{"allowlist miss denies", apikeycontract.APIKey{AllowedIPs: []string{"1.2.3.4"}}, "1.2.3.5", false},
		{"allowlist CIDR match", apikeycontract.APIKey{AllowedIPs: []string{"10.0.0.0/8"}}, "10.9.9.9", true},
		{"allowlist CIDR miss", apikeycontract.APIKey{AllowedIPs: []string{"10.0.0.0/8"}}, "11.0.0.1", false},
		{"denylist match denies", apikeycontract.APIKey{DeniedIPs: []string{"1.2.3.4"}}, "1.2.3.4", false},
		{"denylist precedence over allow", apikeycontract.APIKey{AllowedIPs: []string{"1.2.3.4"}, DeniedIPs: []string{"1.2.3.4"}}, "1.2.3.4", false},
		{"unparseable ip with allowlist denies", apikeycontract.APIKey{AllowedIPs: []string{"1.2.3.4"}}, "", false},
		{"unparseable ip without allowlist allows", apikeycontract.APIKey{DeniedIPs: []string{"1.2.3.4"}}, "", true},
		{"ipv6 cidr match", apikeycontract.APIKey{AllowedIPs: []string{"2001:db8::/32"}}, "2001:db8::1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := gatewayKeyIPAllowed(tc.key, tc.ip)
			if tc.allowed && err != nil {
				t.Fatalf("expected allowed, got %v", err)
			}
			if !tc.allowed && err == nil {
				t.Fatalf("expected denied, got nil")
			}
		})
	}
}

func TestGatewayEnforcesAPIKeyIPAllowlist(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateOpenAIChatGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, "ip-scoped-provider", "ip-scoped-model")
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"ip-scoped-gateway","scopes":["gateway:invoke"],"allowed_ips":["10.0.0.0/8"]}`)
	apiKey := keyResp.Data.PlaintextKey

	// Client IP outside the allow list is rejected before scheduling.
	denied := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"ip-scoped-model","messages":[{"role":"user","content":"blocked"}]}`))
	denied.Header.Set("Content-Type", "application/json")
	denied.Header.Set("Authorization", "Bearer "+apiKey)
	denied.Header.Set("X-Forwarded-For", "203.0.113.7")
	deniedRec := httptest.NewRecorder()
	handler.ServeHTTP(deniedRec, denied)
	if deniedRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disallowed IP, got %d body=%s", deniedRec.Code, deniedRec.Body.String())
	}
	var errResp apiopenapi.GatewayErrorResponse
	if err := json.NewDecoder(deniedRec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode ip error response: %v", err)
	}
	if errResp.Error.Code == nil || *errResp.Error.Code != "ip_not_allowed" || errResp.Error.Type != apiopenapi.PermissionError {
		t.Fatalf("unexpected ip error response: %+v", errResp)
	}

	// Client IP inside the allow list is permitted.
	allowed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"ip-scoped-model","messages":[{"role":"user","content":"permitted"}]}`))
	allowed.Header.Set("Content-Type", "application/json")
	allowed.Header.Set("Authorization", "Bearer "+apiKey)
	allowed.Header.Set("X-Forwarded-For", "10.1.2.3")
	allowedRec := httptest.NewRecorder()
	handler.ServeHTTP(allowedRec, allowed)
	if allowedRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed IP, got %d body=%s", allowedRec.Code, allowedRec.Body.String())
	}
}

func TestGatewayEnforcesAPIKeyWindowLimit(t *testing.T) {
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	defer redisClient.Close()
	limiter, err := ratelimit.New(redisClient)
	if err != nil {
		t.Fatalf("new rate limiter: %v", err)
	}

	handler := New(config.Load(), nil, WithRateLimiter(limiter))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustCreateOpenAIChatGatewayTarget(t, handler, sessionCookie, loginResp.Data.CsrfToken, "window-limited-provider", "window-limited-model")
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"window-limited-gateway","scopes":["gateway:invoke"],"request_limit_5h":1}`)
	apiKey := keyResp.Data.PlaintextKey

	first := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"window-limited-model","messages":[{"role":"user","content":"first windowed request"}]}`))
	first.Header.Set("Content-Type", "application/json")
	first.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first windowed request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"window-limited-model","messages":[{"role":"user","content":"second windowed request"}]}`))
	second.Header.Set("Content-Type", "application/json")
	second.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second windowed request 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
}

func TestGatewayEnforcesAPIKeyCostQuota(t *testing.T) {
	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, modelResp := mustCreateOpenAIChatGatewayTargetWithModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, "cost-quota-provider", "cost-quota-model")
	mustCreatePricingRule(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"0","input_price_per_million_tokens":"10000","output_price_per_million_tokens":"10000","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"USD"}`)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"cost-quota-gateway","scopes":["gateway:invoke"],"cost_quota":"0.01000000"}`)
	apiKey := keyResp.Data.PlaintextKey

	first := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"cost-quota-model","messages":[{"role":"user","content":"first paid request with enough tokens"}]}`))
	first.Header.Set("Content-Type", "application/json")
	first.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected over-quota request 429, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}
}

func TestGatewayEnforcesAPIKeyCostWindowLimit(t *testing.T) {
	keyStore := apikeymemory.New()
	handler := New(config.Load(), nil, WithAPIKeyStore(keyStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	_, modelResp := mustCreateOpenAIChatGatewayTargetWithModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, "cost-window-provider", "cost-window-model")
	mustCreatePricingRule(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"model_id":"`+string(modelResp.Data.Id)+`","provider_id":"0","input_price_per_million_tokens":"1","output_price_per_million_tokens":"1","cache_read_price_per_million_tokens":"0","cache_write_price_per_million_tokens":"0","currency":"USD"}`)
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"cost-window-gateway","scopes":["gateway:invoke"],"cost_limit_5h":"0.01000000"}`)
	apiKey := keyResp.Data.PlaintextKey
	keyID, err := strconv.Atoi(string(keyResp.Data.ApiKey.Id))
	if err != nil {
		t.Fatalf("parse api key id: %v", err)
	}
	if _, err := keyStore.ApplyCostUsage(t.Context(), apikeycontract.CostUsageUpdate{
		KeyID:        keyID,
		BillableCost: "0.00999900",
		OccurredAt:   time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed api key cost usage: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"cost-window-model","messages":[{"role":"user","content":"window budget exceeded"}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected cost-window request 429, got %d body=%s", rec.Code, rec.Body.String())
	}
}
