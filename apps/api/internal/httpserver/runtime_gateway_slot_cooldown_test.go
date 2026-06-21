package httpserver

import (
	"net/http"
	"testing"
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
