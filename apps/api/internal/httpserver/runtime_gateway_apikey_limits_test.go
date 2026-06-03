package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/srapi/srapi/apps/api/internal/config"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/platform/ratelimit"
)

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
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"ip-scoped-gateway","scopes":["gateway:invoke"],"allowed_ips":["10.0.0.0/8"]}`)
	apiKey := keyResp.Data.PlaintextKey

	// Client IP outside the allow list is rejected before scheduling.
	denied := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"blocked"}]}`))
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
	allowed := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"permitted"}]}`))
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
	keyResp := mustCreateAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"window-limited-gateway","scopes":["gateway:invoke"],"request_limit_5h":1}`)
	apiKey := keyResp.Data.PlaintextKey

	first := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"first windowed request"}]}`))
	first.Header.Set("Content-Type", "application/json")
	first.Header.Set("Authorization", "Bearer "+apiKey)
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, first)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("expected first windowed request 200, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"second windowed request"}]}`))
	second.Header.Set("Content-Type", "application/json")
	second.Header.Set("Authorization", "Bearer "+apiKey)
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, second)
	if secondRec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second windowed request 429, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
}
