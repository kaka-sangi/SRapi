package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	apikeyservice "github.com/srapi/srapi/apps/api/internal/modules/api_keys/service"
	apikeymemory "github.com/srapi/srapi/apps/api/internal/modules/api_keys/store/memory"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	subscriptioncontract "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	subscriptionservice "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

func TestWarnDefaultZeroGatewayPricing(t *testing.T) {
	var logs bytes.Buffer
	rt := &runtimeState{
		logger: slog.New(slog.NewTextHandler(&logs, nil)),
	}

	rt.warnDefaultZeroGatewayPricing(gatewayUsageRecord{
		RequestID:      "req_default_zero",
		SourceEndpoint: "/v1/chat/completions",
	}, "zero-priced-model", gatewayPricingEvidence{PricingSource: "default_zero"})

	got := logs.String()
	if !strings.Contains(got, "gateway usage recorded with default zero pricing") {
		t.Fatalf("expected default zero pricing warning, got %q", got)
	}
	if !strings.Contains(got, "req_default_zero") || !strings.Contains(got, "zero-priced-model") {
		t.Fatalf("expected request and model fields in warning, got %q", got)
	}
}

func TestWarnDefaultZeroGatewayPricingIgnoresExplicitSources(t *testing.T) {
	var logs bytes.Buffer
	rt := &runtimeState{
		logger: slog.New(slog.NewTextHandler(&logs, nil)),
	}

	rt.warnDefaultZeroGatewayPricing(gatewayUsageRecord{
		RequestID:      "req_priced",
		SourceEndpoint: "/v1/chat/completions",
	}, "priced-model", gatewayPricingEvidence{PricingSource: "pricing_rule"})

	if got := logs.String(); got != "" {
		t.Fatalf("did not expect warning for explicit pricing source, got %q", got)
	}
}

func TestGatewayConfiguredErrorCooldownRulesMatchCanonicalAndLegacyMetadata(t *testing.T) {
	tests := []struct {
		name            string
		metadata        map[string]any
		errorClass      string
		statusCode      int
		providerMessage string
		wantReason      string
		wantWindow      time.Duration
	}{
		{
			name: "canonical status class and keyword",
			metadata: map[string]any{
				"error_cooldown_rules": []any{
					map[string]any{
						"status_code":      float64(503),
						"error_class":      "provider_5xx",
						"keywords":         []any{"CAPACITY"},
						"cooldown_seconds": float64(90),
						"reason":           "Provider Capacity",
					},
				},
			},
			errorClass:      "provider_5xx",
			statusCode:      http.StatusServiceUnavailable,
			providerMessage: "capacity unavailable",
			wantReason:      "provider_capacity",
			wantWindow:      90 * time.Second,
		},
		{
			name: "legacy temp unschedulable rule",
			metadata: map[string]any{
				"temp_unschedulable_enabled": true,
				"temp_unschedulable_rules": []any{
					map[string]any{
						"error_code":       float64(401),
						"keywords":         []any{"unauthorized"},
						"duration_minutes": float64(10),
					},
				},
			},
			errorClass:      "auth_failed",
			statusCode:      http.StatusUnauthorized,
			providerMessage: "unauthorized",
			wantReason:      "temp_unschedulable",
			wantWindow:      10 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, ok := gatewayCooldownDecisionForFailure(tt.metadata, tt.errorClass, ptrInt(tt.statusCode), tt.providerMessage, nil)
			if !ok {
				t.Fatalf("expected configured cooldown decision")
			}
			if decision.Reason != tt.wantReason || decision.Window != tt.wantWindow || decision.LastErrorClass != tt.errorClass {
				t.Fatalf("unexpected cooldown decision: %+v", decision)
			}
		})
	}
}

