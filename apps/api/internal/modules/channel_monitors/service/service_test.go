package service

import (
	"context"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/channel_monitors/store/memory"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
)

func newService(t *testing.T) *Service {
	t.Helper()
	svc, err := New(memory.New())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func TestCreateDefinitionNormalizesAndValidates(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	if _, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "  ", Scope: contract.ScopeAccount}); err == nil {
		t.Fatal("expected blank name to be rejected")
	}
	if _, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "x", Scope: "bogus"}); err == nil {
		t.Fatal("expected invalid scope to be rejected")
	}
	if _, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "x", Scope: contract.ScopeAccount, Request: contract.CustomRequest{Body: "{not json"}}); err == nil {
		t.Fatal("expected invalid json body to be rejected")
	}

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{
		Name:     "monitor",
		Scope:    contract.ScopeAccount,
		Interval: 5, // below minimum, clamps to 30
		Request: contract.CustomRequest{
			Method:              "post",
			ExpectedStatusCodes: []int{200, 200, 99, 401},
			Headers:             map[string]string{" X ": " v ", "empty": ""},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if def.Interval != 30 {
		t.Fatalf("expected interval clamp to 30, got %d", def.Interval)
	}
	if def.Request.Method != "POST" {
		t.Fatalf("expected method uppercased, got %q", def.Request.Method)
	}
	if len(def.Request.ExpectedStatusCodes) != 2 {
		t.Fatalf("expected deduped/filtered status codes, got %v", def.Request.ExpectedStatusCodes)
	}
	if def.Request.Headers["X"] != "v" || len(def.Request.Headers) != 1 {
		t.Fatalf("expected trimmed headers, got %v", def.Request.Headers)
	}
}

func TestApplyTemplateUpdatesExistingSkipsMissing(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m1", Scope: contract.ScopeProvider, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create def: %v", err)
	}
	tpl, err := svc.CreateTemplate(ctx, contract.CreateTemplate{
		Name:    "tpl",
		Request: contract.CustomRequest{Method: "GET", ResponseContains: "ok"},
	})
	if err != nil {
		t.Fatalf("create tpl: %v", err)
	}

	applied, err := svc.ApplyTemplate(ctx, tpl.ID, []int{def.ID, 9999})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected one applied (missing skipped), got %d", len(applied))
	}
	if applied[0].Request.ResponseContains != "ok" || applied[0].Request.Method != "GET" {
		t.Fatalf("template request not applied: %+v", applied[0].Request)
	}
}

func TestRunHistoryRecordedAndOrdered(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m", Scope: contract.ScopeAccount, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := svc.RecordRun(ctx, contract.RecordRun{
			MonitorID:    def.ID,
			RunID:        "run",
			CheckedCount: 1,
			OKCount:      1,
			Results:      []contract.CheckResult{{AccountID: 1, Model: "gpt-4o-mini", OK: true}},
		}); err != nil {
			t.Fatalf("record run %d: %v", i, err)
		}
	}
	runs, err := svc.ListRuns(ctx, def.ID, 50)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	// Newest first.
	if runs[0].ID < runs[1].ID {
		t.Fatalf("expected runs newest-first, got %d before %d", runs[0].ID, runs[1].ID)
	}
}

func TestListDefinitionsWithSummaryAttachesLastRun(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	// Monitor with no runs — summary entry must exist but LastRun should be nil.
	noRuns, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "no-runs", Scope: contract.ScopeAccount, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create no-runs: %v", err)
	}
	// Monitor with two runs — only the newest run should populate the summary.
	withRuns, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "with-runs", Scope: contract.ScopeAccount, ScopeRef: "2"})
	if err != nil {
		t.Fatalf("create with-runs: %v", err)
	}
	if _, err := svc.RecordRun(ctx, contract.RecordRun{
		MonitorID:    withRuns.ID,
		RunID:        "older",
		OK:           false,
		CheckedCount: 1,
		OKCount:      0,
		LatencyMS:    900,
	}); err != nil {
		t.Fatalf("record older: %v", err)
	}
	if _, err := svc.RecordRun(ctx, contract.RecordRun{
		MonitorID:    withRuns.ID,
		RunID:        "newer",
		OK:           true,
		CheckedCount: 1,
		OKCount:      1,
		LatencyMS:    120,
	}); err != nil {
		t.Fatalf("record newer: %v", err)
	}

	entries, err := svc.ListDefinitionsWithSummary(ctx)
	if err != nil {
		t.Fatalf("list with summary: %v", err)
	}
	byID := map[int]contract.DefinitionWithSummary{}
	for _, e := range entries {
		byID[e.ID] = e
	}
	if entry, ok := byID[noRuns.ID]; !ok {
		t.Fatalf("no-runs monitor missing from result")
	} else if entry.LastRun != nil {
		t.Fatalf("no-runs monitor should have nil LastRun, got %+v", entry.LastRun)
	}
	entry, ok := byID[withRuns.ID]
	if !ok {
		t.Fatalf("with-runs monitor missing from result")
	}
	if entry.LastRun == nil {
		t.Fatalf("with-runs monitor should have a LastRun summary")
	}
	if !entry.LastRun.OK {
		t.Fatalf("LastRun.OK: want true (newest run), got false")
	}
	if entry.LastRun.LatencyMS != 120 {
		t.Fatalf("LastRun.LatencyMS: want 120 (newest run), got %d", entry.LastRun.LatencyMS)
	}
	if entry.LastRun.At.IsZero() {
		t.Fatalf("LastRun.At must be set")
	}
}

