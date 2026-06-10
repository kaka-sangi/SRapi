package service_test

import (
	"context"
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
)

func TestEstimatePriceMatchesDecimalBaselines(t *testing.T) {
	svc, err := service.NewPricing(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
	}
	if _, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1.50000000",
		OutputPricePerMillionTokens:     "2.50000000",
		CacheReadPricePerMillionTokens:  "0.50000000",
		CacheWritePricePerMillionTokens: "0.25000000",
		Currency:                        "usd",
	}); err != nil {
		t.Fatalf("create usd pricing rule: %v", err)
	}
	if _, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         2,
		ProviderID:                      7,
		InputPricePerMillionTokens:      "3.00000000",
		OutputPricePerMillionTokens:     "4.00000000",
		CacheReadPricePerMillionTokens:  "0.30000000",
		CacheWritePricePerMillionTokens: "0.00000000",
		Currency:                        "eur",
	}); err != nil {
		t.Fatalf("create eur pricing rule: %v", err)
	}

	tests := []struct {
		name string
		req  contract.PricingRequest
		want contract.PricingResult
	}{
		{
			name: "normal input output",
			req: contract.PricingRequest{
				ModelID:      1,
				ProviderID:   0,
				InputTokens:  1000,
				OutputTokens: 2000,
			},
			want: contract.PricingResult{Amount: "0.00650000", Currency: "USD"},
		},
		{
			name: "cache read and write",
			req: contract.PricingRequest{
				ModelID:          1,
				ProviderID:       0,
				InputTokens:      1000,
				CacheReadTokens:  3000,
				CacheWriteTokens: 4000,
			},
			want: contract.PricingResult{Amount: "0.00400000", Currency: "USD"},
		},
		{
			name: "zero cache write falls back to input",
			req: contract.PricingRequest{
				ModelID:          2,
				ProviderID:       7,
				CacheWriteTokens: 1000,
			},
			want: contract.PricingResult{Amount: "0.00300000", Currency: "EUR"},
		},
		{
			name: "multi currency override",
			req: contract.PricingRequest{
				ModelID:      1,
				ProviderID:   0,
				InputTokens:  1000,
				OutputTokens: 1000,
				PricingOverride: map[string]any{
					"input_price_per_million_tokens":  "3.00000000",
					"output_price_per_million_tokens": "4.00000000",
					"currency":                        "cny",
				},
			},
			want: contract.PricingResult{Amount: "0.00700000", Currency: "CNY"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.EstimatePrice(t.Context(), tt.req)
			if err != nil {
				t.Fatalf("estimate price: %v", err)
			}
			if got.Amount != tt.want.Amount || got.Currency != tt.want.Currency {
				t.Fatalf("EstimatePrice = %+v, want amount=%s currency=%s", got, tt.want.Amount, tt.want.Currency)
			}
		})
	}
}

func TestEstimatePriceUsesDecimalSafeProviderSpecificRulesAndOverrides(t *testing.T) {
	svc, err := service.NewPricing(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
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

func TestEstimatePriceFallsBackToModelFamilyRule(t *testing.T) {
	store := billingmemory.New()
	svc, err := service.NewPricing(store, nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
	}
	rule, err := store.CreatePricingRule(t.Context(), contract.PricingRule{
		ModelID:                         10,
		ModelFamily:                     "opus",
		ProviderID:                      0,
		InputPricePerMillionTokens:      "15.00000000",
		OutputPricePerMillionTokens:     "75.00000000",
		CacheReadPricePerMillionTokens:  "1.50000000",
		CacheWritePricePerMillionTokens: "15.00000000",
		Currency:                        "USD",
	})
	if err != nil {
		t.Fatalf("create family fallback pricing rule: %v", err)
	}

	estimated, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:      99,
		ModelFamily:  "opus",
		ProviderID:   7,
		InputTokens:  1000,
		OutputTokens: 1000,
	})
	if err != nil {
		t.Fatalf("estimate family fallback price: %v", err)
	}
	if estimated.Amount != "0.09000000" || estimated.Currency != "USD" {
		t.Fatalf("expected non-zero opus family fallback price, got %+v", estimated)
	}
	if estimated.PricingRuleID == nil || *estimated.PricingRuleID != rule.ID {
		t.Fatalf("expected fallback rule id %d, got %+v", rule.ID, estimated.PricingRuleID)
	}
}

func TestEstimatePriceUsesBoundedPricingRuleQuery(t *testing.T) {
	store := &queryTrackingPricingStore{Store: billingmemory.New()}
	svc, err := service.NewPricing(store, nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
	}
	generic, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "9",
		OutputPricePerMillionTokens:     "9",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create generic rule: %v", err)
	}
	specific, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         1,
		ProviderID:                      7,
		InputPricePerMillionTokens:      "1",
		OutputPricePerMillionTokens:     "2",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create provider rule: %v", err)
	}
	if _, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         2,
		ProviderID:                      8,
		InputPricePerMillionTokens:      "100",
		OutputPricePerMillionTokens:     "100",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	}); err != nil {
		t.Fatalf("create unrelated rule: %v", err)
	}

	estimated, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:      1,
		ProviderID:   7,
		InputTokens:  1000,
		OutputTokens: 1000,
	})
	if err != nil {
		t.Fatalf("estimate price: %v", err)
	}
	if estimated.Amount != "0.00300000" || estimated.Currency != "USD" {
		t.Fatalf("unexpected price result: %+v", estimated)
	}
	if estimated.PricingRuleID == nil || *estimated.PricingRuleID != specific.ID || *estimated.PricingRuleID == generic.ID {
		t.Fatalf("expected provider-specific rule %d, got %+v", specific.ID, estimated.PricingRuleID)
	}
	if store.queryCalls != 1 {
		t.Fatalf("expected one bounded query, got %d", store.queryCalls)
	}
	if store.listCalls != 0 {
		t.Fatalf("EstimatePrice should not use full ListPricingRules, got %d calls", store.listCalls)
	}
}

type queryTrackingPricingStore struct {
	*billingmemory.Store
	queryCalls int
	listCalls  int
}

func (s *queryTrackingPricingStore) QueryPricingRules(ctx context.Context, query contract.PricingRuleQuery) ([]contract.PricingRule, error) {
	s.queryCalls++
	return s.Store.QueryPricingRules(ctx, query)
}

func (s *queryTrackingPricingStore) ListPricingRules(ctx context.Context) ([]contract.PricingRule, error) {
	s.listCalls++
	return s.Store.ListPricingRules(ctx)
}

func TestValidatePricingRuleDoesNotPersist(t *testing.T) {
	svc, err := service.NewPricing(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
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
