package service

import (
	"testing"

	"github.com/srapi/srapi/apps/api/internal/modules/subscriptions/contract"
)

func TestPriceFromRuleBillsCacheWriteAtWriteRate(t *testing.T) {
	rule := contract.PricingRule{
		InputPricePerMillionTokens:      "3.00000000",
		OutputPricePerMillionTokens:     "15.00000000",
		CacheReadPricePerMillionTokens:  "0.30000000",
		CacheWritePricePerMillionTokens: "3.75000000",
		Currency:                        "USD",
	}
	req := contract.PricingRequest{
		InputTokens:      1_000_000,
		CacheReadTokens:  1_000_000,
		CacheWriteTokens: 1_000_000,
	}
	// 3.00 (input) + 0.30 (cache read) + 3.75 (cache write) = 7.05
	if got := priceFromRule(rule, req, nil).Amount; got != "7.05000000" {
		t.Fatalf("amount = %q, want 7.05000000 (cache write must bill at the write rate, not read rate)", got)
	}
}

func TestCacheWriteRateFallsBackToInputWhenUnset(t *testing.T) {
	rule := contract.PricingRule{InputPricePerMillionTokens: "3.00000000"}

	if got := cacheWriteRateOrInput(rule); got != "3.00000000" {
		t.Fatalf("unset write rate fallback = %q, want input rate 3.00000000", got)
	}

	rule.CacheWritePricePerMillionTokens = "5.00000000"
	if got := cacheWriteRateOrInput(rule); got != "5.00000000" {
		t.Fatalf("configured write rate = %q, want 5.00000000", got)
	}

	rule.CacheWritePricePerMillionTokens = "0.00000000"
	if got := cacheWriteRateOrInput(rule); got != "3.00000000" {
		t.Fatalf("zero write rate must fall back to input %q, want 3.00000000", got)
	}
}
