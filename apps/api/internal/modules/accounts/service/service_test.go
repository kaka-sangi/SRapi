package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	"github.com/srapi/srapi/apps/api/internal/testsupport/oteltest"
)

func TestCreateEncryptsCredential(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	created, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "main",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if created.CredentialCiphertext == "" {
		t.Fatal("expected ciphertext")
	}
	if strings.Contains(created.CredentialCiphertext, "secret-value") {
		t.Fatal("ciphertext leaked plaintext")
	}
}

func TestCreateRejectsMissingCredential(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "main",
		RuntimeClass: contract.RuntimeClassAPIKey,
	})
	if !errors.Is(err, ErrCredentialMissing) {
		t.Fatalf("expected ErrCredentialMissing, got %v", err)
	}
}

func TestAccountRiskLevelCreateUpdateAndValidation(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	high := "high"
	created, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "risky",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		RiskLevel:    &high,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if created.RiskLevel == nil || *created.RiskLevel != "high" {
		t.Fatalf("expected high risk level, got %+v", created.RiskLevel)
	}
	medium := "medium"
	updated, err := svc.Update(context.Background(), created.ID, contract.UpdateRequest{RiskLevel: &medium})
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	if updated.RiskLevel == nil || *updated.RiskLevel != "medium" {
		t.Fatalf("expected medium risk level, got %+v", updated.RiskLevel)
	}
	invalid := "critical"
	if _, err := svc.Update(context.Background(), created.ID, contract.UpdateRequest{RiskLevel: &invalid}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid risk level to fail, got %v", err)
	}
}

func TestProxyRegistryEncryptsURLAndResolvesRuntimeURL(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()

	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name:     "us-east-egress",
		Type:     contract.ProxyTypeHTTPS,
		URL:      "https://proxy-user:proxy-pass@example.invalid:8443",
		Metadata: map[string]any{"region": "us-east"},
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if proxy.URLCiphertext == "" || strings.Contains(proxy.URLCiphertext, "proxy-pass") {
		t.Fatalf("proxy url was not encrypted: %+v", proxy)
	}
	if proxy.URLVersion != "v1" || proxy.Metadata["region"] != "us-east" {
		t.Fatalf("unexpected proxy metadata: %+v", proxy)
	}

	id := strconv.Itoa(proxy.ID)
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   1,
		Name:         "proxied",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	bound, err := svc.BindProxy(ctx, account.ID, &id)
	if err != nil {
		t.Fatalf("bind proxy: %v", err)
	}
	if bound.ProxyID == nil || *bound.ProxyID != id {
		t.Fatalf("unexpected proxy binding: %+v", bound)
	}

	runtimeURL, err := svc.ResolveProxyURL(ctx, bound.ProxyID)
	if err != nil {
		t.Fatalf("resolve proxy url: %v", err)
	}
	if runtimeURL == nil || *runtimeURL != "https://proxy-user:proxy-pass@example.invalid:8443" {
		t.Fatalf("unexpected runtime proxy url: %v", runtimeURL)
	}

	rawProxyID := "https://proxy-user:proxy-pass@example.invalid:8443"
	if _, err := svc.ResolveProxyURL(ctx, &rawProxyID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected raw proxy_id URL to be rejected, got %v", err)
	}

	disabled := contract.ProxyStatusDisabled
	if _, err := svc.UpdateProxy(ctx, proxy.ID, contract.UpdateProxyRequest{Status: &disabled}); err != nil {
		t.Fatalf("disable proxy: %v", err)
	}
	if _, err := svc.ResolveProxyURL(ctx, &id); !errors.Is(err, ErrProxyUnavailable) {
		t.Fatalf("expected unavailable proxy resolution, got %v", err)
	}
}

