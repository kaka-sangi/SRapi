package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/config"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
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
