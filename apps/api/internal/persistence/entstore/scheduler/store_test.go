package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/srapi/srapi/apps/api/ent/enttest"
	"github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"

	_ "github.com/mattn/go-sqlite3"
)

func TestStoreListsActiveGlobalStrategies(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 25, 12, 0, 0, 0, time.UTC)
	active, err := client.SchedulerStrategy.Create().
		SetName(string(contract.StrategyBalanced)).
		SetVersion("v2").
		SetStatus("active").
		SetScopeType("global").
		SetConfigJSON(map[string]any{
			"weights": map[string]any{"cost_weight": 1.0},
		}).
		SetConfigHash("sha256:stale").
		SetDescription("Persistent balanced override").
		SetActivatedAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create active strategy: %v", err)
	}
	newer, err := client.SchedulerStrategy.Create().
		SetName(string(contract.StrategyBalanced)).
		SetVersion("v10").
		SetStatus("active").
		SetScopeType("global").
		SetConfigJSON(map[string]any{
			"weights": map[string]any{"health_weight": 1.0},
		}).
		SetConfigHash("sha256:newer").
		SetDescription("Newer persistent balanced override").
		SetActivatedAt(now.Add(time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create newer active strategy: %v", err)
	}
	if _, err := client.SchedulerStrategy.Create().
		SetName(string(contract.StrategyCostSaver)).
		SetVersion("v2").
		SetStatus("draft").
		SetScopeType("global").
		SetConfigJSON(map[string]any{"weights": map[string]any{"cost": 1.0}}).
		SetConfigHash("sha256:draft").
		Save(ctx); err != nil {
		t.Fatalf("create draft strategy: %v", err)
	}
	if _, err := client.SchedulerStrategy.Create().
		SetName(string(contract.StrategyLatencyFirst)).
		SetVersion("v2").
		SetStatus("active").
		SetScopeType("api_key").
		SetScopeID(10).
		SetConfigJSON(map[string]any{"weights": map[string]any{"latency": 1.0}}).
		SetConfigHash("sha256:scoped").
		Save(ctx); err != nil {
		t.Fatalf("create scoped strategy: %v", err)
	}

	strategies, err := store.ListActiveStrategies(ctx)
	if err != nil {
		t.Fatalf("list active strategies: %v", err)
	}
	if len(strategies) != 1 {
		t.Fatalf("expected only active global strategy, got %+v", strategies)
	}
	if strategies[0].ID != newer.ID || strategies[0].Name != contract.StrategyBalanced || strategies[0].Version != "v10" {
		t.Fatalf("unexpected active strategy: %+v", strategies[0])
	}
	weights, ok := strategies[0].Config["weights"].(map[string]any)
	if !ok || weights["health_weight"].(float64) != 1.0 {
		t.Fatalf("expected persisted weights, got %+v", strategies[0].Config)
	}
	if active.ID == newer.ID {
		t.Fatal("expected distinct active strategy rows")
	}
}

func TestStoreCreatesDecisionWithRequestSnapshot(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 25, 12, 30, 0, 0, time.UTC)
	selectedAccountID := 10
	selectedProviderID := 20
	decision, snapshot, err := store.CreateDecisionWithSnapshot(ctx, contract.Decision{
		RequestID:          "req_snapshot",
		AttemptNo:          1,
		UserID:             1,
		APIKeyID:           2,
		SourceProtocol:     "openai-compatible",
		SourceEndpoint:     "/v1/chat/completions",
		TargetProtocol:     "openai-compatible",
		Model:              "gpt-test",
		Strategy:           contract.StrategyBalanced,
		StrategyVersion:    "v1",
		StrategyConfigHash: "sha256:test",
		SelectedProviderID: &selectedProviderID,
		SelectedAccountID:  &selectedAccountID,
		CandidateCount:     1,
		Scores:             map[string]any{"account_10": map[string]any{"final_score": 0.9}},
		StrategyWeights:    map[string]any{"health": 0.3},
		CreatedAt:          now,
	}, contract.RequestSnapshot{
		RequestProfile: map[string]any{
			"model":                     "gpt-test",
			"session_affinity_key_hash": "sha256:test",
		},
		CandidateSnapshot: []contract.CandidateSnapshot{
			{
				AccountID:        selectedAccountID,
				ProviderID:       selectedProviderID,
				AccountMetadata:  map[string]any{"quality_score": 0.9},
				ProviderProtocol: "openai-compatible",
				ProviderConfig:   map[string]any{"base_url": "https://provider.example"},
			},
		},
		RankedAccountIDs: []int{selectedAccountID},
		CreatedAt:        now,
	})
	if err != nil {
		t.Fatalf("create decision with snapshot: %v", err)
	}
	if snapshot.DecisionID != decision.ID || snapshot.RequestID != decision.RequestID || snapshot.AttemptNo != decision.AttemptNo {
		t.Fatalf("expected linked snapshot, decision=%+v snapshot=%+v", decision, snapshot)
	}
	if snapshot.SelectedAccountID == nil || *snapshot.SelectedAccountID != selectedAccountID {
		t.Fatalf("expected selected account copied from decision, got %+v", snapshot)
	}

	snapshots, err := store.ListRequestSnapshots(ctx)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected one snapshot, got %+v", snapshots)
	}
	loaded := snapshots[0]
	if loaded.DecisionID != decision.ID || loaded.CandidateSnapshot[0].AccountID != selectedAccountID || loaded.RankedAccountIDs[0] != selectedAccountID {
		t.Fatalf("unexpected loaded snapshot: %+v", loaded)
	}
	if loaded.CandidateSnapshot[0].ProviderConfig["base_url"] != "https://provider.example" {
		t.Fatalf("expected provider config round trip, got %+v", loaded.CandidateSnapshot[0].ProviderConfig)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "scheduler.db") + "?_fk=1"
}
