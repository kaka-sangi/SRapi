package service_test

import (
	"testing"
	"time"

	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/service"
	subscriptionmemory "github.com/srapi/srapi/apps/api/internal/modules/subscriptions/store/memory"
)

func TestCheckEntitlementRejectsBeforeSchedulingAndCarriesRoutingPolicy(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	svc, err := service.New(subscriptionmemory.New(), clock)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}

	plan, err := svc.CreatePlan(t.Context(), contract.CreatePlanRequest{
		Name:         "pro",
		Price:        "9.99",
		Currency:     "usd",
		ValidityDays: 30,
		Entitlements: map[string]any{
			"allowed_models":      []any{"allowed-model", "alias-model"},
			"account_group_scope": []any{10, 20},
			"scheduler_strategy":  "cost_saver",
			"monthly_token_quota": 12,
			"monthly_cost_quota":  "0.00001000",
		},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := svc.CreateUserSubscription(t.Context(), contract.CreateSubscriptionRequest{
		UserID: 1,
		PlanID: plan.ID,
	}); err != nil {
		t.Fatalf("create user subscription: %v", err)
	}

	allowed, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:          1,
		ModelReferences: []string{"allowed-model"},
		EstimatedTokens: 2,
		EstimatedCost:   "0.00000100",
		RequestTime:     clock.now,
	})
	if err != nil {
		t.Fatalf("check allowed entitlement: %v", err)
	}
	if !allowed.Allowed || allowed.Reason != "allowed" || allowed.SchedulerStrategy != "cost_saver" {
		t.Fatalf("unexpected allowed entitlement: %+v", allowed)
	}
	if len(allowed.AccountGroupScope) != 2 || allowed.AccountGroupScope[0] != 10 || allowed.AccountGroupScope[1] != 20 {
		t.Fatalf("expected routing group scope, got %+v", allowed.AccountGroupScope)
	}
	if allowed.MonthlyTokenQuota == nil || *allowed.MonthlyTokenQuota != 12 {
		t.Fatalf("expected token quota snapshot, got %+v", allowed.MonthlyTokenQuota)
	}
	if allowed.MonthlyCostQuota == nil || *allowed.MonthlyCostQuota != "0.00001000" {
		t.Fatalf("expected normalized cost quota, got %+v", allowed.MonthlyCostQuota)
	}

	disallowedModel, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:          1,
		ModelReferences: []string{"blocked-model"},
		EstimatedTokens: 1,
		EstimatedCost:   "0.00000100",
		RequestTime:     clock.now,
	})
	if err != nil {
		t.Fatalf("check model entitlement: %v", err)
	}
	if disallowedModel.Allowed || disallowedModel.Reason != "model_not_allowed" {
		t.Fatalf("expected model entitlement rejection, got %+v", disallowedModel)
	}

	tokenQuota, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:             1,
		ModelReferences:    []string{"allowed-model"},
		EstimatedTokens:    3,
		TokensUsedInPeriod: 10,
		EstimatedCost:      "0.00000100",
		RequestTime:        clock.now,
	})
	if err != nil {
		t.Fatalf("check token quota entitlement: %v", err)
	}
	if tokenQuota.Allowed || tokenQuota.Reason != "monthly_token_quota_exceeded" {
		t.Fatalf("expected token quota rejection, got %+v", tokenQuota)
	}

	costQuota, err := svc.CheckEntitlement(t.Context(), contract.EntitlementCheckRequest{
		UserID:           1,
		ModelReferences:  []string{"allowed-model"},
		EstimatedTokens:  1,
		CostUsedInPeriod: "0.00000900",
		EstimatedCost:    "0.00000200",
		RequestTime:      clock.now,
	})
	if err != nil {
		t.Fatalf("check cost quota entitlement: %v", err)
	}
	if costQuota.Allowed || costQuota.Reason != "monthly_cost_quota_exceeded" {
		t.Fatalf("expected cost quota rejection, got %+v", costQuota)
	}
}

func TestEstimatePriceUsesDecimalSafeProviderSpecificRulesAndOverrides(t *testing.T) {
	svc, err := service.New(subscriptionmemory.New(), nil)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}

	generic, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "9",
		OutputPricePerMillionTokens:     "9",
		CacheReadPricePerMillionTokens:  "9",
		CacheWritePricePerMillionTokens: "9",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create generic pricing rule: %v", err)
	}
	specific, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      7,
		InputPricePerMillionTokens:      "1.5",
		OutputPricePerMillionTokens:     "2.5",
		CacheReadPricePerMillionTokens:  "0.5",
		CacheWritePricePerMillionTokens: "0.25",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create provider pricing rule: %v", err)
	}

	estimated, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:          1,
		ProviderID:       7,
		InputTokens:      1000,
		OutputTokens:     2000,
		CacheReadTokens:  3000,
		CacheWriteTokens: 4000,
	})
	if err != nil {
		t.Fatalf("estimate provider-specific price: %v", err)
	}
	if estimated.Amount != "0.00900000" || estimated.Currency != "USD" {
		t.Fatalf("expected decimal-safe provider-specific amount, got %+v", estimated)
	}
	if estimated.PricingRuleID == nil || *estimated.PricingRuleID != specific.ID || *estimated.PricingRuleID == generic.ID {
		t.Fatalf("expected provider-specific rule id %d, got %+v", specific.ID, estimated.PricingRuleID)
	}

	override, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:      1,
		ProviderID:   7,
		InputTokens:  1000,
		OutputTokens: 1000,
		PricingOverride: map[string]any{
			"input_price_per_million_tokens":  "3.0",
			"output_price_per_million_tokens": "4.0",
			"currency":                        "eur",
		},
	})
	if err != nil {
		t.Fatalf("estimate override price: %v", err)
	}
	if override.Amount != "0.00700000" || override.Currency != "EUR" || override.PricingRuleID != nil {
		t.Fatalf("expected mapping override to take precedence without rule id, got %+v", override)
	}
}

func TestValidatePricingRuleDoesNotPersist(t *testing.T) {
	svc, err := service.New(subscriptionmemory.New(), nil)
	if err != nil {
		t.Fatalf("new subscription service: %v", err)
	}

	if err := svc.ValidatePricingRule(contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1.25",
		OutputPricePerMillionTokens:     "2.50",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	}); err != nil {
		t.Fatalf("validate pricing rule: %v", err)
	}
	rules, err := svc.ListPricingRules(t.Context())
	if err != nil {
		t.Fatalf("list pricing rules: %v", err)
	}
	if len(rules) != 0 {
		t.Fatalf("validation should not persist pricing rules, got %+v", rules)
	}
	if err := svc.ValidatePricingRule(contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "not-money",
		OutputPricePerMillionTokens:     "2.50",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	}); err == nil {
		t.Fatal("expected invalid pricing rule to be rejected")
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}
