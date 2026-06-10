package channelmonitor

import (
	"context"
	"log/slog"
	"strconv"
	"testing"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	channelmonitorscontract "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	channelmonitorsservice "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/service"
	channelmonitorsmemory "github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/store/memory"
	modelmemory "github.com/srapi/srapi/apps/api/internal/modules/models/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
)

const testMasterKey = "0123456789abcdef0123456789abcdef"

func TestWorkerRunsDueEnabledMonitorWithScheduledTrigger(t *testing.T) {
	ctx := context.Background()
	accounts := accountmemory.New()
	providers := providermemory.New()
	models := modelmemory.New()
	monitors := channelmonitorsmemory.New()

	providerSvc, err := providerservice.New(providers, nil)
	if err != nil {
		t.Fatalf("new provider service: %v", err)
	}
	provider, err := providerSvc.Create(ctx, providercontract.CreateRequest{
		Name:        "upstream",
		DisplayName: "Upstream",
		AdapterType: "openai-compatible",
		Protocol:    "openai-compatible",
	})
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}
	accountSvc, err := accountservice.New(accounts, testMasterKey, nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accountSvc.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   provider.ID,
		Name:         "account",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	monitorSvc, err := channelmonitorsservice.New(monitors)
	if err != nil {
		t.Fatalf("new monitor service: %v", err)
	}
	due, err := monitorSvc.CreateDefinition(ctx, channelmonitorscontract.CreateDefinition{
		Name:     "due",
		Enabled:  true,
		Scope:    channelmonitorscontract.ScopeAccount,
		ScopeRef: strconv.Itoa(account.ID),
		Interval: 30,
	})
	if err != nil {
		t.Fatalf("create due monitor: %v", err)
	}
	disabled, err := monitorSvc.CreateDefinition(ctx, channelmonitorscontract.CreateDefinition{
		Name:     "disabled",
		Enabled:  false,
		Scope:    channelmonitorscontract.ScopeAccount,
		ScopeRef: strconv.Itoa(account.ID),
		Interval: 30,
	})
	if err != nil {
		t.Fatalf("create disabled monitor: %v", err)
	}
	adapter := &fakeProbeAdapter{}
	worker, err := New(accounts, providers, models, monitors, slog.Default(), Config{MasterKey: testMasterKey, Adapter: adapter})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	ran, err := worker.RunOnce(ctx)
	if err != nil {
		t.Fatalf("run once: %v", err)
	}
	if ran != 1 || adapter.calls != 1 {
		t.Fatalf("expected one due monitor run and one probe call, ran=%d calls=%d", ran, adapter.calls)
	}
	runs, err := monitorSvc.ListRuns(ctx, due.ID, 10)
	if err != nil {
		t.Fatalf("list due runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Trigger != channelmonitorscontract.TriggerScheduled || runs[0].CheckedCount != 1 || runs[0].Results[0].AccountID != account.ID {
		t.Fatalf("unexpected scheduled run: %+v", runs)
	}
	disabledRuns, err := monitorSvc.ListRuns(ctx, disabled.ID, 10)
	if err != nil {
		t.Fatalf("list disabled runs: %v", err)
	}
	if len(disabledRuns) != 0 {
		t.Fatalf("expected disabled monitor to be skipped, got %+v", disabledRuns)
	}
}

type fakeProbeAdapter struct {
	calls int
}

func (a *fakeProbeAdapter) ProbeAccount(_ context.Context, req provideradaptercontract.ProbeRequest) (provideradaptercontract.ProbeResponse, error) {
	a.calls++
	if req.Credential["api_key"] != "secret" {
		return provideradaptercontract.ProbeResponse{OK: false, StatusCode: 401, ErrorClass: "bad_credential"}, nil
	}
	return provideradaptercontract.ProbeResponse{OK: true, StatusCode: 200, LatencyMS: 3}, nil
}
