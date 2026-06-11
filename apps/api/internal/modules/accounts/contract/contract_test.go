package contract

import (
	"testing"
	"time"
)

func TestShouldRefreshOAuthCredential(t *testing.T) {
	now := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)
	oauthAccount := ProviderAccount{RuntimeClass: RuntimeClassOauthRefresh}

	tests := []struct {
		name       string
		account    ProviderAccount
		credential map[string]any
		want       bool
	}{
		{
			name:       "non oauth account",
			account:    ProviderAccount{RuntimeClass: RuntimeClassAPIKey},
			credential: map[string]any{"access_token": "token"},
			want:       false,
		},
		{
			name:       "missing access token with refresh token",
			account:    oauthAccount,
			credential: map[string]any{"refresh_token": "refresh"},
			want:       true,
		},
		{
			name:       "expired access token",
			account:    oauthAccount,
			credential: map[string]any{"access_token": "old", "expires_at": now.Add(-time.Minute).Format(time.RFC3339)},
			want:       true,
		},
		{
			name:       "nearly expired access token",
			account:    oauthAccount,
			credential: map[string]any{"access_token": "old", "expires_at": now.Add(10 * time.Second).Format(time.RFC3339)},
			want:       true,
		},
		{
			name:       "valid access token",
			account:    oauthAccount,
			credential: map[string]any{"access_token": "fresh", "expires_at": now.Add(time.Hour).Format(time.RFC3339)},
			want:       false,
		},
		{
			name:       "metadata forces refresh",
			account:    ProviderAccount{RuntimeClass: RuntimeClassOauthDeviceCode, Metadata: map[string]any{"force_refresh": true}},
			credential: map[string]any{"access_token": "fresh", "expires_at": now.Add(time.Hour).Format(time.RFC3339)},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRefreshOAuthCredential(tt.account, tt.credential, now); got != tt.want {
				t.Fatalf("ShouldRefreshOAuthCredential() = %v, want %v", got, tt.want)
			}
		})
	}
}