func TestListDefinitionsWithSummaryComputesRollingUptime(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m", Scope: contract.ScopeAccount, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Record 5 runs: 3 OK + 2 failures. Expected SampleCount=5, Successes=3.
	for i, ok := range []bool{true, false, true, false, true} {
		if _, err := svc.RecordRun(ctx, contract.RecordRun{
			MonitorID: def.ID,
			RunID:     "r",
			OK:        ok,
		}); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}

	entries, err := svc.ListDefinitionsWithSummary(ctx)
	if err != nil {
		t.Fatalf("list with summary: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries len: want 1, got %d", len(entries))
	}
	entry := entries[0]
	if entry.Recent == nil {
		t.Fatalf("Recent should be populated for a monitor with runs")
	}
	if entry.Recent.SampleCount != 5 {
		t.Fatalf("SampleCount: want 5, got %d", entry.Recent.SampleCount)
	}
	if entry.Recent.Successes != 3 {
		t.Fatalf("Successes: want 3, got %d", entry.Recent.Successes)
	}
	if entry.Recent.WindowDays != 7 {
		t.Fatalf("WindowDays: want 7, got %d", entry.Recent.WindowDays)
	}
	if got := entry.Recent.SuccessRate(); got != 0.6 {
		t.Fatalf("SuccessRate: want 0.6, got %v", got)
	}
}

func TestDeleteDefinitionRemovesRuns(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m", Scope: contract.ScopeAccount, ScopeRef: "1"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.RecordRun(ctx, contract.RecordRun{MonitorID: def.ID, RunID: "r", CheckedCount: 1}); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := svc.DeleteDefinition(ctx, def.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetDefinition(ctx, def.ID); err != contract.ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestRunDefinitionRejectsDisabledMonitor(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "m", Scope: contract.ScopeAccount, ScopeRef: "1", Enabled: false})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.RunDefinition(ctx, def.ID, monitorDeps(), contract.TriggerManual); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestRunDefinitionPassesModelAndCustomRequestToProbe(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{
		Name:     "model probe",
		Enabled:  true,
		Scope:    contract.ScopeAccount,
		ScopeRef: "1",
		Model:    "gpt-monitor",
		Request: contract.CustomRequest{
			Method:              "POST",
			URL:                 "https://probe.example/v1/chat/completions",
			Headers:             map[string]string{"X-Probe": "monitor"},
			Body:                `{"input":"ping"}`,
			ExpectedStatusCodes: []int{202},
			ResponseJSONPath:    "data.0.id",
			ResponseContains:    "ok",
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	adapter := &monitorProbeAdapter{}
	deps := monitorDeps()
	deps.Adapter = adapter

	run, err := svc.RunDefinition(ctx, def.ID, deps, contract.TriggerManual)
	if err != nil {
		t.Fatalf("run definition: %v", err)
	}
	if !run.OK || run.CheckedCount != 1 {
		t.Fatalf("unexpected run: %+v", run)
	}
	if adapter.last.Model != "gpt-monitor" {
		t.Fatalf("expected model propagated to probe request, got %q", adapter.last.Model)
	}
	metadata := adapter.last.Account.Metadata
	if metadata["existing"] != "value" ||
		metadata["health_probe_method"] != "POST" ||
		metadata["health_probe_url"] != "https://probe.example/v1/chat/completions" ||
		metadata["health_probe_body"] != `{"input":"ping"}` ||
		metadata["health_probe_response_path"] != "data.0.id" ||
		metadata["health_probe_response_contains"] != "ok" {
		t.Fatalf("unexpected probe metadata: %+v", metadata)
	}
	headers, ok := metadata["health_probe_headers"].(map[string]string)
	if !ok || headers["X-Probe"] != "monitor" {
		t.Fatalf("unexpected probe headers metadata: %+v", metadata["health_probe_headers"])
	}
	codes, ok := metadata["health_probe_expected_status_codes"].([]int)
	if !ok || len(codes) != 1 || codes[0] != 202 {
		t.Fatalf("unexpected expected status metadata: %+v", metadata["health_probe_expected_status_codes"])
	}
}

func TestRunDueSkipsDisabledAndNotDueAndRecordsScheduled(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	due, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "due", Scope: contract.ScopeAccount, ScopeRef: "1", Enabled: true, Interval: 30})
	if err != nil {
		t.Fatalf("create due: %v", err)
	}
	disabled, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "disabled", Scope: contract.ScopeAccount, ScopeRef: "1", Enabled: false, Interval: 30})
	if err != nil {
		t.Fatalf("create disabled: %v", err)
	}
	notDue, err := svc.CreateDefinition(ctx, contract.CreateDefinition{Name: "not-due", Scope: contract.ScopeAccount, ScopeRef: "1", Enabled: true, Interval: 3600})
	if err != nil {
		t.Fatalf("create not due: %v", err)
	}
	if _, err := svc.RecordRun(ctx, contract.RecordRun{
		MonitorID: notDue.ID,
		RunID:     "recent",
		Trigger:   contract.TriggerScheduled,
	}); err != nil {
		t.Fatalf("record recent run: %v", err)
	}

	runs, err := svc.RunDue(ctx, monitorDeps(), time.Now().UTC(), 0)
	if err != nil {
		t.Fatalf("run due: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one scheduled run, got %d", len(runs))
	}
	if runs[0].MonitorID != due.ID || runs[0].Trigger != contract.TriggerScheduled || runs[0].CheckedCount != 1 || runs[0].OKCount != 1 {
		t.Fatalf("unexpected scheduled run: %+v", runs[0])
	}
	if disabledRuns, err := svc.ListRuns(ctx, disabled.ID, 10); err != nil || len(disabledRuns) != 0 {
		t.Fatalf("expected disabled monitor to be skipped, runs=%+v err=%v", disabledRuns, err)
	}
	if notDueRuns, err := svc.ListRuns(ctx, notDue.ID, 10); err != nil || len(notDueRuns) != 1 {
		t.Fatalf("expected not-due monitor to keep only seed run, runs=%+v err=%v", notDueRuns, err)
	}
}

