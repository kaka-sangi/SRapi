package httpserver

import (
	"strings"
	"time"

	billingcontract "github.com/srapi/srapi/apps/api/internal/modules/billing/contract"
	gatewaycontract "github.com/srapi/srapi/apps/api/internal/modules/gateway/contract"
	schedulercontract "github.com/srapi/srapi/apps/api/internal/modules/scheduler/contract"
)

func contractGatewayPricingRequest(req billingcontract.PricingRequest, estimated bool) billingcontract.GatewayPricingRequest {
	return billingcontract.GatewayPricingRequest{
		PricingRequest: req,
		RateMultiplier: "1.00000000",
		Success:        true,
		Estimated:      estimated,
	}
}

func gatewayPricingRequest(modelID int, candidate schedulercontract.Candidate, usage gatewaycontract.Usage) billingcontract.PricingRequest {
	return billingcontract.PricingRequest{
		ModelID:          modelID,
		ModelFamily:      candidate.ModelFamily,
		ProviderID:       candidate.Provider.ID,
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.CachedTokens,
		CacheWriteTokens: usage.CacheCreationTokens,
		At:               time.Now().UTC(),
		PricingOverride:  cloneAnyMap(candidate.Mapping.PricingOverride),
	}
}

func gatewayPricingRequestForCanonical(modelID int, candidate schedulercontract.Candidate, canonical gatewaycontract.CanonicalRequest, usage gatewaycontract.Usage) billingcontract.PricingRequest {
	req := gatewayPricingRequest(modelID, candidate, usage)
	req.ImageCount = canonical.ImageCount
	req.ImageSize = canonical.ImageSize
	return req
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