func TestDeleteProxySoftDeletesAndUnbindsAccounts(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()

	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "egress",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://proxy-user:proxy-pass@example.invalid:8443",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	id := strconv.Itoa(proxy.ID)
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   1,
		Name:         "proxied",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := svc.BindProxy(ctx, account.ID, &id); err != nil {
		t.Fatalf("bind proxy: %v", err)
	}

	if err := svc.DeleteProxy(ctx, proxy.ID); err != nil {
		t.Fatalf("delete proxy: %v", err)
	}

	// The proxy is gone from lookup and listing.
	if _, err := svc.FindProxyByID(ctx, proxy.ID); err == nil {
		t.Fatalf("expected deleted proxy to be unfindable")
	}
	list, err := svc.ListProxies(ctx)
	if err != nil {
		t.Fatalf("list proxies: %v", err)
	}
	for _, p := range list {
		if p.ID == proxy.ID {
			t.Fatalf("deleted proxy must not appear in listing: %+v", p)
		}
	}

	// The account that routed through it falls back to a direct connection.
	got, err := svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if got.ProxyID != nil {
		t.Fatalf("expected account proxy binding cleared, got %q", *got.ProxyID)
	}

	// Re-deleting an already-deleted proxy is a not-found, not a crash.
	if err := svc.DeleteProxy(ctx, proxy.ID); err == nil {
		t.Fatalf("expected error deleting an already-deleted proxy")
	}
}

func TestProxyRegistryRejectsMismatchedScheme(t *testing.T) {
	svc, err := New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = svc.CreateProxy(context.Background(), contract.CreateProxyRequest{
		Name: "bad-proxy",
		Type: contract.ProxyTypeSOCKS5,
		URL:  "https://example.invalid:8443",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestAccountProxyIDRejectsRawURLAndNonNumericValues(t *testing.T) {
	svc, err := New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	rawURL := "https://proxy-user:proxy-pass@example.invalid:8443"
	if _, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   1,
		Name:         "raw-url-proxy",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		ProxyID:      &rawURL,
	}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected raw URL proxy_id to fail create, got %v", err)
	}

	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   1,
		Name:         "valid-account",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	for _, proxyID := range []string{"proxy-us", rawURL, "0"} {
		if _, err := svc.BindProxy(ctx, account.ID, &proxyID); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid proxy_id %q to fail bind, got %v", proxyID, err)
		}
		updateProxyID := &proxyID
		if _, err := svc.Update(ctx, account.ID, contract.UpdateRequest{ProxyID: &updateProxyID}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid proxy_id %q to fail update, got %v", proxyID, err)
		}
	}
}

