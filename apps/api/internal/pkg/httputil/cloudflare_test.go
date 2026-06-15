package httputil

import (
	"net/http"
	"testing"
)

// TestIsCloudflareChallengeResponse verifies that a synthetic Cloudflare JS
// challenge (challenge HTML body plus a cf-ray header) is detected on 403/429,
// while a normal 403 (and any non-403/429 status) is not.
func TestIsCloudflareChallengeResponse(t *testing.T) {
	challengeBody := []byte(`<!DOCTYPE html><html><head><title>Just a moment...</title></head>` +
		`<body><script>window._cf_chl_opt={cvId:'3'};</script>` +
		`Enable JavaScript and cookies to continue</body></html>`)
	challengeHeaders := http.Header{}
	challengeHeaders.Set("cf-ray", "8d2f1a9b0c4e1234-IAD")
	challengeHeaders.Set("content-type", "text/html; charset=UTF-8")

	cases := []struct {
		name    string
		status  int
		headers http.Header
		body    []byte
		want    bool
	}{
		{
			name:    "cloudflare challenge body with cf-ray on 403 is detected",
			status:  http.StatusForbidden,
			headers: challengeHeaders,
			body:    challengeBody,
			want:    true,
		},
		{
			name:    "cloudflare challenge on 429 is detected",
			status:  http.StatusTooManyRequests,
			headers: challengeHeaders,
			body:    challengeBody,
			want:    true,
		},
		{
			name:   "cf-mitigated challenge header is detected",
			status: http.StatusForbidden,
			headers: func() http.Header {
				h := http.Header{}
				h.Set("cf-mitigated", "challenge")
				return h
			}(),
			body: []byte("blocked"),
			want: true,
		},
		{
			name:    "normal 403 json error is not a challenge",
			status:  http.StatusForbidden,
			headers: nil,
			body:    []byte(`{"error":{"type":"permission_error","message":"forbidden"}}`),
			want:    false,
		},
		{
			name:    "challenge body on non-403/429 status is ignored",
			status:  http.StatusInternalServerError,
			headers: challengeHeaders,
			body:    challengeBody,
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsCloudflareChallengeResponse(tc.status, tc.headers, tc.body); got != tc.want {
				t.Fatalf("IsCloudflareChallengeResponse(%d) = %v, want %v", tc.status, got, tc.want)
			}
		})
	}
}

// TestExtractCloudflareRayID verifies cf-ray extraction prefers the header and
// falls back to the response body.
func TestExtractCloudflareRayID(t *testing.T) {
	headers := http.Header{}
	headers.Set("cf-ray", "8d2f1a9b0c4e1234-IAD")
	if got := ExtractCloudflareRayID(headers, nil); got != "8d2f1a9b0c4e1234-IAD" {
		t.Fatalf("ExtractCloudflareRayID header = %q, want header value", got)
	}

	body := []byte(`<script>cRay:'7a1b2c3d4e5f6789-LHR';</script>`)
	if got := ExtractCloudflareRayID(nil, body); got != "7a1b2c3d4e5f6789-LHR" {
		t.Fatalf("ExtractCloudflareRayID body = %q, want body value", got)
	}

	if got := ExtractCloudflareRayID(nil, []byte("no ray here")); got != "" {
		t.Fatalf("ExtractCloudflareRayID missing = %q, want empty", got)
	}
}

// TestFormatCloudflareChallengeMessage verifies the cf-ray is appended only when
// present.
func TestFormatCloudflareChallengeMessage(t *testing.T) {
	headers := http.Header{}
	headers.Set("cf-ray", "8d2f1a9b0c4e1234-IAD")
	got := FormatCloudflareChallengeMessage("blocked by cloudflare", headers, nil)
	want := "blocked by cloudflare (cf-ray: 8d2f1a9b0c4e1234-IAD)"
	if got != want {
		t.Fatalf("FormatCloudflareChallengeMessage = %q, want %q", got, want)
	}

	if got := FormatCloudflareChallengeMessage("blocked", nil, nil); got != "blocked" {
		t.Fatalf("FormatCloudflareChallengeMessage no ray = %q, want unchanged", got)
	}
}
