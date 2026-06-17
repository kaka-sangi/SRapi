package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
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

// stepClock is a deterministic Clock that returns the configured time and
// advances on demand. Lets TestRecordProxyProbeWindowReset jump exactly past
// the 7-day window boundary without sleeping.
type stepClock struct{ now time.Time }

func (c *stepClock) Now() time.Time         { return c.now }
func (c *stepClock) advance(d time.Duration) { c.now = c.now.Add(d) }

// TestRecordProxyProbeCountsSuccessAndFailure exercises the rolling
// availability counters: a fresh proxy starts at 0/0, successes/failures
// accumulate, last_probe_latency_ms tracks the most recent OK probe (not
// updated on failures), and ProbeSuccessPct7d rounds correctly across each
// snapshot. Also asserts last_probed_at advances on every call.
func TestRecordProxyProbeCountsSuccessAndFailure(t *testing.T) {
	store := accountmemory.New()
	clock := &stepClock{now: time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)}
	svc, err := New(store, "0123456789abcdef0123456789abcdef", clock)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "rolling",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://proxy.example.invalid:443",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if pct := proxy.ProbeSuccessPct7d(); pct != nil {
		t.Fatalf("fresh proxy availability: want nil, got %d", *pct)
	}

	clock.advance(time.Minute)
	updated, err := svc.RecordProxyProbe(ctx, proxy.ID, true, 123)
	if err != nil {
		t.Fatalf("first probe: %v", err)
	}
	if updated.ProbeSuccessCount != 1 || updated.ProbeFailureCount != 0 {
		t.Fatalf("counts after success: %+v", updated)
	}
	if updated.LastProbeLatencyMs != 123 {
		t.Fatalf("latency snapshot: want 123, got %d", updated.LastProbeLatencyMs)
	}
	if updated.LastProbedAt == nil || !updated.LastProbedAt.Equal(clock.now.UTC()) {
		t.Fatalf("last_probed_at: want %s, got %v", clock.now, updated.LastProbedAt)
	}
	if pct := updated.ProbeSuccessPct7d(); pct == nil || *pct != 100 {
		t.Fatalf("availability after 1 success: want 100, got %v", pct)
	}

	clock.advance(time.Minute)
	// Failed probes must not clobber the latency snapshot from the last success.
	updated, err = svc.RecordProxyProbe(ctx, proxy.ID, false, 9999)
	if err != nil {
		t.Fatalf("second probe: %v", err)
	}
	if updated.ProbeSuccessCount != 1 || updated.ProbeFailureCount != 1 {
		t.Fatalf("counts after failure: %+v", updated)
	}
	if updated.LastProbeLatencyMs != 123 {
		t.Fatalf("failed probe must not clobber latency: got %d", updated.LastProbeLatencyMs)
	}
	if pct := updated.ProbeSuccessPct7d(); pct == nil || *pct != 50 {
		t.Fatalf("availability after 1/2: want 50, got %v", pct)
	}

	// Three more failures to get a 25% bucket and validate rounding.
	for i := 0; i < 2; i++ {
		clock.advance(time.Minute)
		if _, err := svc.RecordProxyProbe(ctx, proxy.ID, false, 0); err != nil {
			t.Fatalf("loop probe: %v", err)
		}
	}
	clock.advance(time.Minute)
	updated, err = svc.RecordProxyProbe(ctx, proxy.ID, false, 0)
	if err != nil {
		t.Fatalf("final failure probe: %v", err)
	}
	if updated.ProbeSuccessCount != 1 || updated.ProbeFailureCount != 4 {
		t.Fatalf("counts after 1S/4F: %+v", updated)
	}
	if pct := updated.ProbeSuccessPct7d(); pct == nil || *pct != 20 {
		t.Fatalf("availability after 1/5: want 20, got %v", pct)
	}
}

