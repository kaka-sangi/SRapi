package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/config"
	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	capabilitiescontract "github.com/srapi/srapi/apps/api/internal/modules/capabilities/contract"
	modelcontract "github.com/srapi/srapi/apps/api/internal/modules/models/contract"
	operationscontract "github.com/srapi/srapi/apps/api/internal/modules/operations/contract"
	operationsmemory "github.com/srapi/srapi/apps/api/internal/modules/operations/store/memory"
	providercontract "github.com/srapi/srapi/apps/api/internal/modules/providers/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	usagecontract "github.com/srapi/srapi/apps/api/internal/modules/usage/contract"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	apiopenapi "github.com/srapi/srapi/apps/api/internal/openapi"
	"github.com/srapi/srapi/apps/api/internal/persistence/entstore"

	_ "github.com/mattn/go-sqlite3"
)

func TestAdminSchedulerStrategiesReflectActivePersistentStrategy(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, persistenceSQLiteDSN(t))
	defer client.Close()

	stores, err := entstore.New(client)
	if err != nil {
		t.Fatalf("new ent stores: %v", err)
	}
	active, err := client.SchedulerStrategy.Create().
		SetName(string(schedulercontract.StrategyBalanced)).
		SetVersion("v2").
		SetStatus("active").
		SetScopeType("global").
		SetConfigJSON(map[string]any{
			"weights": map[string]any{"cost_weight": 1.0},
		}).
		SetConfigHash("sha256:stale").
		SetDescription("Persistent balanced override").
		Save(t.Context())
	if err != nil {
		t.Fatalf("seed scheduler strategy: %v", err)
	}

	handler := New(config.Load(), nil, WithSchedulerStore(stores.Scheduler))
	_, sessionCookie := mustLoginAdmin(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/scheduler/strategies", nil)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected scheduler strategies 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp apiopenapi.SchedulerStrategyListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode scheduler strategies: %v", err)
	}
	for _, strategy := range resp.Data {
		if strategy.Name != apiopenapi.SchedulerStrategyNameBalanced {
			continue
		}
		if strategy.Id != apiopenapi.Id(strconv.Itoa(active.ID)) || strategy.Version != "v2" {
			t.Fatalf("expected persistent balanced strategy row, got %+v", strategy)
		}
		weights, ok := strategy.Config["weights"].(map[string]any)
		if !ok || weights["cost"].(float64) != 1.0 {
			t.Fatalf("expected normalized persistent weights, got %+v", strategy.Config)
		}
		return
	}
	t.Fatalf("expected balanced strategy in %+v", resp.Data)
}

