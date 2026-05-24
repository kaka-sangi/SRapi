package healthprobe

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
)

const testMasterKey = "health_probe_master_key_32_bytes_min"

func TestRunOnceProbesOnlyActiveAPIKeyAccounts(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC)
	accountStore := accountmemory.New()
	providerStore := providermemory.New()
	providers, err := providerservice.New(providerStore, nil)
	if err != nil {
		t.Fatalf("new provider service: %v", err)
	}
	provider, err := providers.Create(ctx, providercontract.CreateRequest{
		Name:        "openai-compatible",
		DisplayName: "OpenAI Compatible",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
		ConfigSchema: map[string]any{
			"base_url": "https://provider.test/v1",
		},
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	accounts, err := accountservice.New(accountStore, testMasterKey, fixedClock{now: now.Add(-time.Minute)})
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	active, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   provider.ID,
		Name:         "active-api-key",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "active-secret"},
	})
	if err != nil {
		t.Fatalf("create active account: %v", err)
	}
	disabled := accountcontract.StatusDisabled
	if _, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   provider.ID,
		Name:         "disabled-api-key",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "disabled-secret"},
		Status:       &disabled,
	}); err != nil {
		t.Fatalf("create disabled account: %v", err)
	}
	if _, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   provider.ID,
		Name:         "reverse-proxy",
		RuntimeClass: accountcontract.RuntimeClassCustomReverseProxy,
		Credential:   map[string]any{"api_key": "reverse-secret"},
	}); err != nil {
		t.Fatalf("create reverse proxy account: %v", err)
	}

	adapter := &fakeProbeAdapter{
		responses: map[int]provideradaptercontract.ProbeResponse{
			active.ID: {
				OK:         true,
				StatusCode: 200,
				LatencyMS:  31,
				Metadata: map[string]any{
					"endpoint": "https://provider.test/v1/models",
				},
			},
		},
	}
	worker, err := New(accountStore, providerStore, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		MasterKey:     testMasterKey,
		Clock:         fixedClock{now: now},
		Adapter:       adapter,
		MaxConcurrent: 1,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if result.Selected != 3 || result.Skipped != 2 || result.Probed != 1 || result.Failed != 0 || result.Unhealthy != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	calls := adapter.callsSnapshot()
	if len(calls) != 1 || calls[0].Account.ID != active.ID || calls[0].Credential["api_key"] != "active-secret" {
		t.Fatalf("unexpected probe calls: %+v", calls)
	}
	snapshot, err := accounts.LatestHealthSnapshotByAccount(ctx, active.ID)
	if err != nil {
		t.Fatalf("latest health snapshot: %v", err)
	}
	if snapshot.Status != "healthy" || snapshot.CircuitState != "closed" || snapshot.SnapshotAt != now {
		t.Fatalf("unexpected health snapshot: %+v", snapshot)
	}
	updated, err := accounts.FindByID(ctx, active.ID)
	if err != nil {
		t.Fatalf("find updated account: %v", err)
	}
	if updated.Metadata["last_probe_endpoint"] != "https://provider.test/v1/models" ||
		updated.Metadata["last_health_snapshot_id"] != snapshot.ID {
		t.Fatalf("expected probe metadata to be written, got %+v", updated.Metadata)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time { return c.now }

type fakeProbeAdapter struct {
	mu        sync.Mutex
	responses map[int]provideradaptercontract.ProbeResponse
	calls     []provideradaptercontract.ProbeRequest
}

func (a *fakeProbeAdapter) ProbeAccount(_ context.Context, req provideradaptercontract.ProbeRequest) (provideradaptercontract.ProbeResponse, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls = append(a.calls, req)
	if resp, ok := a.responses[req.Account.ID]; ok {
		return resp, nil
	}
	return provideradaptercontract.ProbeResponse{OK: false, ErrorClass: "not_configured", StatusCode: 500}, nil
}

func (a *fakeProbeAdapter) callsSnapshot() []provideradaptercontract.ProbeRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]provideradaptercontract.ProbeRequest, len(a.calls))
	copy(out, a.calls)
	return out
}
