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

func TestQuotaMetadataFromReportSurfacesPlanType(t *testing.T) {
	fetchedAt := time.Date(2026, 6, 11, 1, 2, 3, 0, time.UTC)

	t.Run("plan surfaced as plan_type and last_quota_plan", func(t *testing.T) {
		metadata := QuotaMetadataFromReport(map[string]any{"existing": "kept"}, QuotaCreditReport{
			Plan:      "  pro  ",
			FetchedAt: fetchedAt,
		})
		if metadata["plan_type"] != "pro" {
			t.Fatalf("plan_type = %v, want pro", metadata["plan_type"])
		}
		if metadata["last_quota_plan"] != "pro" {
			t.Fatalf("last_quota_plan = %v, want pro", metadata["last_quota_plan"])
		}
		if metadata["existing"] != "kept" {
			t.Fatalf("existing metadata not preserved: %v", metadata["existing"])
		}
	})

	t.Run("empty plan does not set plan_type", func(t *testing.T) {
		metadata := QuotaMetadataFromReport(nil, QuotaCreditReport{
			Plan:      "   ",
			FetchedAt: fetchedAt,
		})
		if _, ok := metadata["plan_type"]; ok {
			t.Fatalf("plan_type should be absent for empty plan, got %v", metadata["plan_type"])
		}
		if _, ok := metadata["last_quota_plan"]; ok {
			t.Fatalf("last_quota_plan should be absent for empty plan, got %v", metadata["last_quota_plan"])
		}
	})
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
