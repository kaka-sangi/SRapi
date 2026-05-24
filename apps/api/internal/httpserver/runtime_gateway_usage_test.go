package httpserver

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
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
