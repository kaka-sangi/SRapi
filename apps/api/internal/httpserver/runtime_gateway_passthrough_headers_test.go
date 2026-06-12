package httpserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/config"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
)

func upstreamPassthroughHeaders() http.Header {
	h := http.Header{}
	h.Set("Retry-After", "30")
	h.Set("X-Request-Id", "req-123")
	h.Set("Location", "https://auth.example/challenge")
	h.Set("Www-Authenticate", `Bearer realm="upstream"`)
	h.Set("X-RateLimit-Remaining", "42")
	h.Set("X-RateLimit-Reset", "1717000000")
	h.Set("X-Secret-Token", "should-not-leak")
	h.Set("Connection", "keep-alive")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Content-Type", "application/json")
	h.Set("Set-Cookie", "upstream_session=secret")
	return h
}

func TestForwardUpstreamResponseHeaders_DisabledForwardsNothing(t *testing.T) {
	rec := httptest.NewRecorder()
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), gatewayPassthroughHeaderConfig{})
	if got := len(rec.Header()); got != 0 {
		t.Fatalf("expected no headers forwarded when disabled, got %d: %v", got, rec.Header())
	}
}

func TestForwardUpstreamResponseHeaders_AllowlistedForwarded(t *testing.T) {
	rec := httptest.NewRecorder()
	cfg := gatewayPassthroughHeaderConfig{
		enabled:   true,
		allowlist: []string{"retry-after", "location", "www-authenticate", "x-request-id", "x-ratelimit-*"},
	}
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), cfg)

	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("retry-after not forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("x-request-id not forwarded, got %q", got)
	}
	if got := rec.Header().Get("Location"); got != "https://auth.example/challenge" {
		t.Fatalf("location not forwarded, got %q", got)
	}
	if got := rec.Header().Get("Www-Authenticate"); got != `Bearer realm="upstream"` {
		t.Fatalf("www-authenticate not forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "42" {
		t.Fatalf("x-ratelimit-remaining (wildcard) not forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Reset"); got != "1717000000" {
		t.Fatalf("x-ratelimit-reset (wildcard) not forwarded, got %q", got)
	}
}

func TestForwardUpstreamResponseHeaders_NonAllowlistedDropped(t *testing.T) {
	rec := httptest.NewRecorder()
	cfg := gatewayPassthroughHeaderConfig{
		enabled:   true,
		allowlist: []string{"retry-after", "location", "www-authenticate", "x-request-id", "x-ratelimit-*"},
	}
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), cfg)

	if got := rec.Header().Get("X-Secret-Token"); got != "" {
		t.Fatalf("non-allowlisted header leaked: %q", got)
	}
}

func TestForwardUpstreamResponseHeaders_HopByHopDropped(t *testing.T) {
	rec := httptest.NewRecorder()
	// Even explicitly allowlisting hop-by-hop / body-framing headers must not
	// forward them.
	cfg := gatewayPassthroughHeaderConfig{
		enabled:   true,
		allowlist: []string{"connection", "transfer-encoding", "content-type", "set-cookie"},
	}
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), cfg)

	for _, name := range []string{"Connection", "Transfer-Encoding", "Content-Type", "Set-Cookie"} {
		if got := rec.Header().Get(name); got != "" {
			t.Fatalf("hop-by-hop/framing header %q forwarded with value %q", name, got)
		}
	}
}

