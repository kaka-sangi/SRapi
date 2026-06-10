package litellmpricing_test

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
	litellmpricing "github.com/srapi/srapi/apps/api/internal/workers/litellm_pricing"
)

func TestWorkerUpsertsLiteLLMFamilyPricing(t *testing.T) {
	payload := `{
		"gpt-test": {
			"input_cost_per_token": 0.0000015,
			"output_cost_per_token": "0.0000025",
			"cache_read_input_token_cost": "0.0000001",
			"cache_creation_input_token_cost": "0.0000004"
		},
		"zero-price": {
			"input_cost_per_token": "0",
			"output_cost_per_token": "0"
		}
	}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	store := billingmemory.New()
	worker, err := litellmpricing.New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), litellmpricing.Config{SourceURL: server.URL})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}

	result, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if result.Fetched != 2 || result.Upserted != 1 || result.Skipped != 1 {
		t.Fatalf("unexpected first sync result: %+v", result)
	}
	assertLiteLLMRule(t, store, "1.50000000", "2.50000000")

	payload = `{
		"gpt-test": {
			"input_cost_per_token": "0.000003",
			"output_cost_per_token": "0.000004"
		}
	}`
	result, err = worker.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("run worker again: %v", err)
	}
	if result.Fetched != 1 || result.Upserted != 1 || result.Skipped != 0 {
		t.Fatalf("unexpected second sync result: %+v", result)
	}
	assertLiteLLMRule(t, store, "3.00000000", "4.00000000")
}

func TestWorkerFamilyRuleStaysBelowSpecificPricingRule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"gpt-test": {
				"input_cost_per_token": "0.000001",
				"output_cost_per_token": "0"
			}
		}`)
	}))
	defer server.Close()

	store := billingmemory.New()
	worker, err := litellmpricing.New(store, slog.New(slog.NewTextHandler(io.Discard, nil)), litellmpricing.Config{SourceURL: server.URL})
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if _, err := worker.RunOnce(t.Context()); err != nil {
		t.Fatalf("run worker once: %v", err)
	}
	if _, err := store.CreatePricingRule(t.Context(), billingcontract.PricingRule{
		ModelID:                         41,
		ModelFamily:                     "gpt-test",
		ProviderID:                      0,
		BillingMode:                     billingcontract.BillingModeToken,
		InputPricePerMillionTokens:      "9.00000000",
		OutputPricePerMillionTokens:     "0.00000000",
		CacheReadPricePerMillionTokens:  "0.00000000",
		CacheWritePricePerMillionTokens: "0.00000000",
		Currency:                        "USD",
	}); err != nil {
		t.Fatalf("create specific pricing rule: %v", err)
	}

	pricing, err := billingservice.NewPricing(store, nil)
	if err != nil {
		t.Fatalf("new pricing service: %v", err)
	}
	estimated, err := pricing.EstimatePrice(t.Context(), billingcontract.PricingRequest{
		ModelID:     41,
		ModelFamily: "gpt-test",
		ProviderID:  0,
		InputTokens: 1_000_000,
	})
	if err != nil {
		t.Fatalf("estimate price: %v", err)
	}
	if estimated.Amount != "9.00000000" {
		t.Fatalf("expected specific rule amount 9.00000000, got %+v", estimated)
	}
}

func assertLiteLLMRule(t *testing.T, store *billingmemory.Store, inputPrice string, outputPrice string) {
	t.Helper()
	rules, err := store.ListPricingRules(t.Context())
	if err != nil {
		t.Fatalf("list pricing rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected one pricing rule, got %+v", rules)
	}
	rule := rules[0]
	if rule.ModelID != 0 || rule.ModelFamily != "gpt-test" || rule.ProviderID != 0 {
		t.Fatalf("expected gpt-test family fallback rule, got %+v", rule)
	}
	if rule.InputPricePerMillionTokens != inputPrice || rule.OutputPricePerMillionTokens != outputPrice {
		t.Fatalf("unexpected token prices: %+v", rule)
	}
}
