package httpserver

import (
	"encoding/json"
	"strings"
	"time"

	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

// contractGatewayPricingRequest builds the priceable request. rateMultiplier
// is the per-account group rate (gatewayAccountRateMultiplier), or the
// canonical "1.00000000" when the caller doesn't yet know which account will
// serve the request (e.g. admission, before scheduling). Passing the real
// multiplier here matches what recordGatewayUsage later persists, so the
// estimated price the gateway returns matches what the balance_charger
// ultimately debits.
func contractGatewayPricingRequest(req billingcontract.PricingRequest, rateMultiplier string, estimated bool) billingcontract.GatewayPricingRequest {
	if strings.TrimSpace(rateMultiplier) == "" {
		rateMultiplier = "1.00000000"
	}
	return billingcontract.GatewayPricingRequest{
		PricingRequest: req,
		RateMultiplier: rateMultiplier,
		Success:        true,
		Estimated:      estimated,
	}
}

func gatewayPricingRequest(modelID int, candidate schedulercontract.Candidate, usage gatewaycontract.Usage) billingcontract.PricingRequest {
	return billingcontract.PricingRequest{
		ModelID:            modelID,
		ModelFamily:        candidate.ModelFamily,
		ProviderID:         candidate.Provider.ID,
		InputTokens:        usage.InputTokens,
		OutputTokens:       usage.OutputTokens,
		ImageOutputTokens:  usage.ImageOutputTokens,
		CacheReadTokens:    usage.CachedTokens,
		CacheWriteTokens:   usage.CacheCreationTokens,
		CacheWrite5mTokens: usage.CacheCreation5mTokens,
		CacheWrite1hTokens: usage.CacheCreation1hTokens,
		At:                 time.Now().UTC(),
		PricingOverride:    cloneAnyMap(candidate.Mapping.PricingOverride),
	}
}

func gatewayPricingRequestForCanonical(modelID int, candidate schedulercontract.Candidate, canonical gatewaycontract.CanonicalRequest, usage gatewaycontract.Usage) billingcontract.PricingRequest {
	req := gatewayPricingRequest(modelID, candidate, usage)
	req.ImageCount = canonical.ImageCount
	req.ImageSize = canonical.ImageSize
	req.RequestedModel = gatewayRequestedModel(canonical)
	req.UpstreamModel = gatewayUpstreamModel(candidate)
	req.BillingModelSource = mapString(candidate.Mapping.PricingOverride, "billing_model_source")
	req.ServiceTier = gatewayServiceTier(canonical)
	return req
}

func gatewayServiceTier(canonical gatewaycontract.CanonicalRequest) string {
	var raw map[string]any
	if len(canonical.RawBody) == 0 {
		return ""
	}
	if err := json.Unmarshal(canonical.RawBody, &raw); err != nil {
		return ""
	}
	return normalizeGatewayServiceTier(mapString(raw, "service_tier"))
}

func normalizeGatewayServiceTier(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	if value == "fast" {
		value = "priority"
	}
	switch value {
	case "priority", "flex", "auto", "default", "scale":
		return value
	default:
		return ""
	}
}

func gatewayRequestedModel(canonical gatewaycontract.CanonicalRequest) string {
	if model := strings.TrimSpace(canonical.Model); model != "" {
		return model
	}
	return strings.TrimSpace(canonical.CanonicalModel)
}

func gatewayUpstreamModel(candidate schedulercontract.Candidate) string {
	return strings.TrimSpace(candidate.Mapping.UpstreamModelName)
}

func gatewayUsageModelSnapshot(canonical gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) (string, string) {
	upstream := gatewayUpstreamModel(candidate)
	if upstream == "" {
		upstream = strings.TrimSpace(canonical.CanonicalModel)
	}
	return gatewayRequestedModel(canonical), upstream
}

func gatewayUsageRequestedSnapshot(canonical gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) string {
	requested, _ := gatewayUsageModelSnapshot(canonical, candidate)
	return requested
}

func gatewayUsageUpstreamSnapshot(canonical gatewaycontract.CanonicalRequest, candidate schedulercontract.Candidate) string {
	_, upstream := gatewayUsageModelSnapshot(canonical, candidate)
	return upstream
}