func TestForwardUpstreamResponseHeaders_ConnectionScopedAndGatewayFingerprintsDropped(t *testing.T) {
	upstream := http.Header{}
	upstream.Add("Connection", "x-hop-a, x-hop-b")
	upstream.Add("Connection", "x-hop-c")
	upstream.Set("X-Hop-A", "a")
	upstream.Set("X-Hop-B", "b")
	upstream.Set("X-Hop-C", "c")
	upstream.Set("X-LiteLLM-Call-ID", "lite")
	upstream.Set("Helicone-Id", "helicone")
	upstream.Set("X-Portkey-Request-Id", "portkey")
	upstream.Set("Cf-Aig-Request-Id", "cf")
	upstream.Set("X-Kong-Proxy-Latency", "kong")
	upstream.Set("X-Bt-Gateway", "bt")
	upstream.Set("X-Request-Id", "req-123")
	rec := httptest.NewRecorder()
	cfg := gatewayPassthroughHeaderConfig{
		enabled: true,
		allowlist: []string{
			"x-hop-*",
			"x-litellm-*",
			"helicone-*",
			"x-portkey-*",
			"cf-aig-*",
			"x-kong-*",
			"x-bt-*",
			"x-request-id",
		},
	}

	forwardUpstreamResponseHeaders(rec, upstream, cfg)

	for _, name := range []string{"X-Hop-A", "X-Hop-B", "X-Hop-C", "X-LiteLLM-Call-ID", "Helicone-Id", "X-Portkey-Request-Id", "Cf-Aig-Request-Id", "X-Kong-Proxy-Latency", "X-Bt-Gateway"} {
		if got := rec.Header().Get(name); got != "" {
			t.Fatalf("blocked upstream fingerprint/scoped header %q forwarded with value %q", name, got)
		}
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("expected ordinary allowlisted header to be forwarded, got %q", got)
	}
}

func TestForwardUpstreamResponseHeaders_NeverOverridesExisting(t *testing.T) {
	rec := httptest.NewRecorder()
	// SRapi already set Retry-After; an allowlisted upstream value must not win.
	rec.Header().Set("Retry-After", "5")
	cfg := gatewayPassthroughHeaderConfig{
		enabled:   true,
		allowlist: []string{"retry-after"},
	}
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), cfg)

	if got := rec.Header().Get("Retry-After"); got != "5" {
		t.Fatalf("existing Retry-After overridden, got %q want 5", got)
	}
}