func TestAdminSchedulerSimulationIsDryRun(t *testing.T) {
	schedulerStore := schedulermemory.New()
	handler := New(config.Load(), nil, WithSchedulerStore(schedulerStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	body := `{
		"current_strategy":"balanced",
		"shadow_strategy":"cost_saver",
		"shadow_rollout_percent":100,
		"rollout_key":"raw-rollout-key",
		"request":{
			"request_id":"sim-http-1",
			"user_id":"1",
			"api_key_id":"1",
			"source_endpoint":"/v1/chat/completions",
			"model":"sim-model",
			"candidates":[
				{
					"account_id":"1",
					"provider_id":"1",
					"runtime_state":{"health_score":0.95},
					"pricing_override":{"relative_cost":0.9}
				},
				{
					"account_id":"2",
					"provider_id":"2",
					"runtime_state":{"health_score":0.60},
					"pricing_override":{"relative_cost":0.1}
				}
			]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scheduler/simulate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected scheduler simulation 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp apiopenapi.SchedulerSimulationResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode scheduler simulation response: %v", err)
	}
	if !resp.Data.DryRun {
		t.Fatalf("expected dry-run response, got %+v", resp.Data)
	}
	if resp.Data.Current.SelectedAccountId == nil || *resp.Data.Current.SelectedAccountId != "1" {
		t.Fatalf("expected balanced simulation to select account 1, got %+v", resp.Data.Current)
	}
	if resp.Data.Shadow.SelectedAccountId == nil || *resp.Data.Shadow.SelectedAccountId != "2" {
		t.Fatalf("expected cost_saver simulation to select account 2, got %+v", resp.Data.Shadow)
	}
	if !resp.Data.Diff.WinnerChanged || resp.Data.Diff.CostScoreDelta <= 0 {
		t.Fatalf("expected winner and cost score delta, got %+v", resp.Data.Diff)
	}
	if !resp.Data.Rollout.Enabled || !resp.Data.Rollout.ShadowSelected || resp.Data.Rollout.Percent != 100 {
		t.Fatalf("expected enabled 100 percent rollout, got %+v", resp.Data.Rollout)
	}
	if resp.Data.Rollout.KeyHash == "" || resp.Data.Rollout.KeyHash == "raw-rollout-key" || !strings.HasPrefix(resp.Data.Rollout.KeyHash, "sha256:") {
		t.Fatalf("expected hashed rollout key only, got %+v", resp.Data.Rollout)
	}

	decisions, err := schedulerStore.ListDecisions(t.Context())
	if err != nil {
		t.Fatalf("list scheduler decisions: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected no persisted scheduler decisions from simulation, got %+v", decisions)
	}
	leases, err := schedulerStore.ListLeases(t.Context())
	if err != nil {
		t.Fatalf("list scheduler leases: %v", err)
	}
	if len(leases) != 0 {
		t.Fatalf("expected no scheduler leases from simulation, got %+v", leases)
	}
}

func TestAdminSchedulerReplayUsesPersistedSnapshots(t *testing.T) {
	schedulerStore := schedulermemory.New()
	schedulerSvc, err := schedulerservice.New(schedulerStore, nil)
	if err != nil {
		t.Fatalf("create scheduler service: %v", err)
	}
	request := schedulercontract.ScheduleRequest{
		RequestID:      "replay-http-1",
		UserID:         1,
		APIKeyID:       1,
		SourceProtocol: "openai-compatible",
		SourceEndpoint: "/v1/chat/completions",
		Model:          "replay-model",
		Strategy:       schedulercontract.StrategyBalanced,
		Candidates: []schedulercontract.Candidate{
			schedulerReplayCandidate(1, 0.95, "0.9"),
			schedulerReplayCandidate(2, 0.60, "0.1"),
		},
	}
	scheduled, err := schedulerSvc.Schedule(t.Context(), request)
	if err != nil {
		t.Fatalf("schedule replay seed: %v", err)
	}
	if scheduled.Candidate.Account.ID != 1 {
		t.Fatalf("expected balanced seed to select account 1, got %+v", scheduled.Candidate)
	}

	handler := New(config.Load(), nil, WithSchedulerStore(schedulerStore))
	loginResp, sessionCookie := mustLoginAdmin(t, handler)

	body := `{"shadow_strategy":"cost_saver","shadow_rollout_percent":100,"limit":10,"model":"replay-model"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/scheduler/replay", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", loginResp.Data.CsrfToken)
	req.AddCookie(sessionCookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected scheduler replay 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp apiopenapi.SchedulerReplayResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode scheduler replay response: %v", err)
	}
	if !resp.Data.DryRun || resp.Data.Requested != 1 || resp.Data.Replayed != 1 || resp.Data.WinnerChanged != 1 {
		t.Fatalf("unexpected replay summary: %+v", resp.Data)
	}
	if len(resp.Data.Items) != 1 {
		t.Fatalf("expected one replay item, got %+v", resp.Data.Items)
	}
	item := resp.Data.Items[0]
	if item.DecisionId != apiopenapi.Id(strconv.Itoa(scheduled.Decision.ID)) || item.Current.SelectedAccountId == nil || *item.Current.SelectedAccountId != "1" {
		t.Fatalf("expected current replay to reference seed decision/account 1, got %+v", item)
	}
	if item.Shadow.SelectedAccountId == nil || *item.Shadow.SelectedAccountId != "2" || !item.Diff.WinnerChanged {
		t.Fatalf("expected shadow replay account 2 with winner change, got %+v", item)
	}
	if !item.Rollout.Enabled || !item.Rollout.ShadowSelected || item.Rollout.KeyHash == "" || item.Rollout.KeyHash == request.RequestID {
		t.Fatalf("expected hashed rollout preview, got %+v", item.Rollout)
	}

	decisions, err := schedulerStore.ListDecisions(t.Context())
	if err != nil {
		t.Fatalf("list scheduler decisions: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected replay not to create decisions, got %+v", decisions)
	}
	leases, err := schedulerStore.ListLeases(t.Context())
	if err != nil {
		t.Fatalf("list scheduler leases: %v", err)
	}
	if len(leases) != 1 {
		t.Fatalf("expected replay not to acquire leases, got %+v", leases)
	}
}

func TestMetricsExposeSchedulerStrategyOperationalSignals(t *testing.T) {
	schedulerStore := schedulermemory.New()
	usageStore := usagememory.New()

	firstSelected := 2
	firstDecision, err := schedulerStore.CreateDecision(t.Context(), schedulercontract.Decision{
		RequestID:          "strategy-metrics",
		AttemptNo:          1,
		UserID:             1,
		APIKeyID:           1,
		Model:              "strategy-metrics-model",
		Strategy:           schedulercontract.StrategyCostSaver,
		StrategyVersion:    "v1",
		SelectedAccountID:  &firstSelected,
		SelectedProviderID: intPtrForSchedulerMetrics(2),
		CandidateCount:     2,
		Scores: map[string]any{
			"account_1": map[string]any{"account_id": 1, "cost_score": 0.2, "latency_score": 0.9},
			"account_2": map[string]any{"account_id": 2, "cost_score": 0.8, "latency_score": 0.5},
			"routing_hints": map[string]any{
				"strategy_rollout": map[string]any{
					"shadow_strategy": "cost_saver",
					"shadow_selected": true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create first decision: %v", err)
	}

	secondSelected := 1
	_, err = schedulerStore.CreateDecision(t.Context(), schedulercontract.Decision{
		RequestID:              firstDecision.RequestID,
		AttemptNo:              2,
		UserID:                 1,
		APIKeyID:               1,
		Model:                  firstDecision.Model,
		Strategy:               schedulercontract.StrategyCostSaver,
		StrategyVersion:        "v1",
		FallbackFromDecisionID: &firstDecision.ID,
		SelectedAccountID:      &secondSelected,
		SelectedProviderID:     intPtrForSchedulerMetrics(1),
		CandidateCount:         2,
		RejectedCount:          1,
		RejectReasons:          map[string]any{"account_2": "fallback_excluded"},
		CompatibilityWarnings:  []string{"strategy_rollout_current_selected"},
		SelectionRationale:     "fallback selected another account",
		EstimatedCost:          "0.00000000",
		Currency:               "USD",
		Scores: map[string]any{
			"account_1": map[string]any{"account_id": 1, "cost_score": 0.6, "latency_score": 0.6},
			"account_2": map[string]any{"account_id": 2, "cost_score": 0.4, "latency_score": 0.4},
		},
	})
	if err != nil {
		t.Fatalf("create fallback decision: %v", err)
	}
	errorClass := "upstream_error"
	for _, log := range []usagecontract.UsageLog{
		{RequestID: firstDecision.RequestID, AttemptNo: 1, Success: false, ErrorClass: &errorClass, SourceEndpoint: "/v1/chat/completions", TargetProtocol: "openai-compatible", Model: firstDecision.Model},
		{RequestID: firstDecision.RequestID, AttemptNo: 2, Success: true, SourceEndpoint: "/v1/chat/completions", TargetProtocol: "openai-compatible", Model: firstDecision.Model},
	} {
		if _, err := usageStore.Create(t.Context(), log); err != nil {
			t.Fatalf("create usage log: %v", err)
		}
	}

	handler := New(config.Load(), nil, WithSchedulerStore(schedulerStore), WithUsageStore(usageStore))
	metrics := metricsBody(t, handler)
	for _, expected := range []string{
		`scheduler_strategy_selected_total{strategy="cost_saver",version="v1"} 2`,
		`scheduler_strategy_fallback_total{strategy="cost_saver",version="v1"} 1`,
		`scheduler_strategy_shadow_diff{selection="shadow",shadow_strategy="cost_saver",strategy="cost_saver",version="v1"} 1`,
		`scheduler_strategy_cost_delta{strategy="cost_saver",version="v1"}`,
		`scheduler_strategy_latency_delta{strategy="cost_saver",version="v1"}`,
		`scheduler_strategy_error_rate{strategy="cost_saver",version="v1"} 0.5`,
		`scheduler_strategy_reject_reason_total{reason="fallback_excluded",strategy="cost_saver",version="v1"} 1`,
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("expected metrics to contain %s, got:\n%s", expected, metrics)
		}
	}
}

func TestMetricsExposeOpsAlertEventCounts(t *testing.T) {
	operationsStore := operationsmemory.New()
	now := time.Now().UTC()
	for _, alert := range []operationscontract.AlertEvent{
		{
			RuleID:      "slo.burn_rate.critical",
			Severity:    operationscontract.AlertSeverityCritical,
			Status:      operationscontract.AlertStatusFiring,
			Fingerprint: "slo:secret-fingerprint-1",
			Summary:     "critical gateway burn rate",
			StartedAt:   now,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			RuleID:      "slo.burn_rate.critical",
			Severity:    operationscontract.AlertSeverityCritical,
			Status:      operationscontract.AlertStatusFiring,
			Fingerprint: "slo:secret-fingerprint-2",
			Summary:     "critical gateway burn rate",
			StartedAt:   now,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			RuleID:      "slo.burn_rate.warning",
			Severity:    operationscontract.AlertSeverityWarning,
			Status:      operationscontract.AlertStatusAcknowledged,
			Fingerprint: "slo:secret-fingerprint-3",
			Summary:     "warning gateway burn rate",
			StartedAt:   now,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	} {
		if _, err := operationsStore.CreateAlert(t.Context(), alert); err != nil {
			t.Fatalf("create alert: %v", err)
		}
	}

	handler := New(config.Load(), nil, WithOperationsStore(operationsStore))
	metrics := metricsBody(t, handler)
	for _, expected := range []string{
		`srapi_ops_alert_events{severity="critical",status="firing"} 2`,
		`srapi_ops_alert_events{severity="warning",status="acknowledged"} 1`,
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("expected metrics to contain %s, got:\n%s", expected, metrics)
		}
	}
	if strings.Contains(metrics, "secret-fingerprint") || strings.Contains(metrics, "slo.burn_rate.critical") {
		t.Fatalf("ops alert metrics must not expose alert fingerprints or rule ids, got:\n%s", metrics)
	}
}

func schedulerReplayCandidate(id int, health float64, relativeCost string) schedulercontract.Candidate {
	return schedulercontract.Candidate{
		Account: accountcontract.ProviderAccount{
			ID:                   id,
			ProviderID:           id,
			RuntimeClass:         accountcontract.RuntimeClassAPIKey,
			CredentialCiphertext: "encrypted",
			Status:               accountcontract.StatusActive,
			Weight:               1,
		},
		Provider: providercontract.Provider{
			ID:       id,
			Protocol: "openai-compatible",
			Status:   providercontract.StatusActive,
		},
		Mapping: modelcontract.ModelProviderMapping{
			ID:                id,
			ModelID:           1,
			ProviderID:        id,
			UpstreamModelName: "replay-model",
			Status:            modelcontract.StatusActive,
			PricingOverride:   map[string]any{"relative_cost": relativeCost},
		},
		EffectiveCapabilities: []capabilitiescontract.Descriptor{
			{Key: capabilitiescontract.KeyStreaming, Level: capabilitiescontract.DescriptorLevelRequired, Status: capabilitiescontract.DescriptorStatusStable, Version: "v1"},
		},
		RuntimeState: schedulercontract.RuntimeState{HealthScore: &health},
	}
}

func intPtrForSchedulerMetrics(value int) *int {
	return &value
}
