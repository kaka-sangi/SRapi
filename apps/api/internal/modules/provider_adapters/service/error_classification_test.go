package service

import (
	"net/http"
	"testing"
)

// TestClassifyProviderHTTPError401SessionInvalid verifies that a 401 whose body
// carries an OAuth revocation marker (invalid_grant / refresh_token_reused) is
// classified as "session_invalid" so it maps to NeedsReauth, while a plain 401
// without such a marker stays "auth_failed".
func TestClassifyProviderHTTPError401SessionInvalid(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		body      string
		wantClass string
	}{
		{
			name:      "invalid_grant marks session invalid",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid_grant"}`,
			wantClass: "session_invalid",
		},
		{
			name:      "invalid_grant with description marks session invalid",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid_grant","error_description":"refresh token revoked"}`,
			wantClass: "session_invalid",
		},
		{
			name:      "refresh_token_reused marks session invalid",
			status:    http.StatusUnauthorized,
			body:      `{"error":"refresh_token_reused"}`,
			wantClass: "session_invalid",
		},
		{
			name:      "plain unauthorized stays auth_failed",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid_api_key"}`,
			wantClass: "auth_failed",
		},
		{
			name:      "expired access token without revocation marker stays auth_failed",
			status:    http.StatusUnauthorized,
			body:      `{"error":"invalid_token","error_description":"access token expired"}`,
			wantClass: "auth_failed",
		},
		{
			name:      "invalid_grant only escalates on 401 not 403",
			status:    http.StatusForbidden,
			body:      `{"error":"invalid_grant"}`,
			wantClass: "forbidden",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyProviderHTTPError(tc.status, []byte(tc.body))
			if got.Class != tc.wantClass {
				t.Fatalf("classifyProviderHTTPError(%d, %q) class = %q, want %q", tc.status, tc.body, got.Class, tc.wantClass)
			}
			if got.StatusCode != tc.status {
				t.Fatalf("classifyProviderHTTPError(%d, %q) status = %d, want %d", tc.status, tc.body, got.StatusCode, tc.status)
			}
		})
	}
}

// TestClassifyProviderHTTPErrorCloudflareChallenge verifies that a 403/429 whose
// response is a Cloudflare JS challenge is classified as "cloudflare_challenge"
// (a transient class deliberately excluded from cooldown) and records the cf-ray
// in the error metadata, while a normal 403 retains its forbidden classification.
func TestClassifyProviderHTTPErrorCloudflareChallenge(t *testing.T) {
	challengeBody := []byte(`<!DOCTYPE html><html><head><title>Just a moment...</title></head>` +
		`<body><script>window._cf_chl_opt={cvId:'3'};</script>` +
		`Enable JavaScript and cookies to continue</body></html>`)
	challengeHeaders := http.Header{}
	challengeHeaders.Set("cf-ray", "8d2f1a9b0c4e1234-IAD")
	challengeHeaders.Set("content-type", "text/html; charset=UTF-8")

	got := classifyProviderHTTPErrorWithHeaders(http.StatusForbidden, challengeHeaders, challengeBody)
	if got.Class != "cloudflare_challenge" {
		t.Fatalf("cloudflare 403 class = %q, want cloudflare_challenge", got.Class)
	}
	if got.Metadata["cf_ray"] != "8d2f1a9b0c4e1234-IAD" {
		t.Fatalf("cloudflare 403 cf_ray metadata = %v, want cf-ray header value", got.Metadata["cf_ray"])
	}

	got429 := classifyProviderHTTPErrorWithHeaders(http.StatusTooManyRequests, challengeHeaders, challengeBody)
	if got429.Class != "cloudflare_challenge" {
		t.Fatalf("cloudflare 429 class = %q, want cloudflare_challenge", got429.Class)
	}

	normal := classifyProviderHTTPErrorWithHeaders(http.StatusForbidden, nil, []byte(`{"error":{"message":"forbidden"}}`))
	if normal.Class == "cloudflare_challenge" {
		t.Fatalf("normal 403 class = %q, want non cloudflare_challenge", normal.Class)
	}
	if _, ok := normal.Metadata["cf_ray"]; ok {
		t.Fatalf("normal 403 should not carry cf_ray metadata, got %v", normal.Metadata["cf_ray"])
	}
}

// TestClassifyGeminiProviderHTTPErrorRecognizesAntigravityCreditExhaustion
// proves the Gemini classifier promotes Antigravity credit-balance
// exhaustion (signalled via google.rpc.ErrorInfo.reason =
// INSUFFICIENT_G1_CREDITS_BALANCE) to the quota_exhausted class instead
// of the generic rate_limit. Without the explicit reason check the
// keyword fallback misses the underscore-cased enum and the account
// gets cooled down briefly then re-picked.
func TestClassifyGeminiProviderHTTPErrorRecognizesAntigravityCreditExhaustion(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"insufficient credit balance",` +
		`"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"INSUFFICIENT_G1_CREDITS_BALANCE"}` +
		`]}}`)
	got := classifyGeminiProviderHTTPError(http.StatusTooManyRequests, body)
	if got.Class != "quota_exhausted" {
		t.Fatalf("credit balance exhausted class = %q, want quota_exhausted", got.Class)
	}
}

// QUOTA_EXHAUSTED reason without a retryDelay should also classify as
// quota_exhausted (was previously only caught via the keyword fallback,
// which depended on the human message text — fragile).
func TestClassifyGeminiProviderHTTPErrorRecognizesQuotaExhaustedReason(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"resource exhausted",` +
		`"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"QUOTA_EXHAUSTED"}` +
		`]}}`)
	got := classifyGeminiProviderHTTPError(http.StatusTooManyRequests, body)
	if got.Class != "quota_exhausted" {
		t.Fatalf("quota exhausted reason class = %q, want quota_exhausted", got.Class)
	}
}

// RATE_LIMIT_EXCEEDED with a structured retryDelay must NOT be promoted
// to quota_exhausted — it's a transient throttle that should drive a
// short cooldown via the parsed RetryAfter (wave-4 item 1).
func TestClassifyGeminiProviderHTTPErrorKeepsRateLimitExceededAsRateLimit(t *testing.T) {
	body := []byte(`{"error":{"code":429,"message":"slow down",` +
		`"status":"RESOURCE_EXHAUSTED","details":[` +
		`{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"RATE_LIMIT_EXCEEDED"},` +
		`{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"2s"}` +
		`]}}`)
	got := classifyGeminiProviderHTTPError(http.StatusTooManyRequests, body)
	if got.Class != "rate_limit" {
		t.Fatalf("rate_limit_exceeded class = %q, want rate_limit", got.Class)
	}
	if got.RetryAfter == nil {
		t.Fatalf("expected parsed retryDelay to populate RetryAfter")
	}
}
