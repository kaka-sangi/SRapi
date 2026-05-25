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

	disabled := contract.ProxyStatusDisabled
	if _, err := svc.UpdateProxy(ctx, proxy.ID, contract.UpdateProxyRequest{Status: &disabled}); err != nil {
		t.Fatalf("disable proxy: %v", err)
	}
	if _, err := svc.ResolveProxyURL(ctx, &id); !errors.Is(err, ErrProxyUnavailable) {
		t.Fatalf("expected unavailable proxy resolution, got %v", err)
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

	proxyID := "proxy-us-east"
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
	proxyID := "proxy-us"
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