// TestRecordProxyProbeWindowResetsAfter7Days verifies the rolling-window
// reset: once 7+ days have elapsed since the last reset marker, the very
// next probe zeros both counters before applying its own outcome so the
// percentage stays current rather than averaging in week-old samples.
func TestRecordProxyProbeWindowResetsAfter7Days(t *testing.T) {
	store := accountmemory.New()
	clock := &stepClock{now: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}
	svc, err := New(store, "0123456789abcdef0123456789abcdef", clock)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "weekly-reset",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://proxy.example.invalid:443",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}

	// Seed three failures so the counters are non-zero before the rollover.
	for i := 0; i < 3; i++ {
		clock.advance(time.Hour)
		if _, err := svc.RecordProxyProbe(ctx, proxy.ID, false, 0); err != nil {
			t.Fatalf("seed probe: %v", err)
		}
	}
	pre, err := svc.FindProxyByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("find pre-rollover: %v", err)
	}
	if pre.ProbeFailureCount != 3 {
		t.Fatalf("pre-rollover failures: want 3, got %d", pre.ProbeFailureCount)
	}

	// Jump just past the 7-day window — the next probe must reset both
	// counters before recording its own success.
	clock.advance(7*24*time.Hour + time.Minute)
	post, err := svc.RecordProxyProbe(ctx, proxy.ID, true, 42)
	if err != nil {
		t.Fatalf("post-rollover probe: %v", err)
	}
	if post.ProbeSuccessCount != 1 || post.ProbeFailureCount != 0 {
		t.Fatalf("counters not reset after 7d: %+v", post)
	}
	if pct := post.ProbeSuccessPct7d(); pct == nil || *pct != 100 {
		t.Fatalf("availability after rollover: want 100, got %v", pct)
	}
}

// TestProxyCountryRoundTrip checks that the operator-supplied country fields
// survive a Create + Update + Find round-trip and that the empty-string
// default produces a nil API pointer (covered indirectly via the contract's
// own field — the response mapping is exercised in the httpserver tests).
func TestProxyCountryRoundTrip(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	code := "us"
	name := "United States"
	created, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name:        "us-east",
		Type:        contract.ProxyTypeHTTPS,
		URL:         "https://proxy.example.invalid:443",
		CountryCode: &code,
		CountryName: &name,
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	if created.CountryCode != "US" {
		t.Fatalf("country_code normalisation: want US, got %q", created.CountryCode)
	}
	if created.CountryName != "United States" {
		t.Fatalf("country_name: want United States, got %q", created.CountryName)
	}

	newCode := "cn"
	updated, err := svc.UpdateProxy(ctx, created.ID, contract.UpdateProxyRequest{CountryCode: &newCode})
	if err != nil {
		t.Fatalf("update proxy: %v", err)
	}
	if updated.CountryCode != "CN" {
		t.Fatalf("country_code update: want CN, got %q", updated.CountryCode)
	}
}

