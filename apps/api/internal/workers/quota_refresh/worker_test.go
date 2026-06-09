package quotarefresh

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	adaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
)

const testMasterKey = "0123456789abcdef0123456789abcdef"

func TestWorkerPersistsForbiddenQuotaErrorMetadataAndSuspendsAccount(t *testing.T) {
	ctx := context.Background()
	accountStore := accountmemory.New()
	providerStore := providermemory.New()
	provider, err := providerStore.Create(ctx, providercontract.CreateStoredProvider{
		Name:        "anthropic",
		DisplayName: "Anthropic",
		AdapterType: "anthropic-compatible",
		Protocol:    "anthropic-compatible",
		Status:      providercontract.StatusActive,
		ConfigSchema: map[string]any{
			"quota_url": "https://example.invalid/api/oauth/usage",
		},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	accountsSvc, err := accountservice.New(accountStore, testMasterKey, nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accountsSvc.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   provider.ID,
		Name:         "anthropic-oauth",
		RuntimeClass: accountcontract.RuntimeClassOauthRefresh,
		Credential:   map[string]any{"access_token": "token"},
		Metadata:     map[string]any{"existing": "kept"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	worker, err := New(accountStore, providerStore, slog.Default(), Config{
		MasterKey:     testMasterKey,
		MaxConcurrent: 1,
		Adapter: forbiddenQuotaAdapter{
			err: adaptercontract.ProviderError{
				Class:      "policy_violation",
				StatusCode: http.StatusForbidden,
				Message:    "policy violation",
				Metadata: map[string]any{
					"validation_url":      "https://console.anthropic.com/validate",
					"provider_error_kind": "policy_violation",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(ctx)
	if err == nil {
		t.Fatalf("expected quota refresh error")
	}
	if result.Failed != 1 || result.Refreshed != 0 || result.Signals != 0 {
		t.Fatalf("unexpected refresh result: %+v", result)
	}
	stored, err := accountStore.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if stored.Status != accountcontract.StatusSuspended {
		t.Fatalf("expected account to be suspended, got %s", stored.Status)
	}
	statusCode, ok := stored.Metadata["last_quota_error_status_code"].(float64)
	if !ok || stored.Metadata["existing"] != "kept" ||
		stored.Metadata["last_quota_error_class"] != "policy_violation" ||
		int(statusCode) != http.StatusForbidden ||
		stored.Metadata["validation_url"] != "https://console.anthropic.com/validate" {
		t.Fatalf("unexpected quota error metadata: %+v", stored.Metadata)
	}
}

type forbiddenQuotaAdapter struct {
	err error
}

func (a forbiddenQuotaAdapter) FetchAccountQuota(context.Context, adaptercontract.ProbeRequest) (adaptercontract.QuotaReport, error) {
	return adaptercontract.QuotaReport{}, a.err
}

func (a forbiddenQuotaAdapter) QuotaConfigured(adaptercontract.ProbeRequest) bool {
	return true
}
