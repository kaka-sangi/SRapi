package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
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
