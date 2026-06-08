package httpserver

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	accountcontract "github.com/srapi/srapi/apps/api/internal/modules/accounts/contract"
	accountservice "github.com/srapi/srapi/apps/api/internal/modules/accounts/service"
	accountmemory "github.com/srapi/srapi/apps/api/internal/modules/accounts/store/memory"
	apikeycontract "github.com/srapi/srapi/apps/api/internal/modules/api_keys/contract"
	eventsservice "github.com/srapi/srapi/apps/api/internal/modules/events/service"
	eventsmemory "github.com/srapi/srapi/apps/api/internal/modules/events/store/memory"
	provideradaptercontract "github.com/srapi/srapi/apps/api/internal/modules/provider_adapters/contract"
	schedulerservice "github.com/srapi/srapi/apps/api/internal/modules/scheduler/service"
	schedulermemory "github.com/srapi/srapi/apps/api/internal/modules/scheduler/store/memory"
	usageservice "github.com/srapi/srapi/apps/api/internal/modules/usage/service"
	usagememory "github.com/srapi/srapi/apps/api/internal/modules/usage/store/memory"
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