func TestBatchCreateAccountsAllSuccess(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{
		ProviderID:   1,
		RuntimeClass: contract.RuntimeClassAPIKey,
	}
	items := []contract.BatchAccountItem{
		{Name: "fleet-a", Credential: map[string]any{"api_key": "k-a"}},
		{Name: "fleet-b", Credential: map[string]any{"api_key": "k-b"}},
		{Name: "fleet-c", Credential: map[string]any{"api_key": "k-c"}},
	}
	results, err := svc.BatchCreateAccounts(context.Background(), defaults, items)
	if err != nil {
		t.Fatalf("BatchCreateAccounts: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" || row.AccountID == nil {
			t.Fatalf("row %d unexpectedly failed: %+v", i, row)
		}
	}
	all, _ := svc.List(context.Background())
	if len(all) != 3 {
		t.Fatalf("expected 3 stored accounts, got %d", len(all))
	}
}

func TestBatchCreateAccountsPerRowValidationSurfaces(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{
		ProviderID:   1,
		RuntimeClass: contract.RuntimeClassAPIKey,
	}
	items := []contract.BatchAccountItem{
		{Name: "ok", Credential: map[string]any{"api_key": "k1"}},
		{Name: "", Credential: map[string]any{"api_key": "k2"}},       // bad name
		{Name: "no-cred", Credential: map[string]any{}},                // missing credential
		{Name: "ok2", Credential: map[string]any{"api_key": "k4"}},
	}
	results, err := svc.BatchCreateAccounts(context.Background(), defaults, items)
	if err != nil {
		t.Fatalf("BatchCreateAccounts unexpected outer error: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].AccountID == nil || results[0].Error != "" {
		t.Fatalf("row 0 should succeed, got %+v", results[0])
	}
	if results[1].Error == "" || results[1].AccountID != nil {
		t.Fatalf("row 1 (empty name) should fail, got %+v", results[1])
	}
	if results[2].Error == "" || results[2].AccountID != nil {
		t.Fatalf("row 2 (no credential) should fail, got %+v", results[2])
	}
	if results[3].AccountID == nil || results[3].Error != "" {
		t.Fatalf("row 3 should succeed (subsequent rows not aborted), got %+v", results[3])
	}
}

func TestBatchCreateAccountsDeduplicatesInBatch(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{
		ProviderID:   1,
		RuntimeClass: contract.RuntimeClassAPIKey,
	}
	items := []contract.BatchAccountItem{
		{Name: "dup", Credential: map[string]any{"api_key": "k-1"}},
		{Name: "Dup", Credential: map[string]any{"api_key": "k-2"}}, // case-insensitive dup
		{Name: "ok", Credential: map[string]any{"api_key": "k-3"}},
	}
	results, err := svc.BatchCreateAccounts(context.Background(), defaults, items)
	if err != nil {
		t.Fatalf("BatchCreateAccounts: %v", err)
	}
	if results[0].Error != "" || results[0].AccountID == nil {
		t.Fatalf("first occurrence should win, got %+v", results[0])
	}
	if results[1].Error == "" || results[1].AccountID != nil {
		t.Fatalf("second occurrence should be flagged duplicate, got %+v", results[1])
	}
	if !strings.Contains(strings.ToLower(results[1].Error), "duplicate") {
		t.Fatalf("expected duplicate error, got %q", results[1].Error)
	}
	if results[2].AccountID == nil {
		t.Fatalf("third unique row should succeed, got %+v", results[2])
	}
}

func TestBatchCreateAccountsRejectsExistingName(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := svc.Create(context.Background(), contract.CreateRequest{
		ProviderID:   1,
		Name:         "preexisting",
		RuntimeClass: contract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "old"},
	}); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{
		ProviderID:   1,
		RuntimeClass: contract.RuntimeClassAPIKey,
	}
	items := []contract.BatchAccountItem{
		{Name: "preexisting", Credential: map[string]any{"api_key": "new"}},
		{Name: "newcomer", Credential: map[string]any{"api_key": "new2"}},
	}
	results, err := svc.BatchCreateAccounts(context.Background(), defaults, items)
	if err != nil {
		t.Fatalf("BatchCreateAccounts: %v", err)
	}
	if results[0].Error == "" || results[0].AccountID != nil {
		t.Fatalf("row 0 should reject preexisting, got %+v", results[0])
	}
	if results[1].AccountID == nil {
		t.Fatalf("row 1 should succeed, got %+v", results[1])
	}
}

func TestBatchCreateAccountsRejectsEmptyAndOversize(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{
		ProviderID:   1,
		RuntimeClass: contract.RuntimeClassAPIKey,
	}
	if _, err := svc.BatchCreateAccounts(context.Background(), defaults, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty items should be rejected, got %v", err)
	}
	tooMany := make([]contract.BatchAccountItem, BatchCreateAccountsMaxItems+1)
	for i := range tooMany {
		tooMany[i] = contract.BatchAccountItem{Name: "x" + strconv.Itoa(i), Credential: map[string]any{"api_key": "k"}}
	}
	if _, err := svc.BatchCreateAccounts(context.Background(), defaults, tooMany); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("oversize batch should be rejected, got %v", err)
	}
}

func TestBatchCreateAccountsRejectsInvalidDefaults(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	items := []contract.BatchAccountItem{
		{Name: "ok", Credential: map[string]any{"api_key": "k"}},
	}
	// Missing provider id.
	if _, err := svc.BatchCreateAccounts(context.Background(), contract.BatchCreateAccountsDefaults{
		RuntimeClass: contract.RuntimeClassAPIKey,
	}, items); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("missing provider id should fail, got %v", err)
	}
	// Bad runtime class.
	if _, err := svc.BatchCreateAccounts(context.Background(), contract.BatchCreateAccountsDefaults{
		ProviderID:   1,
		RuntimeClass: contract.RuntimeClass("bogus"),
	}, items); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("bad runtime class should fail, got %v", err)
	}
}

