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

func TestQuotaCreditSnapshotFromReport(t *testing.T) {
	account := ProviderAccount{ID: 10, ProviderID: 20}
	fetchedAt := time.Date(2026, 6, 11, 1, 2, 3, 0, time.UTC)

	snapshot, ok := QuotaCreditSnapshotFromReport(account, QuotaCreditReport{
		CreditsRemaining: "900",
		CreditsUsed:      "100",
		CreditsLimit:     "1000",
		FetchedAt:        fetchedAt,
	})
	if !ok {
		t.Fatal("expected provider credits snapshot")
	}
	if snapshot.QuotaType != QuotaTypeProviderCredits ||
		snapshot.Remaining != "900" ||
		snapshot.Used != "100" ||
		snapshot.QuotaLimit != "1000" ||
		snapshot.RemainingRatio != 0.9 ||
		!snapshot.SnapshotAt.Equal(fetchedAt) {
		t.Fatalf("unexpected provider credits snapshot: %+v", snapshot)
	}
}

func TestQuotaCreditSnapshotFromReportSkipsUnknownCreditRatio(t *testing.T) {
	account := ProviderAccount{ID: 10, ProviderID: 20}

	if snapshot, ok := QuotaCreditSnapshotFromReport(account, QuotaCreditReport{
		CreditsRemaining: "25000",
		Currency:         "GOOGLE_ONE_AI",
	}); ok {
		t.Fatalf("expected unknown credit ratio to skip provider credits snapshot, got %+v", snapshot)
	}
}
