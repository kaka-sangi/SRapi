package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func upstreamPassthroughHeaders() http.Header {
	h := http.Header{}
	h.Set("Retry-After", "30")
	h.Set("X-Request-Id", "req-123")
	h.Set("X-RateLimit-Remaining", "42")
	h.Set("X-RateLimit-Reset", "1717000000")
	h.Set("X-Secret-Token", "should-not-leak")
	h.Set("Connection", "keep-alive")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Content-Type", "application/json")
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
		allowlist: []string{"retry-after", "x-request-id", "x-ratelimit-*"},
	}
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), cfg)

	if got := rec.Header().Get("Retry-After"); got != "30" {
		t.Fatalf("retry-after not forwarded, got %q", got)
	}
	if got := rec.Header().Get("X-Request-Id"); got != "req-123" {
		t.Fatalf("x-request-id not forwarded, got %q", got)
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
		allowlist: []string{"retry-after", "x-request-id", "x-ratelimit-*"},
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
		allowlist: []string{"connection", "transfer-encoding", "content-type"},
	}
	forwardUpstreamResponseHeaders(rec, upstreamPassthroughHeaders(), cfg)

	for _, name := range []string{"Connection", "Transfer-Encoding", "Content-Type"} {
		if got := rec.Header().Get(name); got != "" {
			t.Fatalf("hop-by-hop/framing header %q forwarded with value %q", name, got)
		}
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