func TestAccountOperationsManageGroupsProxyRecoveryAndSnapshots(t *testing.T) {
	svc, err := New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()

	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   7,
		Name:         "ops-main",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		Metadata: map[string]any{
			"cooldown_active":  true,
			"cooldown_reason":  "rate_limit",
			"cooldown_until":   "2026-05-22T00:00:00Z",
			"circuit_open":     true,
			"last_error_class": "rate_limit",
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	group, err := svc.CreateGroup(ctx, contract.CreateGroupRequest{
		Name:        "premium-pool",
		Description: "premium accounts",
		ProviderScope: map[string]any{
			"provider_ids": []any{float64(7)},
		},
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := svc.AddAccountToGroup(ctx, account.ID, group.ID); err != nil {
		t.Fatalf("add account to group: %v", err)
	}
	groupIDs, err := svc.ListGroupIDsByAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("list group ids: %v", err)
	}
	if len(groupIDs) != 1 || groupIDs[0] != group.ID {
		t.Fatalf("unexpected group ids: %v", groupIDs)
	}
	groupIDsByAccount, err := svc.ListGroupIDsByAccounts(ctx, []int{account.ID, account.ID})
	if err != nil {
		t.Fatalf("list group ids by accounts: %v", err)
	}
	got := groupIDsByAccount[account.ID]
	if len(got) != 1 || got[0] != group.ID {
		t.Fatalf("unexpected batched group ids: %v", groupIDsByAccount)
	}

	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "group-egress",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://proxy-user:proxy-pass@example.invalid:8443",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	proxyID := strconv.Itoa(proxy.ID)
	withProxy, err := svc.BindProxy(ctx, account.ID, &proxyID)
	if err != nil {
		t.Fatalf("bind proxy: %v", err)
	}
	if withProxy.ProxyID == nil || *withProxy.ProxyID != proxyID {
		t.Fatalf("expected proxy binding, got %+v", withProxy)
	}

	recovered, err := svc.Recover(ctx, account.ID)
	if err != nil {
		t.Fatalf("recover account: %v", err)
	}
	if recovered.Status != contract.StatusActive || recovered.Metadata["cooldown_active"] != nil || recovered.Metadata["circuit_open"] != nil {
		t.Fatalf("expected recovery to clear protection metadata, got %+v", recovered)
	}
	if recovered.Metadata["last_recovered_at"] == nil {
		t.Fatalf("expected recovery timestamp, got %+v", recovered.Metadata)
	}

	now := time.Now().UTC()
	health, err := svc.RecordHealthSnapshot(ctx, contract.AccountHealthSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		Status:         "degraded",
		SuccessRate:    0.25,
		ErrorRate:      0.75,
		LatencyP50MS:   120,
		LatencyP95MS:   450,
		RateLimitCount: 2,
		TimeoutCount:   1,
		CircuitState:   "open",
		SnapshotAt:     now,
	})
	if err != nil {
		t.Fatalf("record health snapshot: %v", err)
	}
	latest, err := svc.LatestHealthSnapshotByAccount(ctx, account.ID)
	if err != nil {
		t.Fatalf("latest health snapshot: %v", err)
	}
	if latest.ID != health.ID || latest.CircuitState != "open" || latest.RateLimitCount != 2 {
		t.Fatalf("unexpected latest health: %+v", latest)
	}

	quota, err := svc.RecordQuotaSnapshot(ctx, contract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "monthly_tokens",
		Remaining:      "1000",
		Used:           "250",
		QuotaLimit:     "1250",
		RemainingRatio: 0.8,
		SnapshotAt:     now,
	})
	if err != nil {
		t.Fatalf("record quota snapshot: %v", err)
	}
	quotas, err := svc.ListQuotaSnapshotsByAccount(ctx, account.ID, 10)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	if len(quotas) != 1 || quotas[0].ID != quota.ID || quotas[0].RemainingRatio != 0.8 {
		t.Fatalf("unexpected quota snapshots: %+v", quotas)
	}
}