func TestForwardProviderErrorHeaders_AllowlistedForwarded(t *testing.T) {
	rec := httptest.NewRecorder()
	cfg := gatewayPassthroughHeaderConfig{
		enabled:   true,
		allowlist: []string{"retry-after", "x-request-id", "x-ratelimit-*"},
	}
	providerErr := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		Message:    "too many requests",
		Headers:    upstreamPassthroughHeaders(),
	}

	forwardProviderErrorHeaders(rec, providerErr, cfg)

	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("expected Retry-After to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("expected X-Request-Id to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "42" {
		t.Fatalf("expected X-RateLimit-Remaining to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-Secret-Token"); got != "" {
		t.Fatalf("non-allowlisted header leaked: %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "" {
		t.Fatalf("content type should stay owned by SRapi, got %q", got)
	}
}

func TestGatewayProviderErrorUsesProviderRetryAfter(t *testing.T) {
	retryAt := time.Now().Add(45 * time.Second)
	rec := httptest.NewRecorder()
	providerErr := provideradaptercontract.ProviderError{
		Class:      "rate_limit",
		StatusCode: http.StatusTooManyRequests,
		Message:    "too many requests",
		RetryAfter: &retryAt,
	}

	(&Server{}).writeProviderGatewayError(rec, providerErr)

	got := rec.Header().Get("Retry-After")
	if got == "" || got == "60" {
		t.Fatalf("expected provider retry hint instead of default 60, got %q", got)
	}
	seconds, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("expected numeric Retry-After, got %q", got)
	}
	if seconds < 1 || seconds > 45 {
		t.Fatalf("expected Retry-After within provider retry window, got %d", seconds)
	}
}

func TestHeaderAllowlistMatches(t *testing.T) {
	allow := []string{"retry-after", "x-ratelimit-*", "  X-Request-Id  "}
	cases := []struct {
		name string
		want bool
	}{
		{"retry-after", true},
		{"Retry-After", false}, // matcher receives a canonical lowercased name
		{"x-ratelimit-remaining", true},
		{"x-ratelimit-reset", true},
		{"x-request-id", true},
		{"x-secret", false},
		{"x-rate", false},
	}
	for _, tc := range cases {
		if got := headerAllowlistMatches(tc.name, allow); got != tc.want {
			t.Errorf("headerAllowlistMatches(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestGatewayBufferedRenderedResponseForwardsAllowlistedUpstreamHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.Header().Set("X-Request-Id", "req-buffered")
		w.Header().Set("X-Upstream-Request-Id", "upstream-buffered")
		w.Header().Set("X-RateLimit-Remaining-Requests", "12")
		w.Header().Set("X-RateLimit-Remaining-Tokens", "345")
		w.Header().Set("X-Secret-Token", "should-not-leak")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"buffered ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustEnableGatewayPassthroughHeaders(t, handler, sessionCookie, loginResp.Data.CsrfToken, []string{"retry-after", "x-request-id", "x-upstream-request-id", "x-ratelimit-*"})
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"buffered-header-provider","display_name":"Buffered Header Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"buffered-header-model","display_name":"Buffered Header Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"buffered-header-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"buffered-header-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"buffered-header-model","messages":[{"role":"user","content":"hi"}]}`)
	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("expected Retry-After to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-Request-Id"); got == "" || got == "req-buffered" {
		t.Fatalf("expected SRapi X-Request-Id to be preserved, got %q", got)
	}
	if got := rec.Header().Get("X-Upstream-Request-Id"); got != "upstream-buffered" {
		t.Fatalf("expected X-Upstream-Request-Id to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining-Requests"); got != "12" {
		t.Fatalf("expected request rate limit header to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-RateLimit-Remaining-Tokens"); got != "345" {
		t.Fatalf("expected token rate limit header to be forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-Secret-Token"); got != "" {
		t.Fatalf("non-allowlisted header leaked: %q", got)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("expected SRapi-rendered JSON content type, got %q", got)
	}
}

func TestGatewayPassthroughDefaultAllowlistForwardsCodexQuotaHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Language", "en-US")
		w.Header().Set("Date", "Fri, 12 Jun 2026 10:00:00 GMT")
		w.Header().Set("Etag", `"quota-headers"`)
		w.Header().Set("Expires", "Fri, 12 Jun 2026 10:05:00 GMT")
		w.Header().Set("Last-Modified", "Fri, 12 Jun 2026 09:59:00 GMT")
		w.Header().Set("Vary", "Authorization")
		w.Header().Set("X-Codex-Primary-Used-Percent", "12")
		w.Header().Set("X-Codex-Primary-Reset-After-Seconds", "600")
		w.Header().Set("X-Codex-Primary-Window-Minutes", "300")
		w.Header().Set("X-Codex-Secondary-Used-Percent", "34")
		w.Header().Set("X-Codex-Secondary-Reset-After-Seconds", "86400")
		w.Header().Set("X-Codex-Secondary-Window-Minutes", "10080")
		w.Header().Set("X-Codex-Primary-Over-Secondary-Limit-Percent", "110")
		w.Header().Set("X-Secret-Token", "should-not-leak")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"codex quota headers ok"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`))
	}))
	defer upstream.Close()

	handler := New(config.Load(), nil)
	loginResp, sessionCookie := mustLoginAdmin(t, handler)
	mustEnableGatewayPassthroughHeadersWithDefaultAllowlist(t, handler, sessionCookie, loginResp.Data.CsrfToken)
	providerResp := mustCreateProvider(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"name":"default-header-provider","display_name":"Default Header Provider","adapter_type":"openai-compatible","protocol":"openai-compatible","status":"active"}`)
	modelResp := mustCreateModel(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"canonical_name":"default-header-model","display_name":"Default Header Model","status":"active"}`)
	mustCreateMapping(t, handler, sessionCookie, loginResp.Data.CsrfToken, string(modelResp.Data.Id), `{"provider_id":"`+string(providerResp.Data.Id)+`","upstream_model_name":"default-header-upstream","status":"active"}`)
	mustCreateAccount(t, handler, sessionCookie, loginResp.Data.CsrfToken, `{"provider_id":"`+string(providerResp.Data.Id)+`","name":"default-header-account","runtime_class":"api_key","credential":{"api_key":"upstream-secret"},"metadata":{"base_url":"`+upstream.URL+`/v1"},"status":"active"}`)
	_, apiKey := mustCreateGatewayAPIKey(t, handler, sessionCookie, loginResp.Data.CsrfToken)

	rec := mustGatewayRequest(t, handler, apiKey, http.MethodPost, "/v1/chat/completions", `{"model":"default-header-model","messages":[{"role":"user","content":"hi"}]}`)
	for name, want := range map[string]string{
		"Cache-Control":                         "no-cache",
		"Content-Language":                      "en-US",
		"Date":                                  "Fri, 12 Jun 2026 10:00:00 GMT",
		"Etag":                                  `"quota-headers"`,
		"Expires":                               "Fri, 12 Jun 2026 10:05:00 GMT",
		"Last-Modified":                         "Fri, 12 Jun 2026 09:59:00 GMT",
		"Vary":                                  "Authorization",
		"X-Codex-Primary-Used-Percent":          "12",
		"X-Codex-Primary-Reset-After-Seconds":   "600",
		"X-Codex-Primary-Window-Minutes":        "300",
		"X-Codex-Secondary-Used-Percent":        "34",
		"X-Codex-Secondary-Reset-After-Seconds": "86400",
		"X-Codex-Secondary-Window-Minutes":      "10080",
		"X-Codex-Primary-Over-Secondary-Limit-Percent": "110",
	} {
		if got := rec.Header().Get(name); got != want {
			t.Fatalf("expected %s to be forwarded as %q, got %q", name, want, got)
		}
	}
	if got := rec.Header().Get("X-Secret-Token"); got != "" {
		t.Fatalf("non-allowlisted header leaked: %q", got)
	}
}