func monitorDeps() contract.RunnerDependencies {
	return contract.RunnerDependencies{
		Accounts: &monitorAccountReader{accounts: []accountcontract.ProviderAccount{{
			ID:           1,
			ProviderID:   10,
			Name:         "account",
			RuntimeClass: accountcontract.RuntimeClassAPIKey,
			Status:       accountcontract.StatusActive,
			Metadata:     map[string]any{"existing": "value"},
		}}},
		Providers: &monitorProviderReader{provider: providercontract.Provider{ID: 10, Name: "provider", Status: providercontract.StatusActive}},
		Models:    &monitorModelReader{},
		Adapter:   &monitorProbeAdapter{},
	}
}

type monitorAccountReader struct {
	accounts []accountcontract.ProviderAccount
}

func (r *monitorAccountReader) List(context.Context) ([]accountcontract.ProviderAccount, error) {
	return r.accounts, nil
}

func (r *monitorAccountReader) ListGroupMembers(context.Context, int) ([]accountcontract.AccountGroupMember, error) {
	return nil, nil
}

func (r *monitorAccountReader) DecryptCredential(context.Context, int) (map[string]any, error) {
	return map[string]any{"api_key": "secret"}, nil
}

type monitorProviderReader struct {
	provider providercontract.Provider
}

func (r *monitorProviderReader) FindByID(context.Context, int) (providercontract.Provider, error) {
	return r.provider, nil
}

type monitorModelReader struct{}

func (r *monitorModelReader) List(context.Context) ([]modelcontract.Model, error) {
	return nil, nil
}

func (r *monitorModelReader) ListMappingsByModel(context.Context, int) ([]modelcontract.ModelProviderMapping, error) {
	return nil, nil
}

type monitorProbeAdapter struct {
	last provideradaptercontract.ProbeRequest
}

func (a *monitorProbeAdapter) ProbeAccount(_ context.Context, req provideradaptercontract.ProbeRequest) (provideradaptercontract.ProbeResponse, error) {
	a.last = req
	if req.Account.Metadata["existing"] != "value" {
		return provideradaptercontract.ProbeResponse{OK: false, StatusCode: 500, ErrorClass: "missing_metadata"}, nil
	}
	return provideradaptercontract.ProbeResponse{OK: true, StatusCode: 200, LatencyMS: 7}, nil
}

// TestUpdateDefinitionEnabledOnlyToggle locks in the contract the iter-38
// frontend inline toggle relies on: PATCH with only Enabled set must not
// require any of the other fields and must flip just that flag.
func TestUpdateDefinitionEnabledOnlyToggle(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	def, err := svc.CreateDefinition(ctx, contract.CreateDefinition{
		Name:    "mon",
		Scope:   contract.ScopeAccount,
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	disabled := false
	updated, err := svc.UpdateDefinition(ctx, def.ID, contract.UpdateDefinition{Enabled: &disabled})
	if err != nil {
		t.Fatalf("update enabled-only: %v", err)
	}
	if updated.Enabled {
		t.Fatal("expected enabled flipped to false")
	}
	if updated.Name != "mon" || updated.Scope != contract.ScopeAccount {
		t.Fatalf("partial update leaked into other fields: %+v", updated)
	}
	enabled := true
	updated, err = svc.UpdateDefinition(ctx, def.ID, contract.UpdateDefinition{Enabled: &enabled})
	if err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if !updated.Enabled {
		t.Fatal("expected enabled flipped back to true")
	}
}