func TestGatewayAccountFailureStatusHandled(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]any
		statusCode *int
		want       bool
	}{
		{
			name:       "unconfigured handles all",
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       true,
		},
		{
			name: "canonical list hit",
			metadata: map[string]any{
				"handled_error_status_codes": []any{float64(401), "429"},
			},
			statusCode: ptrInt(http.StatusTooManyRequests),
			want:       true,
		},
		{
			name: "canonical list miss",
			metadata: map[string]any{
				"handled_error_status_codes": []any{float64(401), "429"},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       false,
		},
		{
			name: "legacy custom codes hit",
			metadata: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(403)},
			},
			statusCode: ptrInt(http.StatusForbidden),
			want:       true,
		},
		{
			name: "legacy custom codes miss",
			metadata: map[string]any{
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(429)},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       false,
		},
		{
			name: "pool mode skips without custom codes",
			metadata: map[string]any{
				"pool_mode": true,
			},
			statusCode: ptrInt(http.StatusUnauthorized),
			want:       false,
		},
		{
			name: "pool mode custom codes take precedence",
			metadata: map[string]any{
				"pool_mode":                  true,
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(401)},
			},
			statusCode: ptrInt(http.StatusUnauthorized),
			want:       true,
		},
		{
			name: "empty configured list handles all",
			metadata: map[string]any{
				"handled_error_status_codes": []any{},
			},
			statusCode: ptrInt(http.StatusServiceUnavailable),
			want:       true,
		},
		{
			name:       "missing status handles",
			statusCode: nil,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gatewayAccountFailureStatusHandled(tt.metadata, tt.statusCode); got != tt.want {
				t.Fatalf("gatewayAccountFailureStatusHandled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRecordGatewayUsagePersistsProviderQuotaSignals(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "quota-signal-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	usage, err := usageservice.New(usagememory.New(), nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	scheduler, err := schedulerservice.New(schedulermemory.New(), nil)
	if err != nil {
		t.Fatalf("new scheduler service: %v", err)
	}
	rt := &runtimeState{
		logger:    slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts:  accounts,
		usage:     usage,
		events:    events,
		scheduler: scheduler,
	}
	resetAt := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_quota_signal",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		DecisionID:     1,
		AttemptNo:      1,
		ProviderID:     ptrInt(account.ProviderID),
		AccountID:      ptrInt(account.ID),
		SourceEndpoint: "/v1/responses",
		TargetProtocol: "openai-compatible",
		Model:          "codex-model",
		Success:        true,
		ProviderQuotaSignals: []provideradaptercontract.QuotaSignal{{
			QuotaType:      "codex_5h_percent",
			Remaining:      "66",
			Used:           "34",
			QuotaLimit:     "100",
			RemainingRatio: 0.66,
			ResetAt:        &resetAt,
			SnapshotAt:     resetAt.Add(-time.Hour),
		}},
	})

	quotas, err := accounts.ListQuotaSnapshotsByAccount(ctx, account.ID, 10)
	if err != nil {
		t.Fatalf("list quota snapshots: %v", err)
	}
	var codexQuota *accountcontract.AccountQuotaSnapshot
	for i := range quotas {
		if quotas[i].QuotaType == "codex_5h_percent" {
			codexQuota = &quotas[i]
			break
		}
	}
	if codexQuota == nil {
		t.Fatalf("expected codex quota snapshot, got %+v", quotas)
	}
	if codexQuota.Used != "34" || codexQuota.Remaining != "66" || codexQuota.QuotaLimit != "100" || codexQuota.RemainingRatio != 0.66 {
		t.Fatalf("unexpected codex quota snapshot: %+v", *codexQuota)
	}
	if codexQuota.ResetAt == nil || !codexQuota.ResetAt.Equal(resetAt) {
		t.Fatalf("unexpected codex reset time: %+v", codexQuota.ResetAt)
	}
}

func TestAccountSchedulerRuntimeStateIgnoresSyntheticQuotaSnapshots(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   12,
		Name:         "real-quota-account",
		RuntimeClass: accountcontract.RuntimeClassCliClientToken,
		Credential:   map[string]any{"cli_client_token": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	now := time.Date(2026, 6, 8, 10, 0, 0, 0, time.UTC)
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      "codex_5h_percent",
		Remaining:      "42",
		Used:           "58",
		QuotaLimit:     "100",
		RemainingRatio: 0.42,
		SnapshotAt:     now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("record real quota: %v", err)
	}
	if _, err := accounts.RecordQuotaSnapshot(ctx, accountcontract.AccountQuotaSnapshot{
		AccountID:      account.ID,
		ProviderID:     account.ProviderID,
		QuotaType:      accountcontract.QuotaTypeSyntheticMonthlyTokens,
		Remaining:      "unlimited",
		Used:           "1",
		QuotaLimit:     "unlimited",
		RemainingRatio: 1,
		SnapshotAt:     now,
	}); err != nil {
		t.Fatalf("record synthetic quota: %v", err)
	}
	rt := &runtimeState{accounts: accounts}

	state := rt.accountSchedulerRuntimeState(ctx, account)
	if state.QuotaRemainingRatio == nil || math.Abs(*state.QuotaRemainingRatio-0.42) > 0.000001 {
		t.Fatalf("expected scheduler to use real quota ratio 0.42, got %+v", state.QuotaRemainingRatio)
	}
	if state.QuotaExhausted {
		t.Fatalf("did not expect real quota to be exhausted: %+v", state)
	}
}

func TestRecordGatewayUsageAppliesAccountGroupRateMultiplier(t *testing.T) {
	ctx := context.Background()
	accounts, err := accountservice.New(accountmemory.New(), "0123456789abcdef0123456789abcdef", nil)
	if err != nil {
		t.Fatalf("new account service: %v", err)
	}
	account, err := accounts.Create(ctx, accountcontract.CreateRequest{
		ProviderID:   21,
		Name:         "discount-account",
		RuntimeClass: accountcontract.RuntimeClassAPIKey,
		Credential:   map[string]any{"api_key": "secret"},
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	rateMultiplier := "0.80000000"
	group, err := accounts.CreateGroup(ctx, accountcontract.CreateGroupRequest{
		Name:           "discount-group",
		RateMultiplier: &rateMultiplier,
	})
	if err != nil {
		t.Fatalf("create account group: %v", err)
	}
	if _, err := accounts.AddAccountToGroup(ctx, account.ID, group.ID); err != nil {
		t.Fatalf("add account to group: %v", err)
	}
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger:   slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		accounts: accounts,
		usage:    usage,
		events:   events,
	}

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_group_multiplier",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		ProviderID:     ptrInt(account.ProviderID),
		AccountID:      ptrInt(account.ID),
		SourceEndpoint: "/v1/chat/completions",
		TargetProtocol: "openai-compatible",
		Model:          "claude-opus-test",
		Success:        true,
		Pricing:        gatewayPricingEvidence{Amount: "0.01000000", Currency: "USD", PricingSource: "pricing_rule"},
	})

	logs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one usage log, got %+v", logs)
	}
	log := logs[0]
	if log.Cost != "0.01000000" || log.ActualCost != "0.00800000" || log.BillableCost != "0.00800000" || log.RateMultiplier != "0.80000000" {
		t.Fatalf("expected multiplier snapshot and actual cost, got %+v", log)
	}
}