func TestApplyQuotaReportPersistsProviderCreditSignals(t *testing.T) {
	store := accountmemory.New()
	now := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   3,
		Name:         "antigravity-oauth",
		RuntimeClass: contract.RuntimeClassOauthRefresh,
		Credential:   map[string]any{"access_token": "secret-value"},
		Metadata:     map[string]any{"existing": "kept"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	signals, err := svc.ApplyQuotaReport(ctx, account, provideradaptercontract.QuotaReport{
		Supported:        true,
		Plan:             "g1-pro-tier",
		CreditsRemaining: "25",
		Currency:         "GOOGLE_ONE_AI",
		FetchedAt:        now,
		QuotaSignals: []provideradaptercontract.QuotaSignal{{
			QuotaType:      "antigravity_google_one_ai_credits",
			Remaining:      "25",
			Used:           "25",
			QuotaLimit:     "50",
			RemainingRatio: 0.5,
			SnapshotAt:     now,
		}},
	})
	if err != nil {
		t.Fatalf("apply quota report: %v", err)
	}
	if signals != 1 {
		t.Fatalf("expected one persisted quota signal, got %d", signals)
	}
	stored, err := svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if stored.Metadata["existing"] != "kept" ||
		stored.Metadata["last_quota_plan"] != "g1-pro-tier" ||
		stored.Metadata["last_quota_credits_remaining"] != "25" ||
		stored.Metadata["last_quota_currency"] != "GOOGLE_ONE_AI" {
		t.Fatalf("unexpected quota metadata: %+v", stored.Metadata)
	}
	quotas, err := svc.ListQuotaSnapshotsByAccount(ctx, account.ID, 10)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	if len(quotas) != 1 ||
		quotas[0].QuotaType != "antigravity_google_one_ai_credits" ||
		quotas[0].Remaining != "25" ||
		quotas[0].Used != "25" ||
		quotas[0].QuotaLimit != "50" ||
		quotas[0].RemainingRatio != 0.5 {
		t.Fatalf("unexpected quota snapshots: %+v", quotas)
	}
}

func TestApplyQuotaReportUpdatesRuntimeQuotaMetadataFromSignals(t *testing.T) {
	store := accountmemory.New()
	now := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)
	resetAt := now.Add(2 * time.Hour)
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   3,
		Name:         "quota-signals",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		Metadata: map[string]any{
			"quota_exhausted":     true,
			"quota_exhausted_at":  now.Add(-time.Hour).Format(time.RFC3339),
			"quota_remaining_old": "kept",
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	signals, err := svc.ApplyQuotaReport(ctx, account, provideradaptercontract.QuotaReport{
		Supported: true,
		FetchedAt: now,
		QuotaSignals: []provideradaptercontract.QuotaSignal{
			{
				QuotaType:      "codex_5h_percent",
				Remaining:      "0",
				Used:           "100",
				QuotaLimit:     "100",
				RemainingRatio: 0,
				ResetAt:        &resetAt,
				SnapshotAt:     now,
				Metadata: map[string]any{
					"codex_primary_over_secondary_percent": 117.5,
					"codex_usage_updated_at":               now.Format(time.RFC3339),
					"nested_ignored":                       map[string]any{"unsafe": true},
				},
			},
			{
				QuotaType:      "codex_7d_percent",
				Remaining:      "75",
				Used:           "25",
				QuotaLimit:     "100",
				RemainingRatio: 0.75,
				SnapshotAt:     now,
			},
		},
	})
	if err != nil {
		t.Fatalf("apply quota report: %v", err)
	}
	if signals != 2 {
		t.Fatalf("expected two persisted quota signals, got %d", signals)
	}
	stored, err := svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if stored.Metadata["quota_remaining_ratio"] != float64(0) ||
		stored.Metadata["quota_exhausted"] != true ||
		stored.Metadata["quota_type"] != "codex_5h_percent" ||
		stored.Metadata["quota_remaining"] != "0" ||
		stored.Metadata["quota_used"] != "100" ||
		stored.Metadata["quota_limit"] != "100" ||
		stored.Metadata["quota_reset_at"] != resetAt.Format(time.RFC3339) ||
		stored.Metadata["quota_exhausted_at"] != now.Format(time.RFC3339) ||
		stored.Metadata["codex_primary_over_secondary_percent"] != 117.5 ||
		stored.Metadata["codex_usage_updated_at"] != now.Format(time.RFC3339) ||
		stored.Metadata["nested_ignored"] != nil ||
		stored.Metadata["quota_remaining_old"] != "kept" {
		t.Fatalf("unexpected quota metadata: %+v", stored.Metadata)
	}
}

func TestApplyQuotaReportClearsRuntimeQuotaExhaustionWhenSignalsRecover(t *testing.T) {
	store := accountmemory.New()
	now := time.Date(2026, 6, 9, 1, 2, 3, 0, time.UTC)
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   3,
		Name:         "quota-recovered",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		Metadata: map[string]any{
			"quota_exhausted":    true,
			"quota_exhausted_at": now.Add(-time.Hour).Format(time.RFC3339),
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := svc.ApplyQuotaReport(ctx, account, provideradaptercontract.QuotaReport{
		Supported: true,
		FetchedAt: now,
		QuotaSignals: []provideradaptercontract.QuotaSignal{{
			QuotaType:      "codex_5h_percent",
			Remaining:      "35",
			Used:           "65",
			QuotaLimit:     "100",
			RemainingRatio: 0.35,
			SnapshotAt:     now,
		}},
	}); err != nil {
		t.Fatalf("apply quota report: %v", err)
	}
	stored, err := svc.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if stored.Metadata["quota_remaining_ratio"] != 0.35 ||
		stored.Metadata["quota_exhausted"] != nil ||
		stored.Metadata["quota_exhausted_at"] != nil {
		t.Fatalf("unexpected recovered quota metadata: %+v", stored.Metadata)
	}
}

func TestProbeAccountOpensCircuitAfterConsecutiveFailures(t *testing.T) {
	store := accountmemory.New()
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   11,
		Name:         "probe-main",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.RecordHealthSnapshot(ctx, contract.AccountHealthSnapshot{
			AccountID:    account.ID,
			ProviderID:   account.ProviderID,
			Status:       "unhealthy",
			SuccessRate:  0,
			ErrorRate:    1,
			LatencyP50MS: 150 + i,
			LatencyP95MS: 300 + i,
			TimeoutCount: 1,
			CircuitState: "open",
			SnapshotAt:   now.Add(time.Duration(-2+i) * time.Minute),
		}); err != nil {
			t.Fatalf("record history: %v", err)
		}
	}

	snapshot, updated, err := svc.ProbeAccount(ctx, account.ID, fakeAccountProber{
		result: contract.AccountProbeResult{
			OK:         false,
			ErrorClass: "timeout",
			StatusCode: 504,
			LatencyMS:  100,
			CheckedAt:  now,
			Metadata:   map[string]any{"endpoint": "https://provider.test/v1/models"},
		},
	}, contract.AccountProbePolicy{
		HistoryLimit:           3,
		FailureThreshold:       3,
		ErrorRateThreshold:     0.5,
		MinSamplesForErrorRate: 3,
		Cooldown:               2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("probe account: %v", err)
	}
	if snapshot.Status != "unhealthy" || snapshot.CircuitState != "open" || snapshot.CooldownUntil == nil {
		t.Fatalf("expected unhealthy open circuit snapshot, got %+v", snapshot)
	}
	if !snapshot.CooldownUntil.Equal(now.Add(2 * time.Minute)) {
		t.Fatalf("unexpected cooldown_until: %v", snapshot.CooldownUntil)
	}
	if snapshot.SuccessRate != 0 || snapshot.ErrorRate != 1 || snapshot.TimeoutCount != 3 {
		t.Fatalf("unexpected aggregate probe metrics: %+v", snapshot)
	}
	if updated.Metadata["cooldown_active"] != true ||
		updated.Metadata["circuit_open"] != true ||
		updated.Metadata["cooldown_reason"] != "timeout" ||
		updated.Metadata["last_error_class"] != "timeout" ||
		updated.Metadata["consecutive_probe_failures"] != 3 {
		t.Fatalf("expected protective metadata, got %+v", updated.Metadata)
	}
	if updated.Metadata["last_probe_endpoint"] != "https://provider.test/v1/models" {
		t.Fatalf("expected probe metadata to be persisted, got %+v", updated.Metadata)
	}
	if updated.Metadata["last_health_snapshot_id"] != snapshot.ID {
		t.Fatalf("expected latest snapshot id in metadata, got %+v", updated.Metadata)
	}
}

func TestProbeAccountRecordsTraceSpan(t *testing.T) {
	exporter := oteltest.NewExporter(t)
	store := accountmemory.New()
	now := time.Date(2026, 5, 25, 10, 0, 0, 0, time.UTC)
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	account, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   15,
		Name:         "probe-traced",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	_, _, err = svc.ProbeAccount(context.Background(), account.ID, fakeAccountProber{
		result: contract.AccountProbeResult{
			OK:         false,
			ErrorClass: "timeout",
			StatusCode: 504,
			LatencyMS:  150,
			CheckedAt:  now,
		},
	}, contract.AccountProbePolicy{})
	if err != nil {
		t.Fatalf("probe account: %v", err)
	}

	span := oteltest.FindSpan(t, exporter.GetSpans(), "accounts.ProbeAccount")
	oteltest.AssertIntAttr(t, span.Attributes, "srapi.account.id", account.ID)
	oteltest.AssertIntAttr(t, span.Attributes, "srapi.provider.id", account.ProviderID)
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.account.runtime_class", "api_key")
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.account.probe_outcome", "degraded")
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.account.health_status", "degraded")
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.account.circuit_state", "half_open")
	oteltest.AssertStringAttr(t, span.Attributes, "srapi.account.error_class", "timeout")
	oteltest.AssertIntAttr(t, span.Attributes, "srapi.account.probe_latency_ms", 150)
}

func TestProbeAccountSuccessfulProbeClearsProtectionMetadata(t *testing.T) {
	store := accountmemory.New()
	now := time.Date(2026, 5, 25, 10, 5, 0, 0, time.UTC)
	svc, err := New(store, "0123456789abcdef0123456789abcdef", fixedClock{now: now})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   12,
		Name:         "probe-recovered",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		Metadata: map[string]any{
			"cooldown_active":    true,
			"cooldown_reason":    "timeout",
			"cooldown_until":     now.Add(time.Minute).Format(time.RFC3339),
			"circuit_open":       true,
			"last_error_class":   "timeout",
			"last_error_message": "timeout",
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	snapshot, updated, err := svc.ProbeAccount(ctx, account.ID, fakeAccountProber{
		result: contract.AccountProbeResult{
			OK:         true,
			StatusCode: 200,
			LatencyMS:  42,
			CheckedAt:  now,
		},
		wantCredential: "secret-value",
	}, contract.AccountProbePolicy{})
	if err != nil {
		t.Fatalf("probe account: %v", err)
	}
	if snapshot.Status != "healthy" || snapshot.CircuitState != "closed" || snapshot.CooldownUntil != nil {
		t.Fatalf("expected healthy closed circuit snapshot, got %+v", snapshot)
	}
	for _, key := range []string{"cooldown_active", "cooldown_reason", "cooldown_until", "circuit_open", "last_error_class", "last_error_message"} {
		if _, ok := updated.Metadata[key]; ok {
			t.Fatalf("expected %s to be cleared from metadata: %+v", key, updated.Metadata)
		}
	}
	if updated.Metadata["last_probe_ok"] != true || updated.Metadata["health_state"] != "healthy" || updated.Metadata["health_score"] != float32(1) {
		t.Fatalf("expected healthy probe metadata, got %+v", updated.Metadata)
	}
}

func TestAdminAccountLifecycleHelpers(t *testing.T) {
	svc, err := New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "admin-egress",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://proxy-user:proxy-pass@example.invalid:8443",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	proxyID := strconv.Itoa(proxy.ID)
	resetAt := time.Now().UTC().Add(time.Minute).Truncate(time.Second)
	account, err := svc.Create(ctx, contract.CreateRequest{
		ProviderID:   9,
		Name:         "admin-main",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		ProxyID:      &proxyID,
		Metadata: map[string]any{
			"rpm_used":             3,
			"rpm_limit":            10,
			"rpm_window_seconds":   60,
			"rpm_reset_at":         resetAt.Format(time.RFC3339),
			"last_error_class":     "rate_limit",
			"last_error_message":   "too many requests",
			"cooldown_active":      true,
			"proxy_region":         "us-east",
			"egress_ip_hash":       "hash",
			"proxy_sample_count":   4,
			"proxy_success_rate":   0.75,
			"proxy_error_rate":     0.25,
			"proxy_latency_p95_ms": 321,
		},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	rpm, err := svc.RPMStatus(ctx, account.ID)
	if err != nil {
		t.Fatalf("rpm status: %v", err)
	}
	if rpm.RPMUsed != 3 || rpm.RPMLimit == nil || *rpm.RPMLimit != 10 || rpm.WindowSeconds != 60 || rpm.ResetAt == nil || !rpm.ResetAt.Equal(resetAt) {
		t.Fatalf("unexpected rpm status: %+v", rpm)
	}

	quality, err := svc.ProxyQuality(ctx, account.ID)
	if err != nil {
		t.Fatalf("proxy quality: %v", err)
	}
	if quality.ProxyID == nil || *quality.ProxyID != proxyID || quality.SuccessRate != 0.75 || quality.ErrorRate != 0.25 || quality.LatencyP95MS != 321 || quality.SampleCount != 4 {
		t.Fatalf("unexpected proxy quality: %+v", quality)
	}
	if quality.Metadata["proxy_region"] != "us-east" || quality.Metadata["egress_ip_hash"] != "hash" {
		t.Fatalf("unexpected proxy quality metadata: %+v", quality.Metadata)
	}

	cleared, err := svc.ClearErrorState(ctx, account.ID)
	if err != nil {
		t.Fatalf("clear error state: %v", err)
	}
	if cleared.Metadata["last_error_class"] != nil || cleared.Metadata["cooldown_active"] != nil || cleared.Metadata["last_error_cleared_at"] == nil {
		t.Fatalf("expected cleared error metadata, got %+v", cleared.Metadata)
	}

	status := contract.StatusDisabled
	result := svc.BatchUpdateStatus(ctx, []int{account.ID}, status)
	if len(result.Errors) != 0 || len(result.Updated) != 1 || result.Updated[0].Status != status {
		t.Fatalf("unexpected batch status result: %+v", result)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type fakeAccountProber struct {
	result         contract.AccountProbeResult
	wantCredential string
}

func (p fakeAccountProber) ProbeAccount(_ context.Context, _ contract.ProviderAccount, credential map[string]any) (contract.AccountProbeResult, error) {
	if p.wantCredential != "" && credential["api_key"] != p.wantCredential {
		return contract.AccountProbeResult{}, errors.New("unexpected credential")
	}
	return p.result, nil
}

// TestProxyTestPersistsResultInMetadata verifies that TestProxy writes the
// most recent probe outcome onto the proxy's metadata under the reserved
// `_last_test` key, so the list view can render a "last test" badge without
// re-probing on every page load. Uses the same loopback:1 unreachable proxy
// so the outcome is deterministic without a network dependency.
func TestProxyTestPersistsResultInMetadata(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name:     "unreachable",
		Type:     contract.ProxyTypeHTTPS,
		URL:      "https://127.0.0.1:1",
		Metadata: map[string]any{"region": "us-east"},
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	result, err := svc.TestProxy(ctx, proxy.ID, "")
	if err != nil {
		t.Fatalf("test proxy: %v", err)
	}
	if result.OK {
		t.Fatalf("expected ok=false for loopback:1, got %+v", result)
	}

	fresh, err := svc.FindProxyByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("find proxy: %v", err)
	}
	// Untouched user metadata survives the persist write.
	if fresh.Metadata["region"] != "us-east" {
		t.Fatalf("user metadata clobbered: %+v", fresh.Metadata)
	}
	snapshot, ok := fresh.Metadata["_last_test"].(map[string]any)
	if !ok {
		t.Fatalf("expected _last_test metadata to be a map, got %T: %+v", fresh.Metadata["_last_test"], fresh.Metadata["_last_test"])
	}
	if snapshot["ok"] != false {
		t.Fatalf("_last_test.ok: want false, got %v", snapshot["ok"])
	}
	if snapshot["error_class"] == nil || snapshot["error_class"] == "" {
		t.Fatalf("_last_test.error_class should be set: %+v", snapshot)
	}
	if snapshot["at"] == nil {
		t.Fatalf("_last_test.at should be set: %+v", snapshot)
	}
}

// TestBatchTestProxiesReturnsOneRowPerIdAndCategorisesMissing checks that
// BatchTestProxies returns rows in input order, that a missing id is
// reported as a row with error_class="not_found" (not a hard error), and
// that input validation rejects empty / negative ids. Like the single-id
// test, this doesn't assert a successful probe — the loopback:1 proxy
// surfaces a transport_error which is the deterministic outcome.
func TestBatchTestProxiesReturnsOneRowPerIdAndCategorisesMissing(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()

	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "unreachable",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	rows, err := svc.BatchTestProxies(ctx, []int{proxy.ID, 99999, proxy.ID})
	if err != nil {
		t.Fatalf("batch test: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows: want 3 (matching input length), got %d", len(rows))
	}
	if rows[0].ProxyID != proxy.ID || rows[2].ProxyID != proxy.ID {
		t.Fatalf("rows out of input order: %+v", rows)
	}
	if rows[1].ProxyID != 99999 {
		t.Fatalf("rows[1] id mismatch: got %d", rows[1].ProxyID)
	}
	if rows[1].Result.OK || rows[1].Result.ErrorClass != "not_found" {
		t.Fatalf("rows[1]: want ok=false not_found, got %+v", rows[1].Result)
	}
	for i := 0; i < 3; i += 2 {
		if rows[i].Result.OK {
			t.Fatalf("rows[%d]: want ok=false (unreachable proxy), got %+v", i, rows[i].Result)
		}
		if rows[i].Result.ErrorClass == "" {
			t.Fatalf("rows[%d]: want non-empty error_class, got %+v", i, rows[i].Result)
		}
	}

	if _, err := svc.BatchTestProxies(ctx, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty ids: want ErrInvalidInput, got %v", err)
	}
	if _, err := svc.BatchTestProxies(ctx, []int{0}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero id: want ErrInvalidInput, got %v", err)
	}
}

// TestProxyTestErrorClassesCategorizeFailures verifies the wiring of
// Service.TestProxy: bad target_url, bad proxy URL ciphertext, and a
// connection that can't be made all produce ok=false with a stable
// error_class. This test deliberately doesn't exercise a real successful
// probe — Service.TestProxy makes a real HTTP request, and a happy path
// would require either a running proxy or network egress in CI.
func TestProxyTestErrorClassesCategorizeFailures(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()

	// Create a valid-shape proxy that points at a TCP port that is virtually
	// guaranteed to refuse connections (port 1 on the loopback interface).
	// We can't reasonably assert "connection refused" vs "i/o timeout" across
	// every platform — the contract is just ok=false + a non-empty error_class
	// when the transport fails.
	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "unreachable",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	// Bad target_url path — the service should categorize it BEFORE touching
	// the network.
	badTarget, err := svc.TestProxy(ctx, proxy.ID, "not a url")
	if err != nil {
		t.Fatalf("bad target: %v", err)
	}
	if badTarget.OK || badTarget.ErrorClass != "bad_target_url" {
		t.Fatalf("bad target: want ok=false bad_target_url, got %+v", badTarget)
	}

	// Transport failure path. We intentionally use a low timeout in the
	// service so this test runs fast; what we assert is just the shape, not
	// the specific class string.
	tcpFail, err := svc.TestProxy(ctx, proxy.ID, "https://example.invalid/")
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if tcpFail.OK {
		t.Fatalf("transport: want ok=false, got %+v", tcpFail)
	}
	if tcpFail.ErrorClass == "" {
		t.Fatalf("transport: want non-empty error_class, got %+v", tcpFail)
	}
	if tcpFail.TargetURL != "https://example.invalid/" {
		t.Fatalf("transport: target_url not echoed, got %q", tcpFail.TargetURL)
	}

	// Unknown proxy id — the service should propagate the not-found error.
	if _, err := svc.TestProxy(ctx, 99999, ""); err == nil {
		t.Fatalf("expected not-found for unknown proxy id, got nil")
	}
	// Invalid id input should surface ErrInvalidInput rather than reach the store.
	if _, err := svc.TestProxy(ctx, 0, ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for id=0, got %v", err)
	}
}
