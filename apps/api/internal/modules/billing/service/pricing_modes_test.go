package service_test

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	"github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	"github.com/srapi/srapi/apps/api/internal/pkg/money"
)

func TestEstimatePricePerRequestBillsZeroTokenRequests(t *testing.T) {
	svc := newPricingService(t)
	rule, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         10,
		ProviderID:                      0,
		BillingMode:                     contract.BillingModePerRequest,
		InputPricePerMillionTokens:      "0",
		OutputPricePerMillionTokens:     "0",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		PerRequestPrice:                 "0.125",
		Currency:                        "usd",
	})
	if err != nil {
		t.Fatalf("create per-request rule: %v", err)
	}

	got, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:    rule.ModelID,
		ProviderID: rule.ProviderID,
	})
	if err != nil {
		t.Fatalf("estimate per-request price: %v", err)
	}
	if got.Amount != "0.12500000" || got.InputCost != "0.12500000" || got.BillingMode != contract.BillingModePerRequest {
		t.Fatalf("expected non-zero per-request pricing, got %+v", got)
	}
	assertPricingBreakdownSums(t, got)
}

func TestEstimatePriceImageBillsCountByTier(t *testing.T) {
	svc := newPricingService(t)
	rule, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         11,
		ProviderID:                      0,
		BillingMode:                     contract.BillingModeImage,
		InputPricePerMillionTokens:      "0",
		OutputPricePerMillionTokens:     "0",
		CacheReadPricePerMillionTokens:  "0",
		CacheWritePricePerMillionTokens: "0",
		PerRequestPrice:                 "0.01000000",
		Intervals: []contract.PricingInterval{
			{ImageSize: "1024x1024", PerImagePrice: "0.04000000"},
			{ImageSize: "2048x2048", PerImagePrice: "0.09000000"},
		},
		Currency: "usd",
	})
	if err != nil {
		t.Fatalf("create image rule: %v", err)
	}

	got, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:    rule.ModelID,
		ProviderID: rule.ProviderID,
		ImageCount: 3,
		ImageSize:  "2048x2048",
	})
	if err != nil {
		t.Fatalf("estimate image price: %v", err)
	}
	if got.Amount != "0.27000000" || got.OutputCost != "0.27000000" || got.BillingMode != contract.BillingModeImage {
		t.Fatalf("expected image tier pricing, got %+v", got)
	}
	assertPricingBreakdownSums(t, got)
}

func TestEstimatePriceTokenIntervalsAndFlatFallback(t *testing.T) {
	svc := newPricingService(t)
	rule, err := svc.CreatePricingRule(t.Context(), contract.CreatePricingRuleRequest{
		ModelID:                         12,
		ProviderID:                      0,
		InputPricePerMillionTokens:      "1.00000000",
		OutputPricePerMillionTokens:     "2.00000000",
		CacheReadPricePerMillionTokens:  "0.50000000",
		CacheWritePricePerMillionTokens: "0.25000000",
		Intervals: []contract.PricingInterval{
			{
				MinTokens:                       200000,
				InputPricePerMillionTokens:      "2.00000000",
				OutputPricePerMillionTokens:     "4.00000000",
				CacheReadPricePerMillionTokens:  "1.00000000",
				CacheWritePricePerMillionTokens: "0.50000000",
			},
		},
		Currency: "usd",
	})
	if err != nil {
		t.Fatalf("create interval rule: %v", err)
	}

	flat, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:          rule.ModelID,
		ProviderID:       rule.ProviderID,
		InputTokens:      1000,
		OutputTokens:     2000,
		CacheReadTokens:  3000,
		CacheWriteTokens: 4000,
	})
	if err != nil {
		t.Fatalf("estimate flat token price: %v", err)
	}
	if flat.Amount != "0.00750000" {
		t.Fatalf("expected flat fallback to preserve old pricing, got %+v", flat)
	}
	assertPricingBreakdownSums(t, flat)

	interval, err := svc.EstimatePrice(t.Context(), contract.PricingRequest{
		ModelID:          rule.ModelID,
		ProviderID:       rule.ProviderID,
		InputTokens:      150000,
		CacheReadTokens:  50000,
		OutputTokens:     1000,
		CacheWriteTokens: 1000,
	})
	if err != nil {
		t.Fatalf("estimate interval token price: %v", err)
	}
	if interval.Amount != "0.35450000" || interval.InputCost != "0.30000000" || interval.CacheReadCost != "0.05000000" {
		t.Fatalf("expected long-context interval pricing, got %+v", interval)
	}
	assertPricingBreakdownSums(t, interval)
}

func newPricingService(t *testing.T) *service.Service {
	t.Helper()
	svc, err := service.NewPricing(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new billing pricing service: %v", err)
	}
	return svc
}

func assertPricingBreakdownSums(t *testing.T, got contract.PricingResult) {
	t.Helper()
	sum := money.AddMoney(got.InputCost, got.OutputCost)
	sum = money.AddMoney(sum, got.CacheReadCost)
	sum = money.AddMoney(sum, got.CacheWriteCost)
	if sum != got.Amount {
		t.Fatalf("pricing breakdown sum %s != amount %s in %+v", sum, got.Amount, got)
	}
}
