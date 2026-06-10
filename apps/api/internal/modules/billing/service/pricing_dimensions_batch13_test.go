package service_test

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
)

func TestEstimatePriceServiceTierMultipliers(t *testing.T) {
	svc := newPricingService(t)
	rule, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         20,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1",
		OutputPricePerMillionTokens:     "1",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create pricing rule: %v", err)
	}

	cases := []struct {
		name string
		tier string
		want string
	}{
		{name: "default", want: "1.00000000"},
		{name: "priority", tier: "priority", want: "2.00000000"},
		{name: "flex", tier: "flex", want: "0.50000000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
				ModelID:      rule.ModelID,
				ProviderID:   rule.ProviderID,
				InputTokens:  500000,
				OutputTokens: 500000,
				ServiceTier:  tc.tier,
			})
			if err != nil {
				t.Fatalf("estimate price: %v", err)
			}
			if got.Amount != tc.want {
				t.Fatalf("amount = %s, want %s", got.Amount, tc.want)
			}
		})
	}
}

func TestEstimatePriceCacheCreationTTLBreakdown(t *testing.T) {
	svc := newPricingService(t)
	rule, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                           21,
		ProviderID:                        0,
		InputPricePerMillionTokens:        "1",
		OutputPricePerMillionTokens:       "0",
		CacheReadPricePerMillionTokens:    "0",
		CacheWritePricePerMillionTokens:   "2",
		CacheWrite5mPricePerMillionTokens: "2",
		CacheWrite1hPricePerMillionTokens: "4",
		Currency:                          "usd",
	})
	if err != nil {
		t.Fatalf("create pricing rule: %v", err)
	}

	withBuckets, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:            rule.ModelID,
		ProviderID:         rule.ProviderID,
		CacheWriteTokens:   2_000_000,
		CacheWrite5mTokens: 1_000_000,
		CacheWrite1hTokens: 1_000_000,
	})
	if err != nil {
		t.Fatalf("estimate bucketed cache write: %v", err)
	}
	if withBuckets.CacheWriteCost != "6.00000000" {
		t.Fatalf("bucketed cache write cost = %s, want 6.00000000", withBuckets.CacheWriteCost)
	}

	fallback, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:          rule.ModelID,
		ProviderID:       rule.ProviderID,
		CacheWriteTokens: 2_000_000,
	})
	if err != nil {
		t.Fatalf("estimate fallback cache write: %v", err)
	}
	if fallback.CacheWriteCost != "4.00000000" {
		t.Fatalf("fallback cache write cost = %s, want 4.00000000", fallback.CacheWriteCost)
	}
}

func TestEstimatePriceLongContextMultiplierOnlyWithoutIntervals(t *testing.T) {
	svc := newPricingService(t)
	threshold := 1000
	noInterval, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         22,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1",
		OutputPricePerMillionTokens:     "1",
		CacheReadPricePerMillionTokens:  "1",
		CacheWritePricePerMillionTokens: "1",
		LongContextThresholdTokens:      &threshold,
		LongContextMultiplier:           "2",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create long context rule: %v", err)
	}
	withInterval, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         23,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1",
		OutputPricePerMillionTokens:     "1",
		CacheReadPricePerMillionTokens:  "1",
		CacheWritePricePerMillionTokens: "1",
		LongContextThresholdTokens:      &threshold,
		LongContextMultiplier:           "2",
		Intervals: []contract.PricingInterval{{
			MinTokens:                       1000,
			InputPricePerMillionTokens:      "3",
			OutputPricePerMillionTokens:     "3",
			CacheReadPricePerMillionTokens:  "3",
			CacheWritePricePerMillionTokens: "3",
		}},
		Currency: "usd",
	})
	if err != nil {
		t.Fatalf("create interval long context rule: %v", err)
	}

	multiplied, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:     noInterval.ModelID,
		ProviderID:  noInterval.ProviderID,
		InputTokens: 1_000_000,
	})
	if err != nil {
		t.Fatalf("estimate multiplied price: %v", err)
	}
	if multiplied.Amount != "2.00000000" {
		t.Fatalf("long context amount = %s, want 2.00000000", multiplied.Amount)
	}

	interval, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:     withInterval.ModelID,
		ProviderID:  withInterval.ProviderID,
		InputTokens: 1_000_000,
	})
	if err != nil {
		t.Fatalf("estimate interval price: %v", err)
	}
	if interval.Amount != "3.00000000" {
		t.Fatalf("interval amount = %s, want 3.00000000 without long-context multiplier", interval.Amount)
	}
}

func TestEstimatePriceImageOutputTokenRate(t *testing.T) {
	svc := newPricingService(t)
	rule, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                          24,
		ProviderID:                       0,
		InputPricePerMillionTokens:       "0",
		OutputPricePerMillionTokens:      "1",
		ImageOutputPricePerMillionTokens: "5",
		CacheReadPricePerMillionTokens:   "0",
		CacheWritePricePerMillionTokens:  "0",
		Currency:                         "usd",
	})
	if err != nil {
		t.Fatalf("create image output token rule: %v", err)
	}
	got, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:           rule.ModelID,
		ProviderID:        rule.ProviderID,
		OutputTokens:      1_000_000,
		ImageOutputTokens: 200_000,
	})
	if err != nil {
		t.Fatalf("estimate image output price: %v", err)
	}
	if got.OutputCost != "1.80000000" || got.Amount != "1.80000000" {
		t.Fatalf("expected 800k text output at 1 + 200k image output at 5, got %+v", got)
	}
}

func TestEstimatePriceBillingModelSource(t *testing.T) {
	store := billingmemory.New()
	storeSvc, err := service.NewPricing(store, nil)
	if err != nil {
		t.Fatalf("new pricing service: %v", err)
	}
	if _, err := store.CreatePricingRule(t.Context(), contract.PricingRule{
		ModelID:                         30,
		ModelFamily:                     "requested-model",
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1",
		OutputPricePerMillionTokens:     "0",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	}); err != nil {
		t.Fatalf("create requested rule: %v", err)
	}
	if _, err := store.CreatePricingRule(t.Context(), contract.PricingRule{
		ModelID:                         31,
		ModelFamily:                     "upstream-model",
		ProviderID:                      0,
		InputPricePerMillionTokens:      "10",
		OutputPricePerMillionTokens:     "0",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		Currency:                        "usd",
	}); err != nil {
		t.Fatalf("create upstream rule: %v", err)
	}

	requested, err := storeSvc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:            31,
		ProviderID:         0,
		RequestedModel:     "requested-model",
		UpstreamModel:      "upstream-model",
		BillingModelSource: "requested",
		InputTokens:        1_000_000,
	})
	if err != nil {
		t.Fatalf("estimate requested source: %v", err)
	}
	if requested.Amount != "1.00000000" {
		t.Fatalf("requested source amount = %s, want 1.00000000", requested.Amount)
	}

	upstream, err := storeSvc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:            30,
		ProviderID:         0,
		RequestedModel:     "requested-model",
		UpstreamModel:      "upstream-model",
		BillingModelSource: "upstream",
		InputTokens:        1_000_000,
	})
	if err != nil {
		t.Fatalf("estimate upstream source: %v", err)
	}
	if upstream.Amount != "10.00000000" {
		t.Fatalf("upstream source amount = %s, want 10.00000000", upstream.Amount)
	}
}