// TestBatchDeleteAccountsAllSuccess pins the happy path: all ids exist,
// all get soft-deleted, results carry no error rows.
func TestBatchDeleteAccountsAllSuccess(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := []contract.BatchAccountItem{
		{Name: "del-a", Credential: map[string]any{"api_key": "k-a"}},
		{Name: "del-b", Credential: map[string]any{"api_key": "k-b"}},
	}
	created, _ := svc.BatchCreateAccounts(context.Background(), defaults, items)
	ids := []int{*created[0].AccountID, *created[1].AccountID}
	results, err := svc.BatchDeleteAccounts(context.Background(), ids)
	if err != nil {
		t.Fatalf("BatchDeleteAccounts: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d unexpectedly failed: %+v", i, row)
		}
	}
	all, _ := svc.List(context.Background())
	if len(all) != 0 {
		t.Fatalf("expected 0 stored accounts after delete, got %d", len(all))
	}
}

// TestBatchDeleteAccountsIsIdempotentOnMissingIds pins the idempotent
// semantics: NotFound on a row is NOT surfaced as an error since the
// caller's intent ("this id should not exist") is already true. Mix
// existing + missing ids and assert all rows come back error-free.
func TestBatchDeleteAccountsIsIdempotentOnMissingIds(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := []contract.BatchAccountItem{{Name: "real-one", Credential: map[string]any{"api_key": "k"}}}
	created, _ := svc.BatchCreateAccounts(context.Background(), defaults, items)
	realID := *created[0].AccountID

	results, err := svc.BatchDeleteAccounts(context.Background(), []int{realID, 9999, 8888})
	if err != nil {
		t.Fatalf("BatchDeleteAccounts: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d (id=%d) should be success (idempotent NotFound), got error %q", i, row.AccountID, row.Error)
		}
	}
}

