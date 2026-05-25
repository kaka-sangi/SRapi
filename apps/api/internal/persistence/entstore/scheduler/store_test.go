package scheduler

import (
	"context"
	"math"
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
		SelectionRationale: "Selected account 10 on provider 20 with final score 0.900.",
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
	if decision.SelectionRationale != "Selected account 10 on provider 20 with final score 0.900." {
		t.Fatalf("expected decision rationale round trip on create, got %+v", decision)
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
	decisions, err := store.ListDecisions(ctx)
	if err != nil {
		t.Fatalf("list decisions: %v", err)
	}
	if len(decisions) != 1 || decisions[0].SelectionRationale != decision.SelectionRationale {
		t.Fatalf("expected listed decision rationale, got %+v", decisions)
	}
}

func TestStoreAggregatesFeedbackSignals(t *testing.T) {
	client := enttest.Open(t, dialect.SQLite, sqliteDSN(t))
	defer client.Close()

	store, err := New(client)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 5, 25, 13, 0, 0, 0, time.UTC)
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:    "req_signal_1",
		DecisionID:   1,
		AttemptNo:    1,
		AccountID:    10,
		ProviderID:   1,
		Model:        "gpt-test",
		Success:      true,
		InputTokens:  900,
		OutputTokens: 100,
		ActualCost:   "0.03000000",
		Currency:     "USD",
		CreatedAt:    now.Add(-time.Hour),
	})
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:    "req_signal_2",
		DecisionID:   2,
		AttemptNo:    1,
		AccountID:    10,
		ProviderID:   1,
		Model:        "gpt-test",
		Success:      true,
		InputTokens:  100,
		OutputTokens: 100,
		CachedTokens: 800,
		ActualCost:   "0.01000000",
		Currency:     "USD",
		CreatedAt:    now.Add(-30 * time.Minute),
	})
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:    "req_signal_other_account",
		DecisionID:   3,
		AttemptNo:    1,
		AccountID:    20,
		ProviderID:   1,
		Model:        "gpt-test",
		Success:      true,
		InputTokens:  400,
		OutputTokens: 100,
		ActualCost:   "0.05000000",
		Currency:     "USD",
		CreatedAt:    now.Add(-time.Minute),
	})
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:    "req_signal_failed",
		DecisionID:   4,
		AttemptNo:    1,
		AccountID:    10,
		ProviderID:   1,
		Model:        "gpt-test",
		Success:      false,
		InputTokens:  10000,
		OutputTokens: 10000,
		ActualCost:   "99.00000000",
		Currency:     "USD",
		CreatedAt:    now.Add(-time.Minute),
	})
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:    "req_signal_other_model",
		DecisionID:   5,
		AttemptNo:    1,
		AccountID:    10,
		ProviderID:   1,
		Model:        "other-model",
		Success:      true,
		InputTokens:  10000,
		OutputTokens: 10000,
		ActualCost:   "99.00000000",
		Currency:     "USD",
		CreatedAt:    now.Add(-time.Minute),
	})
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:   "req_signal_old",
		DecisionID:  6,
		AttemptNo:   1,
		AccountID:   10,
		ProviderID:  1,
		Model:       "gpt-test",
		Success:     true,
		InputTokens: 10000,
		ActualCost:  "99.00000000",
		Currency:    "USD",
		CreatedAt:   now.Add(-48 * time.Hour),
	})
	createFeedbackRow(t, store, contract.Feedback{
		RequestID:  "req_signal_zero_tokens",
		DecisionID: 7,
		AttemptNo:  1,
		AccountID:  10,
		ProviderID: 1,
		Model:      "gpt-test",
		Success:    true,
		ActualCost: "99.00000000",
		Currency:   "USD",
		CreatedAt:  now.Add(-time.Minute),
	})

	signals, err := store.ListFeedbackSignals(ctx, contract.FeedbackSignalQuery{
		AccountIDs: []int{10, 20},
		Model:      "gpt-test",
		Since:      now.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("list feedback signals: %v", err)
	}
	if len(signals) != 2 {
		t.Fatalf("expected two account signals, got %+v", signals)
	}
	account10 := signals[0]
	account20 := signals[1]
	if account10.AccountID != 10 || account20.AccountID != 20 {
		t.Fatalf("expected signals sorted by account id, got %+v", signals)
	}
	if account10.SampleCount != 2 || account10.InputTokens != 1000 || account10.OutputTokens != 200 || account10.CachedTokens != 800 {
		t.Fatalf("unexpected account 10 aggregate: %+v", account10)
	}
	assertClose(t, account10.CostPer1KTokens, 0.02)
	assertClose(t, account10.CacheHitRate, 800.0/1800.0)
	if !account10.HasCost || !account10.HasCache {
		t.Fatalf("expected account 10 cost and cache signals, got %+v", account10)
	}
	if account20.SampleCount != 1 || account20.InputTokens != 400 || account20.OutputTokens != 100 || account20.CachedTokens != 0 {
		t.Fatalf("unexpected account 20 aggregate: %+v", account20)
	}
	assertClose(t, account20.CostPer1KTokens, 0.1)
	assertClose(t, account20.CacheHitRate, 0)
	if !account20.HasCost || !account20.HasCache {
		t.Fatalf("expected account 20 cost and cache signals, got %+v", account20)
	}
}

func sqliteDSN(t *testing.T) string {
	t.Helper()
	return "file:" + filepath.Join(t.TempDir(), "scheduler.db") + "?_fk=1"
}

func createFeedbackRow(t *testing.T, store *Store, feedback contract.Feedback) {
	t.Helper()
	if _, err := store.CreateFeedback(context.Background(), feedback); err != nil {
		t.Fatalf("create feedback row: %v", err)
	}
}

func assertClose(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.000001 {
		t.Fatalf("expected %v, got %v", want, got)
	}
}