func mustEnableGatewayPassthroughHeaders(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string, allowlist []string) {
	t.Helper()
	settings := mustGetAdminSettings(t, handler, sessionCookie)
	enabled := true
	settings.Data.Gateway.PassthroughUpstreamHeaders = &enabled
	settings.Data.Gateway.PassthroughHeaderAllowlist = &allowlist
	body, err := json.Marshal(settings.Data)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected settings update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var updated apiopenapi.AdminSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode settings update: %v", err)
	}
	if updated.Data.Gateway.PassthroughUpstreamHeaders == nil || !*updated.Data.Gateway.PassthroughUpstreamHeaders {
		t.Fatalf("passthrough headers setting was not enabled: %+v", updated.Data.Gateway)
	}
	if updated.Data.Gateway.PassthroughHeaderAllowlist == nil || len(*updated.Data.Gateway.PassthroughHeaderAllowlist) != len(allowlist) {
		t.Fatalf("passthrough header allowlist not saved: %+v", updated.Data.Gateway)
	}
	for i, got := range *updated.Data.Gateway.PassthroughHeaderAllowlist {
		if got != allowlist[i] {
			t.Fatalf("allowlist[%d] = %q, want %q; full=%s", i, got, allowlist[i], fmt.Sprint(*updated.Data.Gateway.PassthroughHeaderAllowlist))
		}
	}
}

func mustEnableGatewayPassthroughHeadersWithDefaultAllowlist(t *testing.T, handler http.Handler, sessionCookie *http.Cookie, csrfToken string) {
	t.Helper()
	settings := mustGetAdminSettings(t, handler, sessionCookie)
	enabled := true
	settings.Data.Gateway.PassthroughUpstreamHeaders = &enabled
	body, err := json.Marshal(settings.Data)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected settings update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var updated apiopenapi.AdminSettingsResponse
	if err := json.NewDecoder(rec.Body).Decode(&updated); err != nil {
		t.Fatalf("decode settings update: %v", err)
	}
	if updated.Data.Gateway.PassthroughUpstreamHeaders == nil || !*updated.Data.Gateway.PassthroughUpstreamHeaders {
		t.Fatalf("passthrough headers setting was not enabled: %+v", updated.Data.Gateway)
	}
	if updated.Data.Gateway.PassthroughHeaderAllowlist == nil || len(*updated.Data.Gateway.PassthroughHeaderAllowlist) == 0 {
		t.Fatalf("default passthrough allowlist was not saved: %+v", updated.Data.Gateway)
	}
}
