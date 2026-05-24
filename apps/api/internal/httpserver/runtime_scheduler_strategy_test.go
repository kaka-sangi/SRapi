package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/config"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
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
