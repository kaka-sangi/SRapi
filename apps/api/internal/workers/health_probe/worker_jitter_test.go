package healthprobe

import (
	"io"
	"log/slog"
	"sync/atomic"
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

func TestJitterDuration(t *testing.T) {
	if d := jitterDuration(0, func(int64) int64 { return 5 }); d != 0 {
		t.Fatalf("expected 0 for non-positive max, got %v", d)
	}
	if d := jitterDuration(time.Second, nil); d != 0 {
		t.Fatalf("expected 0 for nil randN, got %v", d)
	}
	max := 10 * time.Second
	d := jitterDuration(max, func(n int64) int64 { return n - 1 })
	if d != max-time.Duration(1) {
		t.Fatalf("expected %v, got %v", max-time.Duration(1), d)
	}
	if d < 0 || d >= max {
		t.Fatalf("jitter %v escaped [0, %v)", d, max)
	}
}

func TestBackgroundProbePassJittersButRunOnceDoesNot(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 5, 25, 11, 0, 0, 0, time.UTC)
	accountStore := accountmemory.New()
	providerStore := providermemory.New()
	providers, err := providerservice.New(providerStore, nil)
	if err != nil {
		t.Fatalf("new provider service: %v", err)
	}
	provider, err := providers.Create(ctx, providercontract.CreateRequest{
		Name:         "openai-compatible",
		DisplayName:  "OpenAI Compatible",
		AdapterType:  "openai-compatible",
		Protocol:     "openai-compatible",
		ConfigSchema: map[string]any{"base_url": "https://provider.test/v1"},
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
	adapter := &fakeProbeAdapter{responses: map[int]provideradaptercontract.ProbeResponse{
		active.ID: {OK: true, StatusCode: 200, LatencyMS: 10},
	}}
	worker, err := New(accountStore, providerStore, slog.New(slog.NewTextHandler(io.Discard, nil)), Config{
		MasterKey:     testMasterKey,
		Clock:         fixedClock{now: now},
		Adapter:       adapter,
		MaxConcurrent: 4,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	// Record jitter draws and return 0 so the test never actually sleeps.
	var jitterDraws atomic.Int64
	worker.randN = func(int64) int64 {
		jitterDraws.Add(1)
		return 0
	}

	// The background pass desynchronizes: one bounded jitter draw per eligible account.
	res, err := worker.probePass(ctx, true)
	if err != nil || res.Probed != 1 {
		t.Fatalf("background pass: probed=%d err=%v", res.Probed, err)
	}
	if jitterDraws.Load() != 1 {
		t.Fatalf("expected one jitter draw for one eligible account, got %d", jitterDraws.Load())
	}

	// RunOnce is the deterministic single pass: it never jitters.
	jitterDraws.Store(0)
	res, err = worker.RunOnce(ctx)
	if err != nil || res.Probed != 1 {
		t.Fatalf("run once: probed=%d err=%v", res.Probed, err)
	}
	if jitterDraws.Load() != 0 {
		t.Fatalf("expected RunOnce to skip jitter, got %d draws", jitterDraws.Load())
	}
}
