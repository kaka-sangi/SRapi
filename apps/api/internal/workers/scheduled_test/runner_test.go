package scheduledtest

import (
	"context"
	"net/http"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	providerservice "github.com/srapi/srapi/apps/api/internal/modules/providers/service"
	providermemory "github.com/srapi/srapi/apps/api/internal/modules/providers/store/memory"
	scheduledcontract "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/contract"
	scheduledservice "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/service"
	scheduledmemory "github.com/srapi/srapi/apps/api/internal/modules/scheduled_tests/store/memory"
)

const testMasterKey = "0123456789abcdef0123456789abcdef"

// fixedProber returns a fixed probe result regardless of the account.
type fixedProber struct {
	result accountcontract.AccountProbeResult
}

func (p fixedProber) ProbeAccount(_ context.Context, _ accountcontract.ProviderAccount, _ map[string]any) (accountcontract.AccountProbeResult, error) {
	return p.result, nil
}

func okProberFactory() BuildProber {
	return func(_ providercontract.Provider, _ string) accountcontract.AccountProber {
		return fixedProber{result: accountcontract.AccountProbeResult{OK: true, StatusCode: http.StatusOK, LatencyMS: 12, CheckedAt: time.Now().UTC()}}
	}
}

func setupRunner(t *testing.T, build BuildProber) (*Runner, *accountservice.Service, *scheduledservice.Service) {
	t.Helper()
	accountsSvc, err := accountservice.New(accountmemory.New(), testMasterKey, nil)
	if err != nil {
		t.Fatalf("accounts svc: %v", err)
	}
	providersSvc, err := providerservice.New(providermemory.New(), nil)
	if err != nil {
		t.Fatalf("providers svc: %v", err)
	}
	if _, err := providersSvc.Create(context.Background(), providercontract.CreateRequest{
		Name:        "openai",
		DisplayName: "OpenAI",
		AdapterType: "openai",
		Protocol:    "openai",
	}); err != nil {
		t.Fatalf("create provider: %v", err)
	}
	plansSvc, err := scheduledservice.New(scheduledmemory.New(), nil)
	if err != nil {
		t.Fatalf("plans svc: %v", err)
	}
	runner, err := NewRunner(accountsSvc, providersSvc, plansSvc, build)
	if err != nil {
		t.Fatalf("new runner: %v", err)
	}
	return runner, accountsSvc, plansSvc
}

func createAccount(t *testing.T, accounts *accountservice.Service, status accountcontract.Status) accountcontract.ProviderAccount {
	t.Helper()
	account, err := accounts.Create(context.Background(), accountcontract.CreateRequest{
		ProviderID:   1,
		Name:         "acct",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret-value"},
		Status:       &status,
		Metadata:     map[string]any{"test_model": "gpt-test"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return account
}

func TestRunPlanProbesActiveAccountScope(t *testing.T) {
	ctx := context.Background()
	runner, accounts, plans := setupRunner(t, okProberFactory())
	account := createAccount(t, accounts, accountcontract.StatusActive)

	scopeID := account.ID
	plan, err := plans.CreatePlan(ctx, scheduledcontract.CreatePlan{
		Name:      "single",
		Enabled:   true,
		ScopeType: scheduledcontract.ScopeAccount,
		ScopeID:   &scopeID,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	run, err := runner.RunPlan(ctx, plan, scheduledcontract.TriggerManual)
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}
	if run.Selected != 1 || run.Probed != 1 || run.Failed != 0 || run.Unhealthy != 0 {
		t.Fatalf("unexpected run: %+v", run)
	}
	if run.Status != scheduledcontract.RunStatusOK || run.Trigger != scheduledcontract.TriggerManual {
		t.Fatalf("unexpected run status/trigger: %+v", run)
	}
	// run-now must persist run history.
	history, err := plans.ListRuns(ctx, plan.ID, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 run in history, got %d", len(history))
	}
}

func TestRunPlanAutoRecoversHealthyInactiveAccount(t *testing.T) {
	ctx := context.Background()
	runner, accounts, plans := setupRunner(t, okProberFactory())
	account := createAccount(t, accounts, accountcontract.StatusNeedsReauth)

	scopeID := account.ID
	plan, err := plans.CreatePlan(ctx, scheduledcontract.CreatePlan{
		Name:        "recover",
		Enabled:     true,
		ScopeType:   scheduledcontract.ScopeAccount,
		ScopeID:     &scopeID,
		AutoRecover: true,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	run, err := runner.RunPlan(ctx, plan, scheduledcontract.TriggerSchedule)
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}
	if run.Probed != 1 || run.Recovered != 1 {
		t.Fatalf("expected probed=1 recovered=1, got %+v", run)
	}
	reloaded, err := accounts.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if reloaded.Status != accountcontract.StatusActive {
		t.Fatalf("expected account recovered to active, got %s", reloaded.Status)
	}
}

func TestRunPlanSkipsInactiveWithoutAutoRecover(t *testing.T) {
	ctx := context.Background()
	runner, accounts, plans := setupRunner(t, okProberFactory())
	account := createAccount(t, accounts, accountcontract.StatusNeedsReauth)

	scopeID := account.ID
	plan, err := plans.CreatePlan(ctx, scheduledcontract.CreatePlan{
		Name:        "no-recover",
		Enabled:     true,
		ScopeType:   scheduledcontract.ScopeAccount,
		ScopeID:     &scopeID,
		AutoRecover: false,
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	run, err := runner.RunPlan(ctx, plan, scheduledcontract.TriggerSchedule)
	if err != nil {
		t.Fatalf("run plan: %v", err)
	}
	if run.Probed != 0 || run.Skipped != 1 || run.Recovered != 0 {
		t.Fatalf("expected probed=0 skipped=1 recovered=0, got %+v", run)
	}
	reloaded, err := accounts.FindByID(ctx, account.ID)
	if err != nil {
		t.Fatalf("find account: %v", err)
	}
	if reloaded.Status != accountcontract.StatusNeedsReauth {
		t.Fatalf("expected account to stay needs_reauth, got %s", reloaded.Status)
	}
}
