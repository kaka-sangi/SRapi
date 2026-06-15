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