// TestBatchDeleteAccountsDedupesWithinBatch: an accidental double-id must
// surface in the second occurrence's Error, not as a silent second
// NotFound. Pin the contract.
func TestBatchDeleteAccountsDedupesWithinBatch(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := []contract.BatchAccountItem{{Name: "dup-target", Credential: map[string]any{"api_key": "k"}}}
	created, _ := svc.BatchCreateAccounts(context.Background(), defaults, items)
	realID := *created[0].AccountID

	results, err := svc.BatchDeleteAccounts(context.Background(), []int{realID, realID})
	if err != nil {
		t.Fatalf("BatchDeleteAccounts: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("first occurrence should succeed: %+v", results[0])
	}
	if results[1].Error != "duplicate id in batch" {
		t.Fatalf("second occurrence should report duplicate; got: %+v", results[1])
	}
}

// TestBatchDeleteAccountsRejectsEmptyAndOversize: outer error guards.
// Per-row failures stay in the result slice; precondition violations
// (zero or > MaxItems) return ErrInvalidInput without touching anything.
func TestBatchDeleteAccountsRejectsEmptyAndOversize(t *testing.T) {
	store := accountmemory.New()
	svc, _ := New(store, "0123456789abcdef0123456789abcdef", nil)
	if _, err := svc.BatchDeleteAccounts(context.Background(), nil); err != ErrInvalidInput {
		t.Fatalf("empty ids should ErrInvalidInput, got %v", err)
	}
	oversize := make([]int, BatchDeleteAccountsMaxItems+1)
	for i := range oversize {
		oversize[i] = i + 1
	}
	if _, err := svc.BatchDeleteAccounts(context.Background(), oversize); err != ErrInvalidInput {
		t.Fatalf(">MaxItems ids should ErrInvalidInput, got %v", err)
	}
}

// helper to bootstrap a service + group + N accounts for the BatchAdd/Remove tests.
func setupGroupAndAccounts(t *testing.T, count int) (*Service, int, []int) {
	t.Helper()
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	group, err := svc.CreateGroup(ctx, contract.CreateGroupRequest{Name: "g"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := make([]contract.BatchAccountItem, count)
	for i := 0; i < count; i++ {
		items[i] = contract.BatchAccountItem{
			Name:       "acct-" + strconv.Itoa(i),
			Credential: map[string]any{"api_key": "k-" + strconv.Itoa(i)},
		}
	}
	results, _ := svc.BatchCreateAccounts(ctx, defaults, items)
	ids := make([]int, count)
	for i, r := range results {
		ids[i] = *r.AccountID
	}
	return svc, group.ID, ids
}

func TestBatchAddAccountsToGroupAllSuccess(t *testing.T) {
	svc, groupID, ids := setupGroupAndAccounts(t, 3)
	results, err := svc.BatchAddAccountsToGroup(context.Background(), groupID, ids)
	if err != nil {
		t.Fatalf("BatchAddAccountsToGroup: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d: %+v", i, row)
		}
	}
	members, _ := svc.ListGroupMembers(context.Background(), groupID)
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}
}

// Re-adding existing members is silent success (idempotent). Re-running
// the same batch produces zero error rows.
func TestBatchAddAccountsToGroupIsIdempotentOnAlreadyMember(t *testing.T) {
	svc, groupID, ids := setupGroupAndAccounts(t, 2)
	ctx := context.Background()
	if _, err := svc.BatchAddAccountsToGroup(ctx, groupID, ids); err != nil {
		t.Fatalf("initial add: %v", err)
	}
	results, err := svc.BatchAddAccountsToGroup(ctx, groupID, ids)
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	for i, row := range results {
		if row.Error != "" {
			t.Errorf("row %d should be idempotent success, got %q", i, row.Error)
		}
	}
}

// Mixed valid + non-existent ids: real ones succeed, missing ids surface
// per-row without aborting the batch.
func TestBatchAddAccountsToGroupSurfacesPerRowFailures(t *testing.T) {
	svc, groupID, ids := setupGroupAndAccounts(t, 1)
	mixed := []int{ids[0], 99999, 88888}
	results, err := svc.BatchAddAccountsToGroup(context.Background(), groupID, mixed)
	if err != nil {
		t.Fatalf("BatchAddAccountsToGroup: %v", err)
	}
	if results[0].Error != "" {
		t.Errorf("real id should succeed, got %q", results[0].Error)
	}
	if results[1].Error == "" || results[2].Error == "" {
		t.Errorf("missing ids should surface as per-row errors, got %+v", results)
	}
}

// Double-id within the batch: first occurrence wins, second flagged
// "duplicate" — so an accidental double-add doesn't silently re-add.
func TestBatchAddAccountsToGroupDedupesWithinBatch(t *testing.T) {
	svc, groupID, ids := setupGroupAndAccounts(t, 1)
	results, _ := svc.BatchAddAccountsToGroup(context.Background(), groupID, []int{ids[0], ids[0]})
	if results[0].Error != "" {
		t.Errorf("first row should succeed: %+v", results[0])
	}
	if results[1].Error != "duplicate account id in batch" {
		t.Errorf("second row should be duplicate: %+v", results[1])
	}
}

// Outer guards: zero ids, > MaxItems, missing group all return
// ErrInvalidInput before any per-row work runs.
func TestBatchAddAccountsToGroupRejectsBadOuterInput(t *testing.T) {
	svc, groupID, _ := setupGroupAndAccounts(t, 0)
	if _, err := svc.BatchAddAccountsToGroup(context.Background(), groupID, nil); err != ErrInvalidInput {
		t.Errorf("empty ids: got %v, want ErrInvalidInput", err)
	}
	oversize := make([]int, BatchGroupMembersMaxItems+1)
	for i := range oversize {
		oversize[i] = i + 1
	}
	if _, err := svc.BatchAddAccountsToGroup(context.Background(), groupID, oversize); err != ErrInvalidInput {
		t.Errorf("oversize: got %v, want ErrInvalidInput", err)
	}
	if _, err := svc.BatchAddAccountsToGroup(context.Background(), 9999, []int{1}); err == nil {
		t.Errorf("non-existent group should error")
	}
}

// Remove: same idempotent semantics. Not-member rows count as success.
func TestBatchRemoveAccountsFromGroupIsIdempotentOnNotMember(t *testing.T) {
	svc, groupID, ids := setupGroupAndAccounts(t, 2)
	ctx := context.Background()
	// Add only one of the two ids.
	_, _ = svc.BatchAddAccountsToGroup(ctx, groupID, []int{ids[0]})
	// Remove both — the never-added one is a silent success.
	results, err := svc.BatchRemoveAccountsFromGroup(ctx, groupID, ids)
	if err != nil {
		t.Fatalf("BatchRemoveAccountsFromGroup: %v", err)
	}
	for i, row := range results {
		if row.Error != "" {
			t.Errorf("row %d should be idempotent success, got %q", i, row.Error)
		}
	}
	members, _ := svc.ListGroupMembers(ctx, groupID)
	if len(members) != 0 {
		t.Errorf("expected 0 members after batch-remove, got %d", len(members))
	}
}

// TestBatchUpdateConcurrencyAllSuccess pins the happy path: every item has
// a valid id + non-negative concurrency, and the resulting metadata carries
// the right value on every row.
func TestBatchUpdateConcurrencyAllSuccess(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := []contract.BatchAccountItem{
		{Name: "con-a", Credential: map[string]any{"api_key": "k-a"}},
		{Name: "con-b", Credential: map[string]any{"api_key": "k-b"}},
	}
	created, _ := svc.BatchCreateAccounts(context.Background(), defaults, items)
	updates := []contract.BatchUpdateConcurrencyItem{
		{AccountID: *created[0].AccountID, MaxConcurrency: 4},
		{AccountID: *created[1].AccountID, MaxConcurrency: 7},
	}
	results, err := svc.BatchUpdateConcurrency(context.Background(), updates)
	if err != nil {
		t.Fatalf("BatchUpdateConcurrency: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d unexpectedly failed: %+v", i, row)
		}
	}
	for i, target := range []int{4, 7} {
		acct, err := svc.store.FindByID(context.Background(), *created[i].AccountID)
		if err != nil {
			t.Fatalf("lookup created[%d]: %v", i, err)
		}
		raw, ok := acct.Metadata["max_concurrency"]
		if !ok {
			t.Fatalf("metadata[%d] missing max_concurrency: %+v", i, acct.Metadata)
		}
		// Memory store stores the value as int; an SQL backend may round-trip
		// it through a numeric JSON column, so accept both int and float64.
		var got int
		switch v := raw.(type) {
		case int:
			got = v
		case int64:
			got = int(v)
		case float64:
			got = int(v)
		default:
			t.Fatalf("metadata[%d] max_concurrency unexpected type %T", i, raw)
		}
		if got != target {
			t.Fatalf("metadata[%d] max_concurrency: want %d got %d", i, target, got)
		}
	}
}