func TestRecordGatewayUsagePersistsCostBreakdownAndModelSnapshots(t *testing.T) {
	ctx := context.Background()
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	rt := &runtimeState{
		logger: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		usage:  usage,
		events: events,
	}

	rt.recordGatewayUsage(ctx, gatewayUsageRecord{
		RequestID:      "req_breakdown_snapshot",
		Authed:         apikeycontract.AuthResult{UserID: 1, Key: apikeycontract.APIKey{ID: 2}},
		SourceEndpoint: "/v1/images/generations",
		Model:          "canonical-image-model",
		RequestedModel: "public-image-alias",
		UpstreamModel:  "upstream-image-model",
		Success:        true,
		Pricing: gatewayPricingEvidence{
			Amount:         "0.12000000",
			Currency:       "USD",
			BillingMode:    "image",
			InputCost:      "0.01000000",
			OutputCost:     "0.10000000",
			CacheReadCost:  "0.00500000",
			CacheWriteCost: "0.00500000",
			PricingSource:  "pricing_rule",
		},
	})

	logs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected one usage log, got %+v", logs)
	}
	log := logs[0]
	sum := addUsageCostBreakdown(log.InputCost, log.OutputCost, log.CacheReadCost, log.CacheWriteCost)
	if sum != log.Cost {
		t.Fatalf("expected breakdown sum %s to equal total %s in %+v", sum, log.Cost, log)
	}
	if log.RequestedModel != "public-image-alias" || log.UpstreamModel != "upstream-image-model" || log.BillingMode != "image" {
		t.Fatalf("expected model and billing snapshots, got %+v", log)
	}
}

