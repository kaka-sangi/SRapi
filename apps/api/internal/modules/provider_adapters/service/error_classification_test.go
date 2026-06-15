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