// TestBatchUpdateConcurrencyPerRowFailureSurfaces pins the per-row error
// contract: an invalid id reports an Error without aborting the rest of the
// batch. Mirrors batch-delete's shape so the admin UI renders mixed outcomes
// the same way.
func TestBatchUpdateConcurrencyPerRowFailureSurfaces(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := []contract.BatchAccountItem{{Name: "good", Credential: map[string]any{"api_key": "k"}}}
	created, _ := svc.BatchCreateAccounts(context.Background(), defaults, items)
	updates := []contract.BatchUpdateConcurrencyItem{
		{AccountID: *created[0].AccountID, MaxConcurrency: 5},
		{AccountID: 0, MaxConcurrency: 1},                       // invalid id
		{AccountID: *created[0].AccountID + 999, MaxConcurrency: 1}, // missing id → idempotent
		{AccountID: 12345, MaxConcurrency: -1},                  // invalid value
	}
	results, err := svc.BatchUpdateConcurrency(context.Background(), updates)
	if err != nil {
		t.Fatalf("BatchUpdateConcurrency: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("row 0 should succeed: %+v", results[0])
	}
	if results[1].Error == "" || !strings.Contains(results[1].Error, "invalid id") {
		t.Fatalf("row 1 should report invalid id, got %+v", results[1])
	}
	if results[2].Error != "" {
		t.Fatalf("row 2 (missing id) should be idempotent success, got %+v", results[2])
	}
	if results[3].Error == "" || !strings.Contains(results[3].Error, "max_concurrency must be >= 0") {
		t.Fatalf("row 3 should report invalid value, got %+v", results[3])
	}
}

// TestBatchUpdateConcurrencyDedupesWithinBatch: an accidental double-id must
// surface as a duplicate on the second occurrence, not as a silent re-apply.
// Pins the same dedup contract as batch-delete.
func TestBatchUpdateConcurrencyDedupesWithinBatch(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defaults := contract.BatchCreateAccountsDefaults{ProviderID: 1, RuntimeClass: contract.RuntimeClassAPIKey}
	items := []contract.BatchAccountItem{{Name: "dup", Credential: map[string]any{"api_key": "k"}}}
	created, _ := svc.BatchCreateAccounts(context.Background(), defaults, items)
	realID := *created[0].AccountID
	updates := []contract.BatchUpdateConcurrencyItem{
		{AccountID: realID, MaxConcurrency: 3},
		{AccountID: realID, MaxConcurrency: 9},
	}
	results, err := svc.BatchUpdateConcurrency(context.Background(), updates)
	if err != nil {
		t.Fatalf("BatchUpdateConcurrency: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("first occurrence should succeed: %+v", results[0])
	}
	if results[1].Error != "duplicate id in batch" {
		t.Fatalf("second occurrence should report duplicate, got: %+v", results[1])
	}
}

// TestBatchUpdateConcurrencyRejectsEmptyAndOversize: outer error guards.
// Empty / > MaxItems return ErrInvalidInput without touching the store.
func TestBatchUpdateConcurrencyRejectsEmptyAndOversize(t *testing.T) {
	store := accountmemory.New()
	svc, _ := New(store, "0123456789abcdef0123456789abcdef", nil)
	if _, err := svc.BatchUpdateConcurrency(context.Background(), nil); err != ErrInvalidInput {
		t.Fatalf("empty should ErrInvalidInput, got %v", err)
	}
	oversize := make([]contract.BatchUpdateConcurrencyItem, BatchUpdateConcurrencyMaxItems+1)
	for i := range oversize {
		oversize[i] = contract.BatchUpdateConcurrencyItem{AccountID: i + 1, MaxConcurrency: 1}
	}
	if _, err := svc.BatchUpdateConcurrency(context.Background(), oversize); err != ErrInvalidInput {
		t.Fatalf(">MaxItems should ErrInvalidInput, got %v", err)
	}
}

// TestBatchSetGroupRateMultipliersAllSuccess pins the happy path: every
// group gets its multiplier updated and the resulting AccountGroup carries
// the normalized (8-decimal) value.
func TestBatchSetGroupRateMultipliersAllSuccess(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	g1, err := svc.CreateGroup(context.Background(), contract.CreateGroupRequest{Name: "rm-a"})
	if err != nil {
		t.Fatalf("create group 1: %v", err)
	}
	g2, err := svc.CreateGroup(context.Background(), contract.CreateGroupRequest{Name: "rm-b"})
	if err != nil {
		t.Fatalf("create group 2: %v", err)
	}
	items := []contract.BatchSetGroupRateMultiplierItem{
		{GroupID: g1.ID, Multiplier: "0.5"},
		{GroupID: g2.ID, Multiplier: "1.5"},
	}
	results, err := svc.BatchSetGroupRateMultipliers(context.Background(), items)
	if err != nil {
		t.Fatalf("BatchSetGroupRateMultipliers: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, row := range results {
		if row.Error != "" {
			t.Fatalf("row %d failed: %+v", i, row)
		}
	}
	got1, _ := svc.FindGroupByID(context.Background(), g1.ID)
	if got1.RateMultiplier != "0.50000000" {
		t.Fatalf("group 1 multiplier: want 0.50000000 got %q", got1.RateMultiplier)
	}
}

// TestBatchSetGroupRateMultipliersPerRowFailureSurfaces pins the per-row
// error contract: invalid id, invalid multiplier, and missing-group all
// surface in the result row without aborting the batch. Missing-group is
// idempotent (matches the other batch ops).
func TestBatchSetGroupRateMultipliersPerRowFailureSurfaces(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	g1, _ := svc.CreateGroup(context.Background(), contract.CreateGroupRequest{Name: "x"})
	items := []contract.BatchSetGroupRateMultiplierItem{
		{GroupID: g1.ID, Multiplier: "1.25"},
		{GroupID: 0, Multiplier: "1"},      // invalid id
		{GroupID: g1.ID + 999, Multiplier: "1"}, // missing → idempotent success
		{GroupID: 88, Multiplier: "-1"},    // invalid value
		{GroupID: 99, Multiplier: "0"},     // zero is forbidden (sub2api: > 0)
	}
	results, err := svc.BatchSetGroupRateMultipliers(context.Background(), items)
	if err != nil {
		t.Fatalf("BatchSetGroupRateMultipliers: %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	if results[0].Error != "" {
		t.Fatalf("row 0 should succeed: %+v", results[0])
	}
	if results[1].Error == "" {
		t.Fatalf("row 1 invalid id should fail: %+v", results[1])
	}
	if results[2].Error != "" {
		t.Fatalf("row 2 (missing id) should be idempotent success, got %+v", results[2])
	}
	if results[3].Error == "" {
		t.Fatalf("row 3 (negative) should fail: %+v", results[3])
	}
	if results[4].Error == "" || !strings.Contains(results[4].Error, "> 0") {
		t.Fatalf("row 4 (zero) should fail with >0 message, got %+v", results[4])
	}
}

// TestBatchSetGroupRateMultipliersDedupesWithinBatch: double-id surfaces as
// duplicate on the second occurrence.
func TestBatchSetGroupRateMultipliersDedupesWithinBatch(t *testing.T) {
	store := accountmemory.New()
	svc, err := New(store, "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	g, _ := svc.CreateGroup(context.Background(), contract.CreateGroupRequest{Name: "dup"})
	items := []contract.BatchSetGroupRateMultiplierItem{
		{GroupID: g.ID, Multiplier: "1.5"},
		{GroupID: g.ID, Multiplier: "2.0"},
	}
	results, err := svc.BatchSetGroupRateMultipliers(context.Background(), items)
	if err != nil {
		t.Fatalf("BatchSetGroupRateMultipliers: %v", err)
	}
	if results[0].Error != "" {
		t.Fatalf("first should succeed: %+v", results[0])
	}
	if results[1].Error != "duplicate id in batch" {
		t.Fatalf("second should report duplicate, got %+v", results[1])
	}
}

// TestBatchSetGroupRateMultipliersRejectsEmptyAndOversize: outer guards.
func TestBatchSetGroupRateMultipliersRejectsEmptyAndOversize(t *testing.T) {
	store := accountmemory.New()
	svc, _ := New(store, "0123456789abcdef0123456789abcdef", nil)
	if _, err := svc.BatchSetGroupRateMultipliers(context.Background(), nil); err != ErrInvalidInput {
		t.Fatalf("empty should ErrInvalidInput, got %v", err)
	}
	oversize := make([]contract.BatchSetGroupRateMultiplierItem, BatchSetGroupRateMultipliersMaxItems+1)
	for i := range oversize {
		oversize[i] = contract.BatchSetGroupRateMultiplierItem{GroupID: i + 1, Multiplier: "1.0"}
	}
	if _, err := svc.BatchSetGroupRateMultipliers(context.Background(), oversize); err != ErrInvalidInput {
		t.Fatalf(">MaxItems should ErrInvalidInput, got %v", err)
	}
}

// TestRecordProxyProbeSerializesConcurrentCallsPerProxy is the regression
// guard for the SQL-backend TOCTOU race in RecordProxyProbe: two probes for
// the same proxy that both observe "no window reset needed" must both
// increment the counter — not have one overwrite the other. Fires N
// goroutines at the same proxy concurrently and asserts the final
// success+failure tally equals N.
func TestRecordProxyProbeSerializesConcurrentCallsPerProxy(t *testing.T) {
	store := accountmemory.New()
	clock := &stepClock{now: time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)}
	svc, err := New(store, "0123456789abcdef0123456789abcdef", clock)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ctx := context.Background()
	proxy, err := svc.CreateProxy(ctx, contract.CreateProxyRequest{
		Name: "race",
		Type: contract.ProxyTypeHTTPS,
		URL:  "https://proxy.example.invalid:443",
	})
	if err != nil {
		t.Fatalf("create proxy: %v", err)
	}
	const goroutines = 16
	var wg syncwait
	wg.Add(goroutines)
	// Half successes, half failures, fired in parallel.
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			ok := i%2 == 0
			if _, err := svc.RecordProxyProbe(ctx, proxy.ID, ok, 100); err != nil {
				t.Errorf("probe goroutine %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	persisted, err := svc.FindProxyByID(ctx, proxy.ID)
	if err != nil {
		t.Fatalf("find proxy: %v", err)
	}
	if total := persisted.ProbeSuccessCount + persisted.ProbeFailureCount; total != goroutines {
		t.Fatalf("expected success+failure=%d after concurrent probes, got %d (s=%d f=%d)",
			goroutines, total, persisted.ProbeSuccessCount, persisted.ProbeFailureCount)
	}
	if persisted.ProbeSuccessCount != goroutines/2 || persisted.ProbeFailureCount != goroutines/2 {
		t.Fatalf("expected %d success and %d failure, got s=%d f=%d",
			goroutines/2, goroutines/2, persisted.ProbeSuccessCount, persisted.ProbeFailureCount)
	}
}

// syncwait is a tiny alias for sync.WaitGroup used by the concurrent
// regression test. Pulling sync into the existing imports rather than the
// test helpers section keeps the test self-contained.
type syncwait = sync.WaitGroup