func TestRecordGatewayUsageMaterializedCostMatchesUsageLogBillableSum(t *testing.T) {
	ctx := context.Background()
	usageStore := usagememory.New()
	usage, err := usageservice.New(usageStore, nil)
	if err != nil {
		t.Fatalf("new usage service: %v", err)
	}
	events, err := eventsservice.New(eventsmemory.New(), nil)
	if err != nil {
		t.Fatalf("new events service: %v", err)
	}
	billing, err := billingservice.New(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing service: %v", err)
	}
	subscriptions, err := subscriptionservice.New(subscriptionmemory.New(), nil)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}
	apiKeys, err := apikeyservice.New(apikeymemory.New(), strings.Repeat("k", 32), nil)
	if err != nil {
		t.Fatalf("new api key service: %v", err)
	}
	plan, err := subscriptions.CreatePlan(ctx, subscriptioncontract.CreatePlanRequest{
		Name:         "materialized-pro",
		Price:        "9.99",
		Currency:     "USD",
		ValidityDays: 30,
		Entitlements: map[string]any{"monthly_cost_quota": "1.00000000"},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := subscriptions.CreateUserSubscription(ctx, subscriptioncontract.CreateSubscriptionRequest{
		UserID: 1,
		PlanID: plan.ID,
	}); err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	createdKey, err := apiKeys.Create(ctx, apikeycontract.CreateRequest{UserID: 1, Name: "materialized-key"})
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	rt := &runtimeState{
		logger:        slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		usage:         usage,
		events:        events,
		billing:       billing,
		subscriptions: subscriptions,
		apiKeys:       apiKeys,
	}
	for i, amount := range []string{"0.01000000", "0.02500000", "0.00500000"} {
		rt.recordGatewayUsage(ctx, gatewayUsageRecord{
			RequestID:      "req_materialized_" + strconv.Itoa(i+1),
			Authed:         apikeycontract.AuthResult{UserID: 1, Key: createdKey.Key},
			SourceEndpoint: "/v1/chat/completions",
			TargetProtocol: "openai-compatible",
			Model:          "materialized-model",
			Success:        true,
			Pricing:        gatewayPricingEvidence{Amount: amount, Currency: "USD", PricingSource: "pricing_rule"},
		})
	}

	logs, err := usageStore.List(ctx)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	sum := "0.00000000"
	for _, log := range logs {
		sum = money.AddMoney(sum, log.BillableCost)
	}
	if sum != "0.04000000" {
		t.Fatalf("unexpected billable cost sum %s from logs %+v", sum, logs)
	}
	materialized, err := subscriptions.MaterializedUsageForUser(ctx, 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("read materialized usage: %v", err)
	}
	if materialized.MonthlyUsageUSD != sum || materialized.DailyUsageUSD != sum || materialized.WeeklyUsageUSD != sum {
		t.Fatalf("expected materialized usage to match log sum %s, got %+v", sum, materialized)
	}
	key, err := apiKeys.GetByID(ctx, createdKey.Key.ID)
	if err != nil {
		t.Fatalf("find api key: %v", err)
	}
	if key.CostUsed != sum || key.CostUsed5h != sum || key.CostUsed1d != sum || key.CostUsed7d != sum {
		t.Fatalf("expected api key materialized cost to match log sum %s, got %+v", sum, key)
	}
}

func addUsageCostBreakdown(input, output, cacheRead, cacheWrite string) string {
	sum := money.AddMoney(input, output)
	sum = money.AddMoney(sum, cacheRead)
	sum = money.AddMoney(sum, cacheWrite)
	return sum
}
