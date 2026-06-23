package httpserver

import (
	"net/http"
	"testing"
	"time"
)

// TestIsAccountTargetedUpstreamCooldownStatus pins the status codes that
// trigger an in-process account-level cooldown on top of the existing
// Retry-After path. The set must stay aligned with sub2api's
// classifyOpenAITransportError + classifyOpenAIUpstream behaviour:
//
//   - 408 / 429              — explicit throttle / read timeout signals.
//   - 500, 502, 503, 504     — overload / dependency outage on the
//     specific upstream behind the chosen credential.
//
// Anything outside the documented set must NOT trigger a cooldown — a
// 400/401/403 is a client/auth problem, not a "try again later", and
// arbitrary 5xx codes (505, 511, …) carry no retry semantics.
func TestIsAccountTargetedUpstreamCooldownStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   bool
	}{
		{"408 RequestTimeout", http.StatusRequestTimeout, true},
		{"429 TooManyRequests", http.StatusTooManyRequests, true},
		{"500 InternalServerError", http.StatusInternalServerError, true},
		{"502 BadGateway", http.StatusBadGateway, true},
		{"503 ServiceUnavailable", http.StatusServiceUnavailable, true},
		{"504 GatewayTimeout", http.StatusGatewayTimeout, true},

		{"0 NoStatus", 0, false},
		{"200 OK", http.StatusOK, false},
		{"400 BadRequest", http.StatusBadRequest, false},
		{"401 Unauthorized", http.StatusUnauthorized, false},
		{"403 Forbidden", http.StatusForbidden, false},
		{"404 NotFound", http.StatusNotFound, false},
		{"422 Unprocessable", http.StatusUnprocessableEntity, false},
		{"505 HTTPVersionNotSupported", http.StatusHTTPVersionNotSupported, false},
		{"511 NetworkAuthRequired", http.StatusNetworkAuthenticationRequired, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isAccountTargetedUpstreamCooldownStatus(tc.status); got != tc.want {
				t.Errorf("status %d: got %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

func TestAccountAdaptiveThrottleEnabled(t *testing.T) {
	if accountAdaptiveThrottleEnabled(nil) {
		t.Fatal("nil metadata must be disabled")
	}
	if accountAdaptiveThrottleEnabled(map[string]any{}) {
		t.Fatal("empty metadata must be disabled")
	}
	if !accountAdaptiveThrottleEnabled(map[string]any{"adaptive_throttle_enabled": true}) {
		t.Fatal("bool true must enable")
	}
	if !accountAdaptiveThrottleEnabled(map[string]any{"adaptive_throttle.enabled": true}) {
		t.Fatal("dot-separated key must enable")
	}
	if !accountAdaptiveThrottleEnabled(map[string]any{"adaptive_throttle_enabled": "true"}) {
		t.Fatal("string 'true' must enable (metacoerce accepts parseable strings)")
	}
}

func TestAccountAdaptiveThrottleDelay(t *testing.T) {
	enabled := map[string]any{"adaptive_throttle_enabled": true}
	disabled := map[string]any{}

	if d := accountAdaptiveThrottleDelay(disabled, 60); d != 0 {
		t.Fatalf("disabled: expected 0, got %v", d)
	}
	if d := accountAdaptiveThrottleDelay(enabled, 0); d != 0 {
		t.Fatalf("rpm=0: expected 0, got %v", d)
	}
	if d := accountAdaptiveThrottleDelay(enabled, -1); d != 0 {
		t.Fatalf("rpm=-1: expected 0, got %v", d)
	}

	// 60 RPM → 1s delay
	d := accountAdaptiveThrottleDelay(enabled, 60)
	if d != time.Second {
		t.Fatalf("rpm=60: expected 1s, got %v", d)
	}

	// 600 RPM → 100ms (clamped to min 50ms)
	d = accountAdaptiveThrottleDelay(enabled, 600)
	if d != 100*time.Millisecond {
		t.Fatalf("rpm=600: expected 100ms, got %v", d)
	}

	// 100000 RPM → clamped to min 50ms
	d = accountAdaptiveThrottleDelay(enabled, 100000)
	if d != adaptiveThrottleMinDelay {
		t.Fatalf("rpm=100000: expected min %v, got %v", adaptiveThrottleMinDelay, d)
	}

	// 1 RPM → clamped to max 5s
	d = accountAdaptiveThrottleDelay(enabled, 1)
	if d != adaptiveThrottleMaxDelay {
		t.Fatalf("rpm=1: expected max %v, got %v", adaptiveThrottleMaxDelay, d)
	}
}
