package httpserver

import (
	"strings"
	"testing"

	billingservice "github.com/srapi/srapi/apps/api/internal/modules/billing/service"
	billingmemory "github.com/srapi/srapi/apps/api/internal/modules/billing/store/memory"
)

func TestBuiltInPricingPresetsAreValidAndCoverCommonFamilies(t *testing.T) {
	svc, err := billingservice.NewPricing(billingmemory.New(), nil)
	if err != nil {
		t.Fatalf("new pricing service: %v", err)
	}

	seen := make(map[string]pricingPreset, len(builtInPricingPresets))
	for idx, preset := range builtInPricingPresets {
		family := strings.ToLower(strings.TrimSpace(preset.Family))
		if family == "" {
			t.Fatalf("preset %d has empty family", idx)
		}
		if _, exists := seen[family]; exists {
			t.Fatalf("duplicate pricing preset family %q", family)
		}
		if preset.Source != pricingPresetSourceLiteLLMFallback {
			t.Fatalf("preset %q has unexpected source %q", family, preset.Source)
		}
		req := pricingPresetToCreateRequest(preset)
		if req.ModelFamily != family {
			t.Fatalf("preset %q normalized to unexpected family %q", family, req.ModelFamily)
		}
		if err := svc.ValidatePricingRule(req); err != nil {
			t.Fatalf("preset %q is invalid: %v", family, err)
		}
		seen[family] = preset
	}

	wantFamilies := []string{
		"gpt-4o",
		"gpt-5",
		"gpt-5.1",
		"gpt-5.2",
		"gpt-5.4",
		"gpt-5.5",
		"gpt-image-2",
		"gpt-4o-transcribe",
		"claude-sonnet-4-6",
		"claude-opus-4-8",
		"gemini-2.5-pro",
		"gemini-3.1-pro-preview",
		"deepseek-chat",
		"grok-4.3",
	}
	for _, family := range wantFamilies {
		if _, ok := seen[family]; !ok {
			t.Fatalf("missing built-in pricing preset for %q", family)
		}
	}
}

func TestBuiltInPricingPresetsKeepExpectedFallbackRates(t *testing.T) {
	tests := []struct {
		family      string
		input       string
		output      string
		cacheRead   string
		cacheWrite  string
		imageOutput string
	}{
		{family: "gpt-5.4", input: "2.50000000", output: "15.00000000", cacheRead: "0.25000000", cacheWrite: "2.50000000"},
		{family: "gpt-5-nano", input: "0.05000000", output: "0.40000000", cacheRead: "0.00500000", cacheWrite: "0.05000000"},
		{family: "claude-opus-4-6", input: "5.00000000", output: "25.00000000", cacheRead: "0.50000000", cacheWrite: "6.25000000"},
		{family: "claude-haiku-4-5", input: "1.00000000", output: "5.00000000", cacheRead: "0.10000000", cacheWrite: "1.25000000"},
		{family: "gemini-2.5-flash", input: "0.30000000", output: "2.50000000", cacheRead: "0.03000000", cacheWrite: "0.30000000"},
		{family: "deepseek-chat", input: "0.28000000", output: "0.42000000", cacheRead: "0.02800000", cacheWrite: "0.28000000"},
		{family: "gpt-image-2", input: "5.00000000", output: "10.00000000", cacheRead: "1.25000000", cacheWrite: "5.00000000", imageOutput: "30.00000000"},
	}

	presets := make(map[string]pricingPreset, len(builtInPricingPresets))
	for _, preset := range builtInPricingPresets {
		presets[strings.ToLower(strings.TrimSpace(preset.Family))] = preset
	}

	for _, tt := range tests {
		preset, ok := presets[tt.family]
		if !ok {
			t.Fatalf("missing preset %q", tt.family)
		}
		if preset.Input != tt.input || preset.Output != tt.output || preset.CacheRead != tt.cacheRead || preset.CacheWrite != tt.cacheWrite || preset.ImageOutput != tt.imageOutput {
			t.Fatalf("unexpected preset %q rates: %+v", tt.family, preset)
		}
	}
}
